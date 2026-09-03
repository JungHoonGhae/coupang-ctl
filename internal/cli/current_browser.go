package cli

import (
	"context"
	"errors"
	"io"

	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type currentBrowserStatusProvider interface {
	Status(context.Context) (core.CurrentBrowserStatus, error)
}

type nativeCurrentBrowserStatusProvider struct{}

func (nativeCurrentBrowserStatusProvider) Status(ctx context.Context) (core.CurrentBrowserStatus, error) {
	native := browser.NewNativeCurrentBrowser()
	defer native.Close()
	return native.CurrentBrowserStatus(ctx)
}

func runCurrentBrowser(ctx context.Context, args []string, stdout io.Writer, provider currentBrowserStatusProvider) error {
	if len(args) != 1 || args[0] != "status" {
		return errors.New("usage: coupangctl current-browser status")
	}
	status, err := provider.Status(ctx)
	if err != nil {
		return err
	}
	return writeJSON(stdout, status)
}
