package browser

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseBrowserMajorVersionAcceptsVendorOutputOnly(t *testing.T) {
	for _, test := range []struct {
		output string
		want   int
	}{
		{"Google Chrome 152.0.7977.66", 152},
		{"Microsoft Edge 144.1.2.3", 144},
		{"Chromium 151.0.0.0 snap", 151},
	} {
		got, err := parseBrowserMajorVersion(test.output)
		if err != nil || got != test.want {
			t.Fatalf("parseBrowserMajorVersion(%q) = %d, %v; want %d", test.output, got, err, test.want)
		}
	}
	for _, output := range []string{"", "Google Chrome", "version 0.1", "version 10000.1"} {
		if _, err := parseBrowserMajorVersion(output); !errors.Is(err, ErrProfileIncompatible) {
			t.Fatalf("parseBrowserMajorVersion(%q) error = %v", output, err)
		}
	}
}

func TestBrowserFamilyUsesExecutableIdentityWithoutPersistingPath(t *testing.T) {
	for executable, want := range map[string]string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome": "chrome",
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`:       "edge",
		"/usr/bin/chromium": "chromium",
	} {
		got, err := browserFamily(executable)
		if err != nil || got != want {
			t.Fatalf("browserFamily(%q) = %q, %v; want %q", executable, got, err, want)
		}
	}
	if _, err := browserFamily("/synthetic/unknown"); !errors.Is(err, ErrProfileIncompatible) {
		t.Fatalf("unknown browser family error = %v", err)
	}
}

func TestProfileIdentityAllowsUpgradeAndRejectsFamilyChangeOrDowngrade(t *testing.T) {
	profileDir := t.TempDir()
	if runtime.GOOS != "windows" {
		if err := os.Chmod(profileDir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	chrome151 := profileIdentity{SchemaVersion: 1, Family: "chrome", MajorVersion: 151}
	if err := validateOrUpdateProfileIdentity(profileDir, chrome151); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(profileDir, profileIdentityFilename)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != `{"schema_version":1,"family":"chrome","major_version":151}` {
		t.Fatalf("unexpected profile identity content: %q", content)
	}
	chrome152 := profileIdentity{SchemaVersion: 1, Family: "chrome", MajorVersion: 152}
	if err := validateOrUpdateProfileIdentity(profileDir, chrome152); err != nil {
		t.Fatalf("profile upgrade rejected: %v", err)
	}
	for _, identity := range []profileIdentity{
		{SchemaVersion: 1, Family: "edge", MajorVersion: 152},
		{SchemaVersion: 1, Family: "chrome", MajorVersion: 151},
	} {
		if err := validateOrUpdateProfileIdentity(profileDir, identity); !errors.Is(err, ErrProfileIncompatible) {
			t.Fatalf("incompatible profile identity %#v error = %v", identity, err)
		}
	}
}

func TestProfileIdentityRejectsSymlink(t *testing.T) {
	profileDir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside-identity")
	if err := os.WriteFile(outside, []byte(`{"schema_version":1,"family":"chrome","major_version":152}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(profileDir, profileIdentityFilename)); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink unavailable: %v", err)
		}
		t.Fatal(err)
	}
	identity := profileIdentity{SchemaVersion: 1, Family: "chrome", MajorVersion: 152}
	if err := validateOrUpdateProfileIdentity(profileDir, identity); !errors.Is(err, ErrProfileIncompatible) {
		t.Fatalf("symlink identity error = %v", err)
	}
}

func TestProfileIdentityRejectsUnknownOrTrailingContent(t *testing.T) {
	identity := profileIdentity{SchemaVersion: 1, Family: "chrome", MajorVersion: 152}
	for _, content := range []string{
		`{"schema_version":1,"family":"chrome","major_version":152,"path":"sensitive"}`,
		`{"schema_version":1,"family":"chrome","major_version":152}{}`,
	} {
		profileDir := t.TempDir()
		path := filepath.Join(profileDir, profileIdentityFilename)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" {
			if err := os.Chmod(path, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if err := validateOrUpdateProfileIdentity(profileDir, identity); !errors.Is(err, ErrProfileIncompatible) {
			t.Fatalf("profile identity content %q error = %v", content, err)
		}
	}
}
