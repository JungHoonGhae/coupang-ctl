package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestBrowserBridgeDoctorReportsReadyAndDetectsManifestTampering(t *testing.T) {
	home := t.TempDir()
	stateDir := filepath.Join(home, "state")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("COUPANGCTL_STATE_DIR", stateDir)

	if err := Run(context.Background(), []string{"browser-bridge", "install"}, ioDiscard{}, ioDiscard{}, "test"); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := Run(context.Background(), []string{"browser-bridge", "doctor"}, &stdout, ioDiscard{}, "test"); err != nil {
		t.Fatal(err)
	}
	var ready core.BrowserBridgeDoctorReport
	if err := json.Unmarshal(stdout.Bytes(), &ready); err != nil {
		t.Fatal(err)
	}
	if !ready.Ready || ready.Status != "ready" || ready.ExtensionLoadStatus != "not_checked" || ready.NextAction != "load_or_verify_extension_in_chrome" || len(ready.Checks) < 3 {
		t.Fatalf("unexpected ready report: %#v", ready)
	}
	for _, check := range ready.Checks {
		if check.Status != core.CheckOK || check.Message != "" {
			t.Fatalf("unexpected ready check: %#v", check)
		}
	}

	if err := os.WriteFile(ready.NativeHostManifestPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"browser-bridge", "doctor"}, &stdout, ioDiscard{}, "test"); err != nil {
		t.Fatal(err)
	}
	var damaged core.BrowserBridgeDoctorReport
	if err := json.Unmarshal(stdout.Bytes(), &damaged); err != nil {
		t.Fatal(err)
	}
	if damaged.Ready || damaged.Status != "repair_required" {
		t.Fatalf("unexpected damaged report: %#v", damaged)
	}
	found := false
	for _, check := range damaged.Checks {
		if check.Name == "native_host_manifest" && check.Status == core.CheckError {
			found = true
			if check.Message == "" {
				t.Fatal("damaged manifest check must provide a redacted remediation message")
			}
		}
	}
	if !found {
		t.Fatalf("missing native host manifest failure: %#v", damaged.Checks)
	}
}

func TestBrowserBridgeUninstallRemovesOnlyOwnedFilesAndRefusesTampering(t *testing.T) {
	home := t.TempDir()
	stateDir := filepath.Join(home, "state")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("COUPANGCTL_STATE_DIR", stateDir)

	var installOutput bytes.Buffer
	if err := Run(context.Background(), []string{"browser-bridge", "install"}, &installOutput, ioDiscard{}, "test"); err != nil {
		t.Fatal(err)
	}
	var installation core.BrowserBridgeInstallation
	if err := json.Unmarshal(installOutput.Bytes(), &installation); err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(filepath.Dir(installation.NativeHostManifestPath), "other.application.json")
	if err := os.WriteFile(unrelated, []byte("unrelated"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := Run(context.Background(), []string{"browser-bridge", "uninstall"}, &stdout, ioDiscard{}, "test"); err != nil {
		t.Fatal(err)
	}
	var removed core.BrowserBridgeUninstallResult
	if err := json.Unmarshal(stdout.Bytes(), &removed); err != nil {
		t.Fatal(err)
	}
	if removed.Status != "removed" || len(removed.Removed) != 3 {
		t.Fatalf("unexpected uninstall response: %#v", removed)
	}
	if _, err := os.Stat(installation.NativeHostManifestPath); !os.IsNotExist(err) {
		t.Fatalf("native host manifest still exists or could not be checked: %v", err)
	}
	if _, err := os.Stat(installation.ExtensionPath); !os.IsNotExist(err) {
		t.Fatalf("extension bundle still exists or could not be checked: %v", err)
	}
	if content, err := os.ReadFile(unrelated); err != nil || string(content) != "unrelated" {
		t.Fatalf("unrelated native host file was changed: %q %v", content, err)
	}

	installOutput.Reset()
	if err := Run(context.Background(), []string{"browser-bridge", "install"}, &installOutput, ioDiscard{}, "test"); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(installOutput.Bytes(), &installation); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installation.NativeHostManifestPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), []string{"browser-bridge", "uninstall"}, ioDiscard{}, ioDiscard{}, "test"); err == nil {
		t.Fatal("expected uninstall to refuse a modified native host manifest")
	}
	if _, err := os.Stat(installation.ExtensionPath); err != nil {
		t.Fatalf("refused uninstall changed the extension bundle: %v", err)
	}
	if content, err := os.ReadFile(installation.NativeHostManifestPath); err != nil || string(content) != "{}\n" {
		t.Fatalf("refused uninstall changed the modified manifest: %q %v", content, err)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(payload []byte) (int, error) { return len(payload), nil }

func TestBrowserBridgeInstallCreatesPrivateBundleAndExactNativeHost(t *testing.T) {
	home := t.TempDir()
	stateDir := filepath.Join(home, "state")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("COUPANGCTL_STATE_DIR", stateDir)

	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"browser-bridge", "install"}, &stdout, &stderr, "test"); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}

	var got core.BrowserBridgeInstallation
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != 1 || got.Status != "installed" || got.ExtensionID != browser.OrdinaryBrowserExtensionID {
		t.Fatalf("unexpected installation response: %#v", got)
	}
	if !filepath.IsAbs(got.ExtensionPath) || !pathWithin(got.ExtensionPath, stateDir) {
		t.Fatalf("extension path escaped state directory: %q", got.ExtensionPath)
	}
	if !filepath.IsAbs(got.NativeHostManifestPath) || !pathWithin(got.NativeHostManifestPath, home) {
		t.Fatalf("native host manifest escaped fake home: %q", got.NativeHostManifestPath)
	}

	manifestBytes, err := os.ReadFile(got.NativeHostManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Name           string   `json:"name"`
		Path           string   `json:"path"`
		Type           string   `json:"type"`
		AllowedOrigins []string `json:"allowed_origins"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	wantOrigin := "chrome-extension://" + browser.OrdinaryBrowserExtensionID + "/"
	if manifest.Name != "com.coupangctl.browser_bridge" || manifest.Type != "stdio" || !filepath.IsAbs(manifest.Path) || len(manifest.AllowedOrigins) != 1 || manifest.AllowedOrigins[0] != wantOrigin {
		t.Fatalf("unexpected native host manifest: %#v", manifest)
	}
	info, err := os.Stat(got.NativeHostManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("native host manifest mode = %o, want 600", info.Mode().Perm())
	}
	for _, filename := range []string{"manifest.json", "action.js", "page-reader.js", "service-worker.js"} {
		info, err := os.Stat(filepath.Join(got.ExtensionPath, filename))
		if err != nil {
			t.Fatalf("installed extension file %s: %v", filename, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("extension file %s mode = %o, want 600", filename, info.Mode().Perm())
		}
	}
}

func pathWithin(path, root string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && relative != "." && !filepath.IsAbs(relative) && relative[:1] != string(filepath.Separator)
}
