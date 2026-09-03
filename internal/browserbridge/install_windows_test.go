//go:build windows

package browserbridge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

func TestWindowsManagerUsesOwnedPerUserRegistryRegistration(t *testing.T) {
	keyPath := fmt.Sprintf(`Software\coupangctl-test-%d-%d`, os.Getpid(), time.Now().UnixNano())
	registration := windowsPlatformRegistration{keyPath: keyPath}
	t.Cleanup(func() { _ = registry.DeleteKey(registry.CURRENT_USER, keyPath) })

	root := t.TempDir()
	executable := filepath.Join(root, "bin", "coupangctl.exe")
	if err := os.MkdirAll(filepath.Dir(executable), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("synthetic Windows executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := New(Environment{
		GOOS:           "windows",
		HomeDir:        filepath.Join(root, "home"),
		ConfigDir:      filepath.Join(root, "config"),
		StateDir:       filepath.Join(root, "state"),
		ExecutablePath: executable,
	})
	if err != nil {
		t.Fatal(err)
	}
	manager.registration = registration

	installed, err := manager.Install()
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if installed.Status != "installed" {
		t.Fatalf("install status = %q, want installed", installed.Status)
	}
	if err := registration.Check(installed.NativeHostManifestPath); err != nil {
		t.Fatalf("Windows registration check error = %v", err)
	}
	if err := registration.Preflight(installed.NativeHostManifestPath + ".other"); err == nil {
		t.Fatal("expected Windows registration preflight to reject another manifest")
	}
	if report := manager.Doctor(); !report.Ready {
		t.Fatalf("Windows doctor report = %#v", report)
	}

	removed, err := manager.Uninstall()
	if err != nil {
		t.Fatalf("Uninstall() error = %v", err)
	}
	if removed.Status != "removed" {
		t.Fatalf("uninstall status = %q, want removed", removed.Status)
	}
	if err := registration.Check(installed.NativeHostManifestPath); !errors.Is(err, registry.ErrNotExist) {
		t.Fatalf("Windows registry key remains after uninstall: %v", err)
	}
}
