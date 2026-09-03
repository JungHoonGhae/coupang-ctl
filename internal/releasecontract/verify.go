package releasecontract

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var archiveNamePattern = regexp.MustCompile(`^coupangctl_.+_(darwin|linux|windows)_(amd64|arm64)(\.tar\.gz|\.zip)$`)

var expectedTargets = map[string]string{
	"darwin/amd64":  ".tar.gz",
	"darwin/arm64":  ".tar.gz",
	"linux/amd64":   ".tar.gz",
	"linux/arm64":   ".tar.gz",
	"windows/amd64": ".zip",
	"windows/arm64": ".zip",
}

type Options struct {
	RequireSBOM bool
}

// Verify checks that a GoReleaser dist directory has the complete supported
// target set, only public allowlisted archive entries, and matching SHA-256
// checksums. It never opens or interprets the executable payload.
func Verify(distDir string) error {
	return VerifyWithOptions(distDir, Options{})
}

// VerifyWithOptions applies the release contract and can additionally require
// one checksummed SBOM for every platform archive.
func VerifyWithOptions(distDir string, options Options) error {
	entries, err := os.ReadDir(distDir)
	if err != nil {
		return fmt.Errorf("read release directory: %w", err)
	}

	archives := make(map[string]string, len(expectedTargets))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		matches := archiveNamePattern.FindStringSubmatch(entry.Name())
		if matches == nil {
			continue
		}
		target := matches[1] + "/" + matches[2]
		extension := matches[3]
		expectedExtension, supported := expectedTargets[target]
		if !supported || extension != expectedExtension {
			return fmt.Errorf("unsupported release target or format: %s", entry.Name())
		}
		if _, duplicate := archives[target]; duplicate {
			return fmt.Errorf("duplicate release target: %s", target)
		}
		archives[target] = filepath.Join(distDir, entry.Name())
	}

	if got, want := sortedKeys(archives), sortedKeys(expectedTargets); strings.Join(got, ",") != strings.Join(want, ",") {
		return fmt.Errorf("release target set = %v, want %v", got, want)
	}

	allowedArtifacts := make(map[string]string, len(archives)*2)
	sbomCount := 0
	for _, archivePath := range archives {
		allowedArtifacts[filepath.Base(archivePath)] = archivePath
		sbomPath := archivePath + ".sbom.json"
		info, statErr := os.Stat(sbomPath)
		switch {
		case statErr == nil:
			if !info.Mode().IsRegular() {
				return fmt.Errorf("SBOM is not a regular file: %s", filepath.Base(sbomPath))
			}
			sbomCount++
			allowedArtifacts[filepath.Base(sbomPath)] = sbomPath
		case os.IsNotExist(statErr):
		case statErr != nil:
			return fmt.Errorf("inspect SBOM %s: %w", filepath.Base(sbomPath), statErr)
		}
	}
	if sbomCount != 0 && sbomCount != len(archives) {
		return fmt.Errorf("SBOM set has %d entries, want either 0 or %d", sbomCount, len(archives))
	}
	if options.RequireSBOM && sbomCount != len(archives) {
		return fmt.Errorf("SBOM set has %d entries, want %d", sbomCount, len(archives))
	}

	checksums, err := readChecksums(filepath.Join(distDir, "checksums.txt"))
	if err != nil {
		return err
	}
	for name := range checksums {
		if _, allowed := allowedArtifacts[name]; !allowed {
			return fmt.Errorf("unexpected checksummed artifact: %s", name)
		}
	}
	if len(checksums) != len(allowedArtifacts) {
		return fmt.Errorf("checksummed artifact set has %d entries, want %d", len(checksums), len(allowedArtifacts))
	}
	for name, artifactPath := range allowedArtifacts {
		wantChecksum, ok := checksums[name]
		if !ok {
			return fmt.Errorf("checksum missing for %s", name)
		}
		gotChecksum, err := fileSHA256(artifactPath)
		if err != nil {
			return fmt.Errorf("checksum %s: %w", name, err)
		}
		if gotChecksum != wantChecksum {
			return fmt.Errorf("checksum mismatch for %s", name)
		}
	}
	for target, archivePath := range archives {
		name := filepath.Base(archivePath)
		if err := verifyArchive(archivePath, strings.HasPrefix(target, "windows/")); err != nil {
			return fmt.Errorf("verify %s: %w", name, err)
		}
	}
	return nil
}

