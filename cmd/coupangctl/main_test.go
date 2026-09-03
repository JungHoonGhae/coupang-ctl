package main

import (
	"runtime/debug"
	"testing"
)

func TestResolvedVersionPrefersReleaseLinkerValue(t *testing.T) {
	info := &debug.BuildInfo{Main: debug.Module{Version: "v0.0.0-20260903-synthetic"}}
	if got := resolvedVersion("v1.2.3", info, true); got != "v1.2.3" {
		t.Fatalf("resolvedVersion() = %q, want linker release version", got)
	}
}

func TestResolvedVersionUsesGoInstallModuleVersion(t *testing.T) {
	info := &debug.BuildInfo{Main: debug.Module{Version: "v0.0.0-20260903-synthetic"}}
	if got := resolvedVersion("dev", info, true); got != info.Main.Version {
		t.Fatalf("resolvedVersion() = %q, want module version %q", got, info.Main.Version)
	}
}

func TestResolvedVersionKeepsDevForLocalBuild(t *testing.T) {
	for _, test := range []struct {
		name string
		info *debug.BuildInfo
		ok   bool
	}{
		{name: "missing build info"},
		{name: "local module", info: &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, ok: true},
		{name: "empty module", info: &debug.BuildInfo{Main: debug.Module{}}, ok: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := resolvedVersion("dev", test.info, test.ok); got != "dev" {
				t.Fatalf("resolvedVersion() = %q, want dev", got)
			}
		})
	}
}
