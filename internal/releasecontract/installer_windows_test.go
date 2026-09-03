//go:build windows

package releasecontract

import (
	"archive/zip"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPowerShellInstallerInstallsVerifiedPinnedReleaseInUserDirectory(t *testing.T) {
	releaseDir := t.TempDir()
	fakeBinary := filepath.Join(releaseDir, "coupangctl.exe")
	build := exec.Command("go", "build", "-o", fakeBinary, "./testdata/fakecoupangctl")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build synthetic executable: %v\n%s", err, output)
	}

	artifact, err := Resolve("v1.2.3", "windows", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	archiveName := artifact.ArchiveName
	archivePath := filepath.Join(releaseDir, archiveName)
	writeWindowsInstallerArchive(t, archivePath, fakeBinary, false)
	archive, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(archive)
	checksums := fmt.Sprintf("%x  %s\n", digest, archiveName)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1.2.3/" + archiveName:
			_, _ = response.Write(archive)
		case "/v1.2.3/checksums.txt":
			_, _ = response.Write([]byte(checksums))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	installDir := filepath.Join(t.TempDir(), "bin")
	command := exec.Command(
		"pwsh", "-NoLogo", "-NoProfile", "-NonInteractive",
		"-File", filepath.Join("..", "..", "installers", "install.ps1"),
		"-Version", "v1.2.3", "-InstallDir", installDir,
	)
	command.Env = append(os.Environ(),
		"COUPANGCTL_INSTALL_BASE_URL="+server.URL,
		"COUPANGCTL_INSTALL_GOARCH=amd64",
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("install.ps1 error = %v, output = %s", err, output)
	}
	if got, want := strings.TrimSpace(string(output)), `{"name":"coupangctl","version":"1.2.3","status":"installed"}`; got != want {
		t.Fatalf("install.ps1 output = %q, want %q", got, want)
	}
	if info, err := os.Lstat(filepath.Join(installDir, "coupangctl.exe")); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("installed executable info = %v, error = %v", info, err)
	}
}

func TestPowerShellInstallerRejectsUnexpectedArchiveContentAndPreservesExistingBinary(t *testing.T) {
	releaseDir := t.TempDir()
	fakeBinary := filepath.Join(releaseDir, "coupangctl.exe")
	build := exec.Command("go", "build", "-o", fakeBinary, "./testdata/fakecoupangctl")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build synthetic executable: %v\n%s", err, output)
	}
	artifact, err := Resolve("v1.2.3", "windows", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	archiveName := artifact.ArchiveName
	archivePath := filepath.Join(releaseDir, archiveName)
	writeWindowsInstallerArchive(t, archivePath, fakeBinary, true)
	archive, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(archive)
	checksums := fmt.Sprintf("%x  %s\n", digest, archiveName)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1.2.3/" + archiveName:
			_, _ = response.Write(archive)
		case "/v1.2.3/checksums.txt":
			_, _ = response.Write([]byte(checksums))
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()

	installDir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(installDir, 0o700); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(installDir, "coupangctl.exe")
	if err := os.WriteFile(destination, []byte("existing release"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(
		"pwsh", "-NoLogo", "-NoProfile", "-NonInteractive",
		"-File", filepath.Join("..", "..", "installers", "install.ps1"),
		"-Version", "v1.2.3", "-InstallDir", installDir,
	)
	command.Env = append(os.Environ(),
		"COUPANGCTL_INSTALL_BASE_URL="+server.URL,
		"COUPANGCTL_INSTALL_GOARCH=amd64",
	)
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "unexpected_archive_content") {
		t.Fatalf("install.ps1 error = %v, output = %s; want unexpected-entry rejection", err, output)
	}
	contents, readErr := os.ReadFile(destination)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(contents) != "existing release" {
		t.Fatalf("existing binary changed after rejected archive: %q", contents)
	}
}

func writeWindowsInstallerArchive(t *testing.T, archivePath, executablePath string, includeUnexpected bool) {
	t.Helper()
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	writer := zip.NewWriter(file)
	defer writer.Close()
	executable, err := os.ReadFile(executablePath)
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string][]byte{
		"coupangctl.exe":    executable,
		"LICENSE":           []byte("synthetic license"),
		"README.md":         []byte("synthetic readme"),
		"BROWSER_BRIDGE.md": []byte("synthetic bridge docs"),
	}
	if includeUnexpected {
		entries["private-order.json"] = []byte("must never ship")
	}
	for name, contents := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(contents); err != nil {
			t.Fatal(err)
		}
	}
}