func verifyArchive(archivePath string, windows bool) error {
	var (
		entries []archiveEntry
		err     error
	)
	if windows {
		entries, err = zipEntries(archivePath)
	} else {
		entries, err = tarGzipEntries(archivePath)
	}
	if err != nil {
		return err
	}

	binary := "coupangctl"
	if windows {
		binary += ".exe"
	}
	want := map[string]bool{
		binary:              true,
		"LICENSE":           false,
		"README.md":         false,
		"BROWSER_BRIDGE.md": false,
	}
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if entry.name != path.Base(entry.name) || entry.name == "." || entry.name == ".." {
			return fmt.Errorf("unsafe archive entry: %q", entry.name)
		}
		executable, allowed := want[entry.name]
		if !allowed {
			return fmt.Errorf("unexpected archive entry: %q", entry.name)
		}
		if !entry.regular {
			return fmt.Errorf("archive entry is not a regular file: %q", entry.name)
		}
		if _, duplicate := seen[entry.name]; duplicate {
			return fmt.Errorf("duplicate archive entry: %q", entry.name)
		}
		if executable && !windows && entry.mode&0o111 == 0 {
			return fmt.Errorf("archive executable lacks execute permission")
		}
		seen[entry.name] = struct{}{}
	}
	if len(seen) != len(want) {
		missing := make([]string, 0)
		for name := range want {
			if _, ok := seen[name]; !ok {
				missing = append(missing, name)
			}
		}
		sort.Strings(missing)
		return fmt.Errorf("archive entries missing: %v", missing)
	}
	return nil
}

type archiveEntry struct {
	name    string
	mode    os.FileMode
	regular bool
}

func tarGzipEntries(archivePath string) ([]archiveEntry, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	var entries []archiveEntry
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		entries = append(entries, archiveEntry{
			name:    header.Name,
			mode:    header.FileInfo().Mode(),
			regular: header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA,
		})
	}
	return entries, nil
}

func zipEntries(archivePath string) ([]archiveEntry, error) {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	entries := make([]archiveEntry, 0, len(reader.File))
	for _, file := range reader.File {
		entries = append(entries, archiveEntry{name: file.Name, mode: file.Mode(), regular: !file.FileInfo().IsDir() && file.Mode().IsRegular()})
	}
	return entries, nil
}

func readChecksums(checksumPath string) (map[string]string, error) {
	contents, err := os.ReadFile(checksumPath)
	if err != nil {
		return nil, fmt.Errorf("read checksums: %w", err)
	}
	checksums := make(map[string]string)
	for lineNumber, line := range strings.Split(strings.TrimSpace(string(contents)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return nil, fmt.Errorf("invalid checksum line %d", lineNumber+1)
		}
		digest := strings.ToLower(fields[0])
		if decoded, err := hex.DecodeString(digest); err != nil || len(decoded) != sha256.Size {
			return nil, fmt.Errorf("invalid SHA-256 on checksum line %d", lineNumber+1)
		}
		name := strings.TrimPrefix(fields[1], "*")
		if name != filepath.Base(name) {
			return nil, fmt.Errorf("unsafe checksum filename on line %d", lineNumber+1)
		}
		if _, duplicate := checksums[name]; duplicate {
			return nil, fmt.Errorf("duplicate checksum entry for %s", name)
		}
		checksums[name] = digest
	}
	return checksums, nil
}

func fileSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
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

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
