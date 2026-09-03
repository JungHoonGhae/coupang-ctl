package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const (
	profileIdentityFilename = ".coupangctl-browser.json"
	profileIdentitySchema   = 1
	maxProfileIdentityBytes = 1024
)

var ErrProfileIncompatible = errors.New("dedicated browser profile is incompatible with the selected browser")

type profileIdentity struct {
	SchemaVersion int    `json:"schema_version"`
	Family        string `json:"family"`
	MajorVersion  int    `json:"major_version"`
}

func identifyManagedBrowser(ctx context.Context, executable string) (profileIdentity, error) {
	family, err := browserFamily(executable)
	if err != nil {
		return profileIdentity{}, ErrProfileIncompatible
	}
	major, err := installedBrowserMajorVersion(ctx, executable)
	if err != nil || major < 1 {
		return profileIdentity{}, ErrProfileIncompatible
	}
	return profileIdentity{SchemaVersion: profileIdentitySchema, Family: family, MajorVersion: major}, nil
}

func validateOrUpdateProfileIdentity(profileDir string, current profileIdentity) error {
	if !validProfileIdentity(current) {
		return ErrProfileIncompatible
	}
	path := filepath.Join(profileDir, profileIdentityFilename)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return writeProfileIdentity(path, current)
	}
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxProfileIdentityBytes || (runtime.GOOS != "windows" && info.Mode().Perm() != 0o600) {
		return ErrProfileIncompatible
	}
	content, err := os.ReadFile(path)
	if err != nil || int64(len(content)) != info.Size() {
		return ErrProfileIncompatible
	}
	var recorded profileIdentity
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&recorded); err != nil || !validProfileIdentity(recorded) {
		return ErrProfileIncompatible
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ErrProfileIncompatible
	}
	if recorded.Family != current.Family || current.MajorVersion < recorded.MajorVersion {
		return ErrProfileIncompatible
	}
	if current.MajorVersion == recorded.MajorVersion {
		return nil
	}
	return writeProfileIdentity(path, current)
}

func writeProfileIdentity(path string, identity profileIdentity) error {
	content, err := json.Marshal(identity)
	if err != nil || len(content) > maxProfileIdentityBytes {
		return ErrProfileIncompatible
	}
	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".coupangctl-browser-*.tmp")
	if err != nil {
		return fmt.Errorf("create browser profile identity: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure browser profile identity: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write browser profile identity: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync browser profile identity: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close browser profile identity: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("commit browser profile identity: %w", err)
	}
	return nil
}

func validProfileIdentity(identity profileIdentity) bool {
	return identity.SchemaVersion == profileIdentitySchema &&
		(identity.Family == "chrome" || identity.Family == "edge" || identity.Family == "chromium") &&
		identity.MajorVersion >= 1 && identity.MajorVersion <= 9999
}

func browserFamily(executable string) (string, error) {
	normalized := strings.ToLower(strings.ReplaceAll(executable, "\\", "/"))
	switch {
	case strings.Contains(normalized, "google chrome") || strings.Contains(normalized, "google/chrome") || strings.Contains(normalized, "google-chrome"):
		return "chrome", nil
	case strings.Contains(normalized, "microsoft edge") || strings.Contains(normalized, "microsoft/edge") || strings.Contains(normalized, "microsoft-edge") || strings.HasSuffix(normalized, "/msedge.exe"):
		return "edge", nil
	case strings.Contains(normalized, "chromium"):
		return "chromium", nil
	default:
		return "", ErrProfileIncompatible
	}
}

func parseBrowserMajorVersion(output string) (int, error) {
	for _, field := range strings.Fields(output) {
		candidate := strings.Trim(field, "()[],:;vV")
		parts := strings.Split(candidate, ".")
		if len(parts) < 2 {
			continue
		}
		major, err := strconv.Atoi(parts[0])
		if err == nil && major >= 1 && major <= 9999 {
			return major, nil
		}
	}
	return 0, ErrProfileIncompatible
}
