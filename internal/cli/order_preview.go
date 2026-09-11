package cli

import (
	"context"
	"errors"
	"io"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

const orderPreviewUsage = "usage: coupangctl orders preview\nRead only the current source entry page through dedicated headless Camofox. No ledger read, cursor resume, or order persistence. This does not establish stable account identity or complete history."

var errOrderPreviewUsage = errors.New(orderPreviewUsage)

type orderPreviewProvider interface {
	Preview(context.Context) (core.OrderPreview, error)
}

func runOrderPreview(ctx context.Context, stdout io.Writer, provider orderPreviewProvider) error {
	result, err := provider.Preview(ctx)
	if err != nil {
		return err
	}
	return writeJSON(stdout, result)
}
