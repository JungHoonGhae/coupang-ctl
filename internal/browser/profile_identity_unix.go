//go:build !windows

package browser

import (
	"context"
	"os/exec"
	"time"
)

func installedBrowserMajorVersion(ctx context.Context, executable string) (int, error) {
	for attempt := 0; attempt < 2; attempt++ {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		versionCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		output, err := exec.CommandContext(versionCtx, executable, "--version").CombinedOutput()
		cancel()
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
