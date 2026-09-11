package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/JungHoonGhae/coupang-ctl/internal/account"
	"github.com/JungHoonGhae/coupang-ctl/internal/auth"
	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	coupangproducts "github.com/JungHoonGhae/coupang-ctl/internal/coupang/products"
	"github.com/JungHoonGhae/coupang-ctl/internal/orders"
	"github.com/JungHoonGhae/coupang-ctl/internal/partners"
	"github.com/JungHoonGhae/coupang-ctl/internal/platform"
	"github.com/JungHoonGhae/coupang-ctl/internal/products"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
)

func explicitCamofox(args []string) bool {
	for _, arg := range args {
		if arg == "--camofox" {
			return true
		}
	}
	return false
}

// Local evidence commands never inspect or start a browser. Configuration is
// required only at the source-read seam, not to read the isolated ledger.
func camofoxNeedsRuntime(args []string) bool {
	if len(args) >= 2 {
		if args[0] == "orders" && args[1] != "sync" && args[1] != "categories" && args[1] != "preview" {
			return false
		}
		if args[0] == "products" {
			switch args[1] {
			case "price-history", "price-history-purge", "watch-add", "watch-list", "watch-remove", "watch-clear":
				return false
			}
		}
	}
	return true
}

type camofoxReadError struct{ cause error }

func (e *camofoxReadError) Error() string { return "Camofox operation failed" }
func (e *camofoxReadError) Unwrap() error { return e.cause }

