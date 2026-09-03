package extensionpack

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	extensionbundle "github.com/JungHoonGhae/coupang-ctl/extension"
)

func TestBuildWritesDeterministicVerifiedPackage(t *testing.T) {
	directory := t.TempDir()
	firstPath := filepath.Join(directory, "first.zip")
	secondPath := filepath.Join(directory, "second.zip")

	first, err := Build(firstPath)
	if err != nil {
		t.Fatalf("Build(first) error = %v", err)
	}
	second, err := Build(secondPath)
	if err != nil {
		t.Fatalf("Build(second) error = %v", err)
	}

	firstBytes, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatal("Build() produced different bytes for the same embedded bundle")
	}
	if first.SHA256 != second.SHA256 || len(first.SHA256) != 64 {
		t.Fatalf("SHA256 = %q and %q, want matching 64-character digests", first.SHA256, second.SHA256)
	}
	if first.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d, want 1", first.SchemaVersion)
	}
	wantFiles := append([]string(nil), extensionbundle.Filenames...)
	sort.Strings(wantFiles)
	if strings.Join(first.Files, "\n") != strings.Join(wantFiles, "\n") {
		t.Fatalf("Files = %v, want %v", first.Files, wantFiles)
	}
	if first.FileCount != len(wantFiles) || first.SizeBytes != int64(len(firstBytes)) {
		t.Fatalf("report counts = (%d, %d), want (%d, %d)", first.FileCount, first.SizeBytes, len(wantFiles), len(firstBytes))
	}
	if first.OutputPath != firstPath {
		t.Fatalf("OutputPath = %q, want %q", first.OutputPath, firstPath)
	}

	verified, err := Verify(firstPath)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if verified.SHA256 != first.SHA256 {
		t.Fatalf("Verify().SHA256 = %q, want %q", verified.SHA256, first.SHA256)
	}
	assertRootAllowlist(t, firstPath, wantFiles)
}

func TestBuildDoesNotOverwriteExistingOutput(t *testing.T) {
	output := filepath.Join(t.TempDir(), "extension.zip")
	const marker = "keep this file"
	if err := os.WriteFile(output, []byte(marker), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Build(output); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Build() error = %v, want already exists", err)
	}
	contents, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != marker {
		t.Fatalf("existing output = %q, want unchanged marker", contents)
	}
}

func TestVerifyRejectsUnexpectedEntry(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "unexpected.zip")
	writeSyntheticArchive(t, archivePath, "debug-order.json", nil)

	if _, err := Verify(archivePath); err == nil || !strings.Contains(err.Error(), "unexpected archive entry") {
		t.Fatalf("Verify() error = %v, want unexpected archive entry", err)
	}
}

func TestVerifyRejectsTamperedBundleFile(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "tampered.zip")
	writeSyntheticArchive(t, archivePath, "", map[string][]byte{"popup.html": []byte("tampered")})

	if _, err := Verify(archivePath); err == nil || !strings.Contains(err.Error(), "content mismatch") {
		t.Fatalf("Verify() error = %v, want content mismatch", err)
	}
}

func TestVerifyRejectsMissingEntry(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "missing.zip")
	names := make([]string, 0, len(extensionbundle.Filenames)-1)
	for _, name := range extensionbundle.Filenames {
		if name != "action.js" {
			names = append(names, name)
		}
	}
	writeNamedArchive(t, archivePath, names)

	if _, err := Verify(archivePath); err == nil || !strings.Contains(err.Error(), "archive entries missing") {
		t.Fatalf("Verify() error = %v, want archive entries missing", err)
	}
}

func TestVerifyRejectsNestedEntry(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "nested.zip")
	names := append(append([]string(nil), extensionbundle.Filenames...), "nested/debug.txt")
	writeNamedArchive(t, archivePath, names)

	if _, err := Verify(archivePath); err == nil || !strings.Contains(err.Error(), "unsafe archive entry") {
		t.Fatalf("Verify() error = %v, want unsafe archive entry", err)
	}
}

func TestVerifyRejectsDuplicateEntry(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "duplicate.zip")
	names := append(append([]string(nil), extensionbundle.Filenames...), "action.js")
	writeNamedArchive(t, archivePath, names)

	if _, err := Verify(archivePath); err == nil || !strings.Contains(err.Error(), "duplicate archive entry") {
		t.Fatalf("Verify() error = %v, want duplicate archive entry", err)
	}
}

func assertRootAllowlist(t *testing.T, archivePath string, want []string) {
	t.Helper()
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	got := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		if file.Name != filepath.Base(file.Name) {
			t.Fatalf("archive entry %q is not at the ZIP root", file.Name)
		}
		got = append(got, file.Name)
	}
	sort.Strings(got)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("archive entries = %v, want %v", got, want)
	}
}

func writeSyntheticArchive(t *testing.T, archivePath, unexpected string, replacements map[string][]byte) {
	t.Helper()
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, name := range extensionbundle.Filenames {
		contents, err := extensionbundle.Files.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		if replacement, ok := replacements[name]; ok {
			contents = replacement
		}
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(contents); err != nil {
			t.Fatal(err)
		}
	}
	if unexpected != "" {
		entry, err := writer.Create(unexpected)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(entry, "synthetic private payload"); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeNamedArchive(t *testing.T, archivePath string, names []string) {
	t.Helper()
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, name := range names {
		contents, err := extensionbundle.Files.ReadFile(name)
		if err != nil {
			contents = []byte("synthetic unexpected entry")
		}
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(contents); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
