package browserbridge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallPreflightsConflictsBeforeWritingAnything(t *testing.T) {
	manager := newTestManager(t)
	manifestPath, err := manager.nativeHostManifestPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte("owned by another application\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := manager.Install(); err == nil {
		t.Fatal("expected install to reject a conflicting native host manifest")
	}
	if _, err := os.Stat(manager.extensionPath()); !os.IsNotExist(err) {
		t.Fatalf("failed preflight left an extension bundle behind: %v", err)
	}
	if _, err := os.Stat(manager.installationRecordPath()); !os.IsNotExist(err) {
		t.Fatalf("failed preflight left an ownership record behind: %v", err)
	}
	content, err := os.ReadFile(manifestPath)
	if err != nil || string(content) != "owned by another application\n" {
		t.Fatalf("conflicting manifest was changed: %q %v", content, err)
	}
}

func TestInstallRejectsNonExecutableBeforeWritingArtifacts(t *testing.T) {
	manager := newTestManager(t)
	if err := os.Chmod(manager.environment.ExecutablePath, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Install(); err == nil {
		t.Fatal("expected install to reject a non-executable binary")
	}
	if _, err := os.Stat(manager.extensionPath()); !os.IsNotExist(err) {
		t.Fatalf("rejected install wrote an extension bundle: %v", err)
	}
}

func TestInstallRejectsManagedDirectorySymlinkWithoutTouchingTarget(t *testing.T) {
	manager := newTestManager(t)
	victim := filepath.Join(t.TempDir(), "victim")
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(victim, "sentinel")
	if err := os.WriteFile(sentinel, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(manager.extensionPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, manager.extensionPath()); err != nil {
		t.Fatal(err)
	}

	if _, err := manager.Install(); err == nil {
		t.Fatal("expected install to reject a symlinked managed extension directory")
	}
	entries, err := os.ReadDir(victim)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "sentinel" {
		t.Fatalf("rejected install wrote through the symlink: %#v", entries)
	}
	content, err := os.ReadFile(sentinel)
	if err != nil || string(content) != "unchanged" {
		t.Fatalf("sentinel changed: %q %v", content, err)
	}
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	root := t.TempDir()
	executable := filepath.Join(root, "bin", "coupangctl")
	if err := os.MkdirAll(filepath.Dir(executable), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("synthetic executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	manager, err := New(Environment{
		GOOS:           "darwin",
		HomeDir:        filepath.Join(root, "home"),
		ConfigDir:      filepath.Join(root, "config"),
		StateDir:       filepath.Join(root, "state"),
		ExecutablePath: executable,
	})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}