func runCamofox(ctx context.Context, args []string, stdout, stderr io.Writer, version string) (resultErr error) {
	defer func() {
		if resultErr != nil {
			resultErr = &camofoxReadError{cause: resultErr}
		}
	}()
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--camofox" {
			continue
		}
		for _, mode := range []string{"--aside", "--headed", "--current-browser", "--ordinary-browser", "--apple-events"} {
			if arg == mode || strings.HasPrefix(arg, mode+"=") {
				return errors.New("choose only one browser mode")
			}
		}
		filtered = append(filtered, arg)
	}
	args = filtered
	if len(args) == 0 {
		return browser.ErrCamofoxUnsupported
	}
	if args[0] != "products" && args[0] != "orders" && args[0] != "account" && args[0] != "auth" && args[0] != "mcp" && args[0] != "doctor" {
		return browser.ErrCamofoxUnsupported
	}
	if args[0] == "account" && (len(args) < 2 || args[1] != "benefits") {
		return browser.ErrCamofoxUnsupported
	}
	if len(args) == 3 && args[0] == "account" && args[1] == "benefits" && (args[2] == "--help" || args[2] == "-h") {
		_, err := fmt.Fprintln(stdout, "usage: coupangctl account benefits [--cash-pages N]\nRead membership and reward evidence through the dedicated headless Camofox session. Cash pages: 1–100 (default 50). Never changes membership or payment state.")
		return err
	}
	if (args[0] == "mcp" || args[0] == "doctor") && len(args) != 1 {
		return browser.ErrCamofoxUnsupported
	}
	if args[0] == "orders" && len(args) >= 2 && args[1] == "preview" {
		if isFlagHelp(args[2:]) {
			_, err := fmt.Fprintln(stdout, orderPreviewUsage)
			return err
		}
		if len(args) != 2 {
			return errOrderPreviewUsage
		}
	}
	if args[0] == "auth" && len(args) >= 2 && (args[1] == "login" || args[1] == "ensure") && len(args) == 3 && (args[2] == "--help" || args[2] == "-h") {
		_, err := fmt.Fprintln(stdout, "로그인 방식:\n  --manual       전용 Camofox 창에서 직접 인증 (기본값)\n  --qr --link    창 없이 일회성 QR 앱 링크와 확인 숫자로 승인\nSMS 발송·OTP 입력 자동화는 지원하지 않습니다.\n이미 인증된 세션이면 창을 열지 않습니다. QR 링크는 저장·공유하지 마세요.")
		return err
	}
	var loginRequest core.LoginRequest
	if args[0] == "auth" && len(args) >= 2 && (args[1] == "login" || args[1] == "ensure") {
		request, err := camofoxLoginRequest(args[2:], stderr)
		if err != nil {
			return browser.ErrCamofoxUnsupported
		}
		loginRequest = request
	}
	if args[0] == "mcp" {
		return runCamofoxMCP(ctx, version)
	}
	paths, err := platform.DefaultPaths()
	if err != nil {
		return err
	}
	var cfg browser.CamofoxConfig
	if camofoxNeedsRuntime(args) {
		cfg, err = browser.ReadCamofoxConfig(paths.StateDir)
		if err != nil {
			return errors.Join(core.NewError("browser_setup_required"), browser.ErrCamofoxUnavailable)
		}
	}
	source := browser.NewCamofox(paths.StateDir, cfg)
	if args[0] == "orders" && len(args) >= 2 && args[1] == "preview" {
		return runOrderPreview(ctx, stdout, orders.NewWithPageSourceAndSyncSource(nil, source, core.SyncSourceCamofox))
	}
	affiliate := partners.NewFromEnvironment(os.Getenv)
	service := products.NewWithAffiliate(coupangproducts.New(source), affiliate)
	if args[0] == "doctor" {
		if err := browser.CheckCamofoxInstallation(cfg); err != nil {
			return err
		}
		return writeJSON(stdout, map[string]any{"schema_version": 1, "browser": "Camofox", "runtime_installed": true, "browser_started": false, "authentication_checked": false})
	}
	authService := auth.NewService(source)
	if args[0] == "auth" {
		if len(args) < 2 {
			return browser.ErrCamofoxUnsupported
		}
		if args[1] == "login" || args[1] == "ensure" {
			request := loginRequest
			if request.PresentQRLink == nil {
				request.Progress = func(_ context.Context, stage core.LoginStage) error {
					if stage == core.LoginWaitingForApproval {
						_, err := fmt.Fprintln(stderr, "전용 Camofox 창에서 쿠팡 인증을 완료하세요. 완료 후 창을 닫고 백그라운드 세션을 검증합니다.")
						return err
					}
					return nil
				}
			}
			result, err := authService.Login(ctx, request)
			if err != nil {
				return err
			}
			return writeJSON(stdout, result)
		}
		if args[1] != "status" && args[1] != "verify" {
			return browser.ErrCamofoxUnsupported
		}
		return runAuth(ctx, args[1:], stdout, authService)
	}
	// A plain search does not open a ledger. Local history uses a
	// separate Camofox ledger; existing Aside/Chrome data are never merged.
	needsLedger := args[0] == "orders" || args[0] == "account"
	if args[0] == "products" && len(args) > 1 && args[1] != "search" && args[1] != "recommend" {
		needsLedger = true
	}
	for _, arg := range args {
		if arg == "--use-purchase-history" || arg == "--use-purchase-history=true" {
			needsLedger = true
		}
	}
	if !needsLedger {
		return runProducts(ctx, args[1:], stdout, service)
	}
	ledger, err := store.Open(ctx, filepath.Join(paths.StateDir, "camofox-coupangctl.sqlite3"))
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return core.WithErrorCode("local_data_unavailable", err)
	}
	defer ledger.Close()
	orderService := orders.NewWithPageSourceAndSyncSource(ledger, source, core.SyncSourceCamofox)
	accountService := account.NewWithCosts(source, ledger)
	service = products.NewWithAffiliateAndPrices(coupangproducts.New(source), affiliate, ledger).WithPurchaseHistory(ledger)
	if args[0] == "orders" {
		return runOrders(ctx, args[1:], stdout, orderService)
	}
	if args[0] == "account" {
		return runAccount(ctx, args[1:], stdout, accountService)
	}
	return runProducts(ctx, args[1:], stdout, service)
}
