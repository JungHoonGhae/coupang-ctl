//go:build windows

package browser

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

const profileLockFilename = ".coupangctl.lock"

var ErrProfileInUse = errors.New("dedicated browser profile is already in use")

type profileLock struct {
	file       *os.File
	overlapped windows.Overlapped
}

func acquireProfileLock(profileDir string) (*profileLock, error) {
	path := filepath.Join(profileDir, profileLockFilename)
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("dedicated browser profile lock cannot be a symlink")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect dedicated browser profile lock: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open dedicated browser profile lock: %w", err)
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, errors.New("dedicated browser profile lock is not a regular file")
	}
	lock := &profileLock{file: file}
	err = windows.LockFileEx(
		windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		&lock.overlapped,
	)
	if err != nil {
		_ = file.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, ErrProfileInUse
		}
		return nil, fmt.Errorf("lock dedicated browser profile: %w", err)
	}
	return lock, nil
}

func (lock *profileLock) release() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	unlockErr := windows.UnlockFileEx(windows.Handle(lock.file.Fd()), 0, 1, 0, &lock.overlapped)
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
