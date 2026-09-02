package recap

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/JungHoonGhae/oss-coupangctl/internal/core"
)

func WriteNewFile(path string, summary core.ShoppingInsights) (core.RecapWriteResult, error) {
	return WriteNewFileWithOptions(path, summary, Options{})
}

func WriteNewFileWithOptions(path string, summary core.ShoppingInsights, options Options) (core.RecapWriteResult, error) {
	if strings.TrimSpace(path) == "" || !strings.EqualFold(filepath.Ext(path), ".html") {
		return core.RecapWriteResult{}, errors.New("recap output must be a new .html file")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return core.RecapWriteResult{}, errors.New("create private recap output")
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(path)
		}
	}()
	if err := RenderWithOptions(file, summary, options); err != nil {
		_ = file.Close()
		return core.RecapWriteResult{}, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return core.RecapWriteResult{}, errors.New("sync private recap output")
	}
	if err := file.Close(); err != nil {
		return core.RecapWriteResult{}, errors.New("close private recap output")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return core.RecapWriteResult{}, errors.New("secure private recap output")
	}
	info, err := os.Stat(path)
	if err != nil {
		return core.RecapWriteResult{}, errors.New("inspect private recap output")
	}
	committed = true
	visibility := "public_safe"
	if options.Products != nil {
		visibility = "private_products"
	}
	return core.RecapWriteResult{Written: true, Format: "html", Visibility: visibility, Bytes: info.Size()}, nil
}
