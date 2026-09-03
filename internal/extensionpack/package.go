// Package extensionpack builds and verifies the exact Chrome Web Store ZIP
// represented by the reviewed extension bundle embedded in coupangctl.
package extensionpack

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	extensionbundle "github.com/JungHoonGhae/coupang-ctl/extension"
)

const packageKind = "chrome_web_store_extension"

var fixedModificationTime = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)

// Report is the stable, machine-readable result of building or verifying an
// extension ZIP. It intentionally contains no browser or account data.
type Report struct {
	SchemaVersion int      `json:"schema_version"`
	Kind          string   `json:"kind"`
	OutputPath    string   `json:"output_path"`
	FileCount     int      `json:"file_count"`
	Files         []string `json:"files"`
	SizeBytes     int64    `json:"size_bytes"`
	SHA256        string   `json:"sha256"`
}

// Build writes a deterministic, root-only ZIP from the reviewed embedded
// extension. Existing output is never overwritten and a partial ZIP is removed
// if validation fails.
func Build(outputPath string) (_ Report, returnErr error) {
	files, err := reviewedBundle()
	if err != nil {
		return Report{}, err
	}
	absolutePath, err := cleanZIPPath(outputPath)
	if err != nil {
		return Report{}, err
	}

	output, err := os.OpenFile(absolutePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return Report{}, fmt.Errorf("extension package already exists: %s", absolutePath)
		}
		return Report{}, fmt.Errorf("create extension package: %w", err)
	}
	keep := false
	defer func() {
		_ = output.Close()
		if !keep {
			_ = os.Remove(absolutePath)
		}
	}()

	writer := zip.NewWriter(output)
	for _, name := range sortedNames(files) {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetModTime(fixedModificationTime)
		header.SetMode(0o644)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			_ = writer.Close()
			return Report{}, fmt.Errorf("create ZIP entry %s: %w", name, err)
		}
		if _, err := entry.Write(files[name]); err != nil {
			_ = writer.Close()
			return Report{}, fmt.Errorf("write ZIP entry %s: %w", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		return Report{}, fmt.Errorf("finish extension package: %w", err)
	}
	if err := output.Sync(); err != nil {
		return Report{}, fmt.Errorf("sync extension package: %w", err)
	}
	if err := output.Close(); err != nil {
		return Report{}, fmt.Errorf("close extension package: %w", err)
	}

	report, err := Verify(absolutePath)
	if err != nil {
		return Report{}, fmt.Errorf("verify built extension package: %w", err)
	}
	keep = true
	return report, nil
}

// Verify proves that a ZIP contains exactly the reviewed embedded extension at
// its root, with no extra, missing, nested, duplicated, or modified entries.
func Verify(archivePath string) (Report, error) {
	files, err := reviewedBundle()
	if err != nil {
		return Report{}, err
	}
	absolutePath, err := cleanZIPPath(archivePath)
	if err != nil {
		return Report{}, err
	}
	info, err := os.Lstat(absolutePath)
	if err != nil {
		return Report{}, fmt.Errorf("inspect extension package: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Report{}, fmt.Errorf("extension package is not a regular file: %s", absolutePath)
	}

	reader, err := zip.OpenReader(absolutePath)
	if err != nil {
		return Report{}, fmt.Errorf("open extension package: %w", err)
	}
	defer reader.Close()

	seen := make(map[string]struct{}, len(reader.File))
	for _, entry := range reader.File {
		name := entry.Name
		if name != path.Base(name) || name == "." || name == ".." {
			return Report{}, fmt.Errorf("unsafe archive entry: %q", name)
		}
		want, allowed := files[name]
		if !allowed {
			return Report{}, fmt.Errorf("unexpected archive entry: %q", name)
		}
		if _, duplicate := seen[name]; duplicate {
			return Report{}, fmt.Errorf("duplicate archive entry: %q", name)
		}
		if entry.FileInfo().IsDir() || !entry.Mode().IsRegular() {
			return Report{}, fmt.Errorf("archive entry is not a regular file: %q", name)
		}
		contents, err := readLimitedEntry(entry, int64(len(want))+1)
		if err != nil {
			return Report{}, fmt.Errorf("read archive entry %s: %w", name, err)
		}
		if !bytes.Equal(contents, want) {
			return Report{}, fmt.Errorf("archive entry content mismatch: %q", name)
		}
		seen[name] = struct{}{}
	}
	if len(seen) != len(files) {
		missing := make([]string, 0, len(files)-len(seen))
		for name := range files {
			if _, ok := seen[name]; !ok {
				missing = append(missing, name)
			}
		}
		sort.Strings(missing)
		return Report{}, fmt.Errorf("archive entries missing: %v", missing)
	}

	digest, err := fileSHA256(absolutePath)
	if err != nil {
		return Report{}, fmt.Errorf("hash extension package: %w", err)
	}
	return Report{
		SchemaVersion: 1,
		Kind:          packageKind,
		OutputPath:    absolutePath,
		FileCount:     len(files),
		Files:         sortedNames(files),
		SizeBytes:     info.Size(),
		SHA256:        digest,
	}, nil
}

func reviewedBundle() (map[string][]byte, error) {
	files := make(map[string][]byte, len(extensionbundle.Filenames))
	for _, name := range extensionbundle.Filenames {
		if name != path.Base(name) || name == "." || name == ".." || strings.Contains(name, "\\") {
			return nil, fmt.Errorf("unsafe embedded extension filename: %q", name)
		}
		if _, duplicate := files[name]; duplicate {
			return nil, fmt.Errorf("duplicate embedded extension filename: %q", name)
		}
		contents, err := extensionbundle.Files.ReadFile(name)
		if err != nil {
			return nil, fmt.Errorf("read embedded extension file %s: %w", name, err)
		}
		files[name] = contents
	}

	manifestBytes, ok := files["manifest.json"]
	if !ok {
		return nil, fmt.Errorf("embedded extension is missing manifest.json")
	}
	var manifest struct {
		Icons  map[string]string `json:"icons"`
		Action struct {
			DefaultIcon map[string]string `json:"default_icon"`
		} `json:"action"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("parse embedded extension manifest: %w", err)
	}
	for size, filename := range map[string]string{"16": "icon16.png", "48": "icon48.png", "128": "icon128.png"} {
		if manifest.Icons[size] != filename || manifest.Action.DefaultIcon[size] != filename {
			return nil, fmt.Errorf("embedded extension manifest icon %s must reference %s", size, filename)
		}
		if _, ok := files[filename]; !ok {
			return nil, fmt.Errorf("embedded extension is missing %s", filename)
		}
	}
	return files, nil
}

func cleanZIPPath(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("extension package path is required")
	}
	if !strings.EqualFold(filepath.Ext(value), ".zip") {
		return "", fmt.Errorf("extension package path must end in .zip")
	}
	absolutePath, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve extension package path: %w", err)
	}
	return filepath.Clean(absolutePath), nil
}

func sortedNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func readLimitedEntry(entry *zip.File, limit int64) ([]byte, error) {
	reader, err := entry.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(io.LimitReader(reader, limit))
}

func fileSHA256(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
