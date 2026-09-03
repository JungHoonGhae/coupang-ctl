package packagemanifests_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/packagemanifests"
)

func TestRenderProducesSourceFormulaAndPortableWinGetBundle(t *testing.T) {
	bundle, err := packagemanifests.Render(packagemanifests.Request{
		Tag:                "v1.2.3",
		SourceSHA256:       strings.Repeat("a", 64),
		WindowsAMD64SHA256: strings.Repeat("b", 64),
		WindowsARM64SHA256: strings.Repeat("c", 64),
	})
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	files := make(map[string]string, len(bundle.Files))
	for _, file := range bundle.Files {
		if _, duplicate := files[file.Path]; duplicate {
			t.Fatalf("duplicate generated path %q", file.Path)
		}
		files[file.Path] = file.Contents
	}
	if len(files) != 4 {
		t.Fatalf("generated file count = %d, want 4", len(files))
	}

	formula := files["homebrew/Formula/coupangctl.rb"]
	for _, expected := range []string{
		`class Coupangctl < Formula`,
		`url "https://github.com/JungHoonGhae/coupang-ctl/archive/refs/tags/v1.2.3.tar.gz"`,
		`sha256 "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`,
		`depends_on "go" => :build`,
		`std_go_args(ldflags: "-s -w -X main.version=#{version}")`,
		`"./cmd/coupangctl"`,
	} {
		if !strings.Contains(formula, expected) {
			t.Errorf("Homebrew formula missing %q\n%s", expected, formula)
		}
	}

	manifestRoot := "winget/manifests/j/JungHoonGhae/coupangctl/1.2.3/"
	versionManifest := files[manifestRoot+"JungHoonGhae.coupangctl.yaml"]
	if !strings.Contains(versionManifest, "DefaultLocale: ko-KR") || !strings.Contains(versionManifest, "ManifestVersion: 1.12.0") {
		t.Errorf("unexpected WinGet version manifest:\n%s", versionManifest)
	}
	installerManifest := files[manifestRoot+"JungHoonGhae.coupangctl.installer.yaml"]
	for _, expected := range []string{
		"InstallerType: zip",
		"NestedInstallerType: portable",
		"Scope: user",
		"Architecture: x64",
		"coupangctl_1.2.3_windows_amd64.zip",
		"InstallerSha256: BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB",
		"Architecture: arm64",
		"coupangctl_1.2.3_windows_arm64.zip",
		"InstallerSha256: CCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCCC",
		"RelativeFilePath: coupangctl.exe",
		"PortableCommandAlias: coupangctl",
		"ManifestVersion: 1.12.0",
	} {
		if !strings.Contains(installerManifest, expected) {
			t.Errorf("WinGet installer manifest missing %q\n%s", expected, installerManifest)
		}
	}
	localeManifest := files[manifestRoot+"JungHoonGhae.coupangctl.locale.ko-KR.yaml"]
	if !strings.Contains(localeManifest, "PackageLocale: ko-KR") || !strings.Contains(localeManifest, "License: MIT") {
		t.Errorf("unexpected WinGet locale manifest:\n%s", localeManifest)
	}
}

func TestWriteNewCreatesBundleOnceWithoutOverwriting(t *testing.T) {
	bundle, err := packagemanifests.Render(packagemanifests.Request{
		Tag:                "v1.2.3",
		SourceSHA256:       strings.Repeat("a", 64),
		WindowsAMD64SHA256: strings.Repeat("b", 64),
		WindowsARM64SHA256: strings.Repeat("c", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(t.TempDir(), "package-manifests")
	report, err := packagemanifests.WriteNew(outputDir, bundle)
	if err != nil {
		t.Fatalf("WriteNew() error = %v", err)
	}
	wantPaths := []string{
		"homebrew/Formula/coupangctl.rb",
		"winget/manifests/j/JungHoonGhae/coupangctl/1.2.3/JungHoonGhae.coupangctl.yaml",
		"winget/manifests/j/JungHoonGhae/coupangctl/1.2.3/JungHoonGhae.coupangctl.installer.yaml",
		"winget/manifests/j/JungHoonGhae/coupangctl/1.2.3/JungHoonGhae.coupangctl.locale.ko-KR.yaml",
	}
	if report.Tag != "v1.2.3" || report.Version != "1.2.3" || !reflect.DeepEqual(report.Files, wantPaths) {
		t.Fatalf("WriteNew() report = %#v", report)
	}
	for _, relativePath := range wantPaths {
		info, statErr := os.Lstat(filepath.Join(outputDir, filepath.FromSlash(relativePath)))
		if statErr != nil || !info.Mode().IsRegular() {
			t.Fatalf("generated file %q info = %v, error = %v", relativePath, info, statErr)
		}
	}

	marker := filepath.Join(outputDir, "keep.txt")
	if err := os.WriteFile(marker, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := packagemanifests.WriteNew(outputDir, bundle); err == nil {
		t.Fatal("WriteNew() overwrote an existing output directory")
	}
	contents, err := os.ReadFile(marker)
	if err != nil || string(contents) != "keep" {
		t.Fatalf("existing output changed: contents=%q error=%v", contents, err)
	}
}

func TestRenderRejectsMovingTagsAndMalformedHashes(t *testing.T) {
	valid := packagemanifests.Request{
		Tag:                "v1.2.3",
		SourceSHA256:       strings.Repeat("a", 64),
		WindowsAMD64SHA256: strings.Repeat("b", 64),
		WindowsARM64SHA256: strings.Repeat("c", 64),
	}

	movingTag := valid
	movingTag.Tag = "main"
	malformedHash := valid
	malformedHash.WindowsAMD64SHA256 = "not-a-sha256"
	for name, request := range map[string]packagemanifests.Request{
		"moving tag":     movingTag,
		"malformed hash": malformedHash,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := packagemanifests.Render(request); err == nil {
				t.Fatal("Render() accepted mutable or malformed release metadata")
			}
		})
	}
}

func TestWriteNewRejectsUnsafePathsAndCleansPartialOutput(t *testing.T) {
	parent := t.TempDir()
	outputDir := filepath.Join(parent, "package-manifests")
	bundle := packagemanifests.Bundle{
		Tag:     "v1.2.3",
		Version: "1.2.3",
		Files: []packagemanifests.File{
			{Path: "homebrew/Formula/coupangctl.rb", Contents: "safe first file"},
			{Path: "../outside", Contents: "must not escape"},
		},
	}

	if _, err := packagemanifests.WriteNew(outputDir, bundle); err == nil {
		t.Fatal("WriteNew() accepted a path outside the output directory")
	}
	if _, err := os.Lstat(outputDir); !os.IsNotExist(err) {
		t.Fatalf("partial output was not removed: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(parent, "outside")); !os.IsNotExist(err) {
		t.Fatalf("unsafe file escaped output directory: %v", err)
	}
}
