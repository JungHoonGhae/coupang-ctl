package browser

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDedicatedProfileLockRejectsConcurrentOwnerAndCanBeReacquired(t *testing.T) {
	profileDir := t.TempDir()
	first, err := acquireProfileLock(profileDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireProfileLock(profileDir); !errors.Is(err, ErrProfileInUse) {
		t.Fatalf("second profile lock error = %v, want ErrProfileInUse", err)
	}
	if err := first.release(); err != nil {
		t.Fatal(err)
	}
	second, err := acquireProfileLock(profileDir)
	if err != nil {
		t.Fatalf("reacquire profile lock: %v", err)
	}
	if err := second.release(); err != nil {
		t.Fatal(err)
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(profileDir, profileLockFilename))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("profile lock mode = %#o, want 0600", info.Mode().Perm())
		}
	}
}

func TestDedicatedProfileLockRejectsSymlink(t *testing.T) {
	profileDir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside-lock")
	if err := os.WriteFile(outside, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(profileDir, profileLockFilename)); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if _, err := acquireProfileLock(profileDir); err == nil {
		t.Fatal("symlink profile lock was accepted")
	}
}
