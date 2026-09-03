package releasecontract

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPOSIXInstallerInstallsVerifiedPinnedReleaseInUserDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer is covered on Unix runners")
	}

	releaseDir := t.TempDir()
	artifact, err := Resolve("v1.2.3", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	archiveName := artifact.ArchiveName
	archivePath := filepath.Join(releaseDir, archiveName)
	writeTarGzip(t, archivePath, map[string]string{
		"coupangctl":        "#!/bin/sh\nprintf '{\"name\":\"coupangctl\",\"version\":\"1.2.3\"}\\n'\n",
		"LICENSE":           "synthetic license",
		"README.md":         "synthetic readme",
		"BROWSER_BRIDGE.md": "synthetic bridge docs",
	})
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
	command := exec.Command("/bin/sh", filepath.Join("..", "..", "installers", "install.sh"), "--version", "v1.2.3")
	command.Env = append(os.Environ(),
		"COUPANGCTL_INSTALL_BASE_URL="+server.URL,
		"COUPANGCTL_INSTALL_DIR="+installDir,
		"COUPANGCTL_INSTALL_GOOS=linux",
		"COUPANGCTL_INSTALL_GOARCH=amd64",
	)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("install.sh error = %v, output = %s", err, output)
	}
	if got, want := strings.TrimSpace(string(output)), `{"name":"coupangctl","version":"1.2.3","status":"installed"}`; got != want {
		t.Fatalf("install.sh output = %q, want %q", got, want)
	}

	installedPath := filepath.Join(installDir, "coupangctl")
	info, err := os.Lstat(installedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("installed file mode = %v, want executable regular file", info.Mode())
	}
}

func TestPOSIXInstallerRejectsUnexpectedArchiveContentAndPreservesExistingBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer is covered on Unix runners")
	}

	releaseDir := t.TempDir()
	artifact, err := Resolve("v1.2.3", "linux", "amd64")
	if err != nil {
		t.Fatal(err)
	}
	archiveName := artifact.ArchiveName
	archivePath := filepath.Join(releaseDir, archiveName)
	writeTarGzip(t, archivePath, map[string]string{
		"coupangctl":         "#!/bin/sh\nprintf '{\\n  \"name\": \"coupangctl\",\\n  \"version\": \"1.2.3\"\\n}\\n'\n",
		"LICENSE":            "synthetic license",
		"README.md":          "synthetic readme",
		"BROWSER_BRIDGE.md":  "synthetic bridge docs",
		"private-order.json": "must never ship",
	})
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
	destination := filepath.Join(installDir, "coupangctl")
	if err := os.WriteFile(destination, []byte("existing release"), 0o700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("/bin/sh", filepath.Join("..", "..", "installers", "install.sh"), "--version", "v1.2.3")
	command.Env = append(os.Environ(),
		"COUPANGCTL_INSTALL_BASE_URL="+server.URL,
		"COUPANGCTL_INSTALL_DIR="+installDir,
		"COUPANGCTL_INSTALL_GOOS=linux",
		"COUPANGCTL_INSTALL_GOARCH=amd64",
	)
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "release archive contains unexpected entries") {
		t.Fatalf("install.sh error = %v, output = %s; want unexpected-entry rejection", err, output)
	}
	contents, readErr := os.ReadFile(destination)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(contents) != "existing release" {
		t.Fatalf("existing binary changed after rejected archive: %q", contents)
	}
}

func TestPOSIXInstallerRejectsMovingVersionWithStructuredErrorBeforeNetwork(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX installer is covered on Unix runners")
	}
	command := exec.Command("/bin/sh", filepath.Join("..", "..", "installers", "install.sh"), "--version", "main")
	command.Env = append(os.Environ(), "COUPANGCTL_INSTALL_BASE_URL=https://127.0.0.1:1")
	output, err := command.CombinedOutput()
	want := `{"error":{"code":"invalid_version","message":"version must be an immutable semantic release tag such as v0.1.0"}}`
	if err == nil || strings.TrimSpace(string(output)) != want {
		t.Fatalf("install.sh error = %v, output = %q, want %q", err, strings.TrimSpace(string(output)), want)
	}
}
