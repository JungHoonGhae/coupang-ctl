//go:build !windows

package store

import (
	"errors"
	"os"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"golang.org/x/sys/unix"
)

func acquireSyncWriterFile(path string) (func() error, error) {
	fd, err := unix.Open(path, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, errors.New("cannot open sync writer lock")
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, errors.New("sync writer lock must be a regular file")
	}
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return nil, errors.New("cannot secure sync writer lock")
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, core.ErrSyncInProgress
		}
		return nil, errors.New("cannot acquire sync writer lock")
	}
	return func() error {
		unlockErr := unix.Flock(fd, unix.LOCK_UN)
		closeErr := file.Close()
		if unlockErr != nil || closeErr != nil {
			return errors.New("cannot release sync writer lock")
		}
		return nil
	}, nil
}
