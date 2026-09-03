//go:build !windows

package browser

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstalledBrowserMajorVersionRetriesOneTransientProbe(t *testing.T) {
	attempts := 0
	major, err := installedBrowserMajorVersionWithProbe(context.Background(), "synthetic-google-chrome", func(_ context.Context, executable string) ([]byte, error) {
		if executable != "synthetic-google-chrome" {
			t.Fatalf("executable = %q", executable)
		}
		attempts++
		if attempts == 1 {
			return nil, errors.New("synthetic transient probe failure")
		}
		return []byte("Google Chrome 152.0.0.0\n"), nil
	})
	if err != nil {
		t.Fatalf("transient version probe was not retried: %v", err)
	}
	if major != 152 || attempts != 2 {
		t.Fatalf("major = %d, attempts = %d", major, attempts)
	}
}

func TestIdentifyManagedBrowserClassifiesVersionProbeWithoutLeakingPath(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "google-chrome")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := identifyManagedBrowser(context.Background(), executable)
	if !errors.Is(err, ErrProfileIncompatible) || !strings.Contains(err.Error(), "browser version unavailable") {
		t.Fatalf("version-probe error = %v", err)
	}
	if strings.Contains(err.Error(), executable) {
		t.Fatalf("version-probe error leaked executable path: %v", err)
	}
}
