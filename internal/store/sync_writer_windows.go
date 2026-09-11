//go:build windows

package store

import (
	"errors"
	"os"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"golang.org/x/sys/windows"
)

func acquireSyncWriterFile(path string) (func() error, error) {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("sync writer lock cannot be a symlink")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("cannot inspect sync writer lock")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, errors.New("cannot open sync writer lock")
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, errors.New("sync writer lock must be a regular file")
	}
	overlapped := &windows.Overlapped{}
	err = windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, overlapped)
	if err != nil {
		file.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, core.ErrSyncInProgress
		}
		return nil, errors.New("cannot acquire sync writer lock")
	}
	return func() error {
		unlockErr := windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, overlapped)
		closeErr := file.Close()
		if unlockErr != nil || closeErr != nil {
			return errors.New("cannot release sync writer lock")
		}
		return nil
	}, nil
}
