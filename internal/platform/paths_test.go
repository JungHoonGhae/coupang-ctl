package platform

import (
	"path/filepath"
	"testing"
)

func TestDefaultPathsUsesAbsoluteOverride(t *testing.T) {
	root := t.TempDir()
	t.Setenv("COUPANGCTL_STATE_DIR", root)

	got, err := DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if got.StateDir != root {
		t.Fatalf("state dir = %q, want %q", got.StateDir, root)
	}
	if got.ProfileDir != filepath.Join(root, "browser-profile") {
		t.Fatalf("profile dir = %q", got.ProfileDir)
	}
	if got.Database != filepath.Join(root, "coupangctl.sqlite3") {
		t.Fatalf("database = %q", got.Database)
	}
}

func TestDefaultPathsRejectsRelativeOverride(t *testing.T) {
	t.Setenv("COUPANGCTL_STATE_DIR", "relative")
	if _, err := DefaultPaths(); err == nil {
		t.Fatal("expected relative state directory to be rejected")
	}
}
