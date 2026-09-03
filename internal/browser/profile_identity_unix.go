//go:build !windows

package browser

import (
	"context"
	"os/exec"
	"time"
)

type browserVersionProbe func(context.Context, string) ([]byte, error)

func installedBrowserMajorVersion(ctx context.Context, executable string) (int, error) {
	return installedBrowserMajorVersionWithProbe(ctx, executable, func(probeCtx context.Context, path string) ([]byte, error) {
		versionCtx, cancel := context.WithTimeout(probeCtx, 3*time.Second)
		defer cancel()
		return exec.CommandContext(versionCtx, path, "--version").CombinedOutput()
	})
}

func installedBrowserMajorVersionWithProbe(ctx context.Context, executable string, probe browserVersionProbe) (int, error) {
	for attempt := 0; attempt < 2; attempt++ {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		output, err := probe(ctx, executable)
		if err == nil {
			if major, parseErr := parseBrowserMajorVersion(string(output)); parseErr == nil {
				return major, nil
			}
		}
		if err := ctx.Err(); err != nil {
			return 0, err
		}
	}
	return 0, ErrProfileIncompatible
}
