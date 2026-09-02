package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

type Paths struct {
	StateDir   string
	ProfileDir string
	Database   string
}

func DefaultPaths() (Paths, error) {
	root, err := stateRoot()
	if err != nil {
		return Paths{}, err
	}
	return Paths{
		StateDir:   root,
		ProfileDir: filepath.Join(root, "browser-profile"),
		Database:   filepath.Join(root, "coupangctl.sqlite3"),
	}, nil
}

func stateRoot() (string, error) {
	if override := os.Getenv("COUPANGCTL_STATE_DIR"); override != "" {
		if !filepath.IsAbs(override) {
			return "", fmt.Errorf("COUPANGCTL_STATE_DIR must be an absolute path")
		}
		return filepath.Clean(override), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "coupangctl"), nil
	case "windows":
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			return filepath.Join(local, "coupangctl"), nil
		}
		return "", fmt.Errorf("LOCALAPPDATA is not set")
	default:
		if state := os.Getenv("XDG_STATE_HOME"); state != "" {
			if !filepath.IsAbs(state) {
				return "", fmt.Errorf("XDG_STATE_HOME must be an absolute path")
			}
			return filepath.Join(state, "coupangctl"), nil
		}
		return filepath.Join(home, ".local", "state", "coupangctl"), nil
	}
}
