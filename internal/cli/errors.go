package cli

import (
	"errors"
	"io"
	"strings"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func WriteError(w io.Writer, err error) {
	writeCommandFailure(w, "unknown", err)
}

func WriteCommandError(w io.Writer, args []string, err error) {
	writeCommandFailure(w, commandOperation(args), err)
}

func writeCommandFailure(w io.Writer, operation string, err error) {
	if errors.Is(err, errOrderPreviewUsage) || errors.Is(err, core.ErrInvalidLoginMode) || errors.Is(err, core.ErrInvalidLoginRequest) {
		err = core.WithErrorCode("invalid_command", err)
	}
	_ = writeJSON(w, core.PublicError(operation, err))
}

// Canonical names agree with MCP. Only fixed command words are accepted;
// request flags and values never become public operation metadata.
func commandOperation(args []string) string {
	args = expandConvenienceCommand(args)
	if len(args) == 0 {
		return "unknown"
	}
	if args[0] == "doctor" {
		return "doctor"
	}
	if len(args) < 2 {
		return "unknown"
	}
	key := args[0] + " " + args[1]
	switch key {
	case "products inspect":
		return "product_inspect"
	case "products price-history":
		return "product_price_history"
	case "products watch-list":
		return "product_watchlist"
	case "products watch-add", "products watch-remove", "products watch-refresh":
		return "product_" + strings.ReplaceAll(args[1], "-", "_")
	case "products cart-add":
		return "cart_add"
	case "products report":
		return "products_report_render"
	case "orders products":
		return "orders_product_insights"
	case "orders reorder":
		return "orders_reorder_candidates"
	case "orders categories":
		return "orders_enrich_categories"
	case "auth ensure":
		return "auth_login_if_needed"
	default:
		return core.KnownOperation(strings.NewReplacer(" ", "_", "-", "_").Replace(key))
	}
}
