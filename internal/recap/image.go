package recap

import (
	"context"
	"errors"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

const maxShareImageBytes = 25 << 20

type LocalHTMLPNGRenderer interface {
	RenderPNG(context.Context, string, string, int, int) error
}

func WritePublicShareImage(ctx context.Context, path string, summary core.ShoppingInsights, renderer LocalHTMLPNGRenderer) (core.RecapImageWriteResult, error) {
	preview := PublicSharePreview(summary)
	if strings.TrimSpace(path) == "" || !strings.EqualFold(filepath.Ext(path), ".png") {
		return core.RecapImageWriteResult{}, errors.New("recap image output must be a new .png file")
	}
	if renderer == nil {
		return core.RecapImageWriteResult{}, errors.New("recap image renderer unavailable")
	}
	temporaryDirectory, err := os.MkdirTemp("", "coupangctl-recap-image-")
	if err != nil {
		return core.RecapImageWriteResult{}, errors.New("create private recap image workspace")
	}
	defer os.RemoveAll(temporaryDirectory)
	if err := os.Chmod(temporaryDirectory, 0o700); err != nil {
		return core.RecapImageWriteResult{}, errors.New("secure private recap image workspace")
	}
	htmlPath := filepath.Join(temporaryDirectory, "share-card.html")
	htmlFile, err := os.OpenFile(htmlPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return core.RecapImageWriteResult{}, errors.New("create private recap image source")
	}
	if err := RenderPublicShareCard(htmlFile, summary); err != nil {
		_ = htmlFile.Close()
		return core.RecapImageWriteResult{}, err
	}
	if err := htmlFile.Sync(); err != nil {
		_ = htmlFile.Close()
		return core.RecapImageWriteResult{}, errors.New("sync private recap image source")
	}
	if err := htmlFile.Close(); err != nil {
		return core.RecapImageWriteResult{}, errors.New("close private recap image source")
	}

	renderedPath := filepath.Join(temporaryDirectory, "share-card.png")
	if err := renderer.RenderPNG(ctx, htmlPath, renderedPath, ShareImageWidth, ShareImageHeight); err != nil {
		return core.RecapImageWriteResult{}, err
	}
	rendered, err := os.Open(renderedPath)
	if err != nil {
		return core.RecapImageWriteResult{}, errors.New("open rendered recap image")
	}
	configuration, err := png.DecodeConfig(io.LimitReader(rendered, maxShareImageBytes+1))
	closeErr := rendered.Close()
	if err != nil || closeErr != nil || configuration.Width != ShareImageWidth || configuration.Height != ShareImageHeight {
		return core.RecapImageWriteResult{}, errors.New("validate rendered recap image")
	}

	rendered, err = os.Open(renderedPath)
	if err != nil {
		return core.RecapImageWriteResult{}, errors.New("reopen rendered recap image")
	}
	defer rendered.Close()
	output, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return core.RecapImageWriteResult{}, errors.New("create private recap image output")
	}
	committed := false
	defer func() {
		if !committed {
			_ = output.Close()
			_ = os.Remove(path)
		}
	}()
	written, err := io.Copy(output, io.LimitReader(rendered, maxShareImageBytes+1))
	if err != nil || written < 1 || written > maxShareImageBytes {
		return core.RecapImageWriteResult{}, errors.New("write private recap image output")
	}
	if err := output.Sync(); err != nil {
		return core.RecapImageWriteResult{}, errors.New("sync private recap image output")
	}
	if err := output.Close(); err != nil {
		return core.RecapImageWriteResult{}, errors.New("close private recap image output")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return core.RecapImageWriteResult{}, errors.New("secure private recap image output")
	}
	committed = true
	return core.RecapImageWriteResult{
		Written: true, Format: "png", Visibility: "public_safe",
		Width: ShareImageWidth, Height: ShareImageHeight, Bytes: written, Preview: preview,
	}, nil
}
