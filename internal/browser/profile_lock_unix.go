//go:build !windows

package browser

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const profileLockFilename = ".coupangctl.lock"

var ErrProfileInUse = errors.New("dedicated browser profile is already in use")

type profileLock struct {
	file *os.File
}

func acquireProfileLock(profileDir string) (*profileLock, error) {
	path := filepath.Join(profileDir, profileLockFilename)
	descriptor, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open dedicated browser profile lock: %w", err)
	}
	file := os.NewFile(uintptr(descriptor), path)
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, errors.New("dedicated browser profile lock is not a regular file")
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("secure dedicated browser profile lock: %w", err)
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, ErrProfileInUse
		}
		return nil, fmt.Errorf("lock dedicated browser profile: %w", err)
	}
	return &profileLock{file: file}, nil
}

func (lock *profileLock) release() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	unlockErr := unix.Flock(int(lock.file.Fd()), unix.LOCK_UN)
	closeErr := lock.file.Close()
	lock.file = nil
	if unlockErr != nil {
		return fmt.Errorf("unlock dedicated browser profile: %w", unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close dedicated browser profile lock: %w", closeErr)
	}
	return nil
}
