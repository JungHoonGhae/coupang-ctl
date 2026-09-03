//go:build !windows

package browser

import (
	"context"
	"os/exec"
	"time"
)

func installedBrowserMajorVersion(ctx context.Context, executable string) (int, error) {
	versionCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(versionCtx, executable, "--version").CombinedOutput()
	if err != nil {
		return 0, ErrProfileIncompatible
	}
	return parseBrowserMajorVersion(string(output))
}
