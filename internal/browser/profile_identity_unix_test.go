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

func TestIdentifyManagedBrowserRetriesOneTransientVersionProbe(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "google-chrome")
	script := `#!/bin/sh
counter="$0.count"
if [ ! -e "$counter" ]; then
  : >"$counter"
  exit 1
fi
printf '%s\n' 'Google Chrome 152.0.0.0'
`
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	identity, err := identifyManagedBrowser(context.Background(), executable)
	if err != nil {
		t.Fatalf("transient version probe was not retried: %v", err)
	}
	if identity.Family != "chrome" || identity.MajorVersion != 152 {
		t.Fatalf("identity = %#v", identity)
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
