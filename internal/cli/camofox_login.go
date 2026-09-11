package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func camofoxLoginRequest(args []string, stderr io.Writer) (core.LoginRequest, error) {
	request := core.LoginRequest{Mode: core.LoginModeQR}
	seen := map[string]bool{}
	for _, arg := range args {
		if seen[arg] || (arg != "--qr" && arg != "--link" && arg != "--manual") {
			return request, browser.ErrCamofoxUnsupported
		}
		seen[arg] = true
	}
	if seen["--manual"] && (seen["--qr"] || seen["--link"]) {
		return request, browser.ErrCamofoxUnsupported
	}
	if seen["--link"] {
		request.PresentQRLink = func(_ context.Context, link core.QRLoginLink) error {
			_, err := fmt.Fprintf(stderr, "Ephemeral Coupang QR login link (do not share):\n%s\nApproval number: %s\n", link.URL, link.ApprovalCode)
			return err
		}
	}
	return request, nil
}
