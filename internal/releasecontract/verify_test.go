package releasecontract

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var releaseTargets = []struct {
	goos   string
	goarch string
}{
	{"darwin", "amd64"},
	{"darwin", "arm64"},
	{"linux", "amd64"},
	{"linux", "arm64"},
	{"windows", "amd64"},
	{"windows", "arm64"},
}

func TestVerifyAcceptsSixTargetArchives(t *testing.T) {
	dist := makeSyntheticDist(t, "")

	if err := Verify(dist); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestVerifyRejectsUnexpectedPrivatePayload(t *testing.T) {
	dist := makeSyntheticDist(t, "private-order.json")

	err := Verify(dist)
	if err == nil || !strings.Contains(err.Error(), "unexpected archive entry") {
		t.Fatalf("Verify() error = %v, want unexpected archive entry", err)
	}
}

func TestVerifyRejectsMissingTarget(t *testing.T) {
	dist := makeSyntheticDist(t, "")
	if err := os.Remove(filepath.Join(dist, "coupangctl_0.1.0_linux_arm64.tar.gz")); err != nil {
		t.Fatal(err)
	}

	err := Verify(dist)
	if err == nil || !strings.Contains(err.Error(), "release target set") {
		t.Fatalf("Verify() error = %v, want release target set", err)
	}
}

func TestVerifyRejectsChecksumMismatch(t *testing.T) {
	dist := makeSyntheticDist(t, "")
	checksums := filepath.Join(dist, "checksums.txt")
	contents, err := os.ReadFile(checksums)
	if err != nil {
		t.Fatal(err)
	}
	contents[0] = '0'
	if err := os.WriteFile(checksums, contents, 0o600); err != nil {
		t.Fatal(err)
	}

	err = Verify(dist)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Verify() error = %v, want checksum mismatch", err)
	}
}

func TestVerifyWithOptionsRequiresEverySBOM(t *testing.T) {
	dist := makeSyntheticDist(t, "")

	err := VerifyWithOptions(dist, Options{RequireSBOM: true})
	if err == nil || !strings.Contains(err.Error(), "SBOM set") {
		t.Fatalf("VerifyWithOptions() error = %v, want SBOM set", err)
	}
}

func TestVerifyRejectsUnexpectedChecksummedArtifact(t *testing.T) {
	dist := makeSyntheticDist(t, "")
	unexpectedPath := filepath.Join(dist, "raw-response.json")
	if err := os.WriteFile(unexpectedPath, []byte("must never ship"), 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256File(t, unexpectedPath)
	checksumsPath := filepath.Join(dist, "checksums.txt")
	file, err := os.OpenFile(checksumsPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintf(file, "%x  raw-response.json\n", sum); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	err = Verify(dist)
	if err == nil || !strings.Contains(err.Error(), "unexpected checksummed artifact") {
		t.Fatalf("Verify() error = %v, want unexpected checksummed artifact", err)
	}
}

func makeSyntheticDist(t *testing.T, unexpectedEntry string) string {
	t.Helper()
	dist := t.TempDir()
	var checksumLines []string

	for _, target := range releaseTargets {
		ext := ".tar.gz"
		if target.goos == "windows" {
			ext = ".zip"
		}
		name := fmt.Sprintf("coupangctl_0.1.0_%s_%s%s", target.goos, target.goarch, ext)
		path := filepath.Join(dist, name)
		entries := map[string]string{
			"LICENSE":           "synthetic license",
			"README.md":         "synthetic readme",
			"BROWSER_BRIDGE.md": "synthetic bridge docs",
		}
		binary := "coupangctl"
		if target.goos == "windows" {
			binary += ".exe"
		}
		entries[binary] = "synthetic executable"
		if unexpectedEntry != "" && target.goos == "darwin" && target.goarch == "arm64" {
			entries[unexpectedEntry] = "must never ship"
		}

		if ext == ".zip" {
			writeZip(t, path, entries)
		} else {
			writeTarGzip(t, path, entries)
		}
		sum := sha256File(t, path)
		checksumLines = append(checksumLines, fmt.Sprintf("%x  %s", sum, name))
	}
	sort.Strings(checksumLines)
	if err := os.WriteFile(filepath.Join(dist, "checksums.txt"), []byte(strings.Join(checksumLines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return dist
}

func writeTarGzip(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gz := gzip.NewWriter(file)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()
	writeTarEntries(t, tw, entries)
}

func writeTarEntries(t *testing.T, tw *tar.Writer, entries map[string]string) {
	t.Helper()
	for name, value := range entries {
		mode := int64(0o644)
		if name == "coupangctl" {
			mode = 0o755
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(value)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(tw, value); err != nil {
			t.Fatal(err)
		}
	}
}

func writeZip(t *testing.T, path string, entries map[string]string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	zw := zip.NewWriter(file)
	defer zw.Close()
	for name, value := range entries {
		entry, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(entry, value); err != nil {
			t.Fatal(err)
		}
	}
}

func sha256File(t *testing.T, path string) [sha256.Size]byte {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		t.Fatal(err)
	}
	var sum [sha256.Size]byte
	copy(sum[:], hash.Sum(nil))
	return sum
}
