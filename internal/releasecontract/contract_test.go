package releasecontract_test

import (
	"reflect"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/releasecontract"
)

func TestResolveReturnsCanonicalReleaseArtifact(t *testing.T) {
	got, err := releasecontract.Resolve("v0.1.0-rc.1", "darwin", "arm64")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	want := releasecontract.Artifact{
		Tag:              "v0.1.0-rc.1",
		Version:          "0.1.0-rc.1",
		GOOS:             "darwin",
		GOARCH:           "arm64",
		ArchiveFormat:    "tar.gz",
		ArchiveName:      "coupangctl_0.1.0-rc.1_darwin_arm64.tar.gz",
		ExecutableName:   "coupangctl",
		ReleaseAssetPath: "v0.1.0-rc.1/coupangctl_0.1.0-rc.1_darwin_arm64.tar.gz",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Resolve() = %#v, want %#v", got, want)
	}
}

func TestArtifactsReturnsTheExactSupportedReleaseSet(t *testing.T) {
	got, err := releasecontract.Artifacts("v1.2.3")
	if err != nil {
		t.Fatalf("Artifacts() error = %v", err)
	}
	want := []string{
		"coupangctl_1.2.3_darwin_amd64.tar.gz",
		"coupangctl_1.2.3_darwin_arm64.tar.gz",
		"coupangctl_1.2.3_linux_amd64.tar.gz",
		"coupangctl_1.2.3_linux_arm64.tar.gz",
		"coupangctl_1.2.3_windows_amd64.zip",
		"coupangctl_1.2.3_windows_arm64.zip",
	}
	if len(got) != len(want) {
		t.Fatalf("Artifacts() count = %d, want %d", len(got), len(want))
	}
	for index := range want {
		if got[index].ArchiveName != want[index] {
			t.Fatalf("Artifacts()[%d].ArchiveName = %q, want %q", index, got[index].ArchiveName, want[index])
		}
	}
}
