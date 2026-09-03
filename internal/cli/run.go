package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	accountworkflow "github.com/JungHoonGhae/coupang-ctl/internal/account"
	"github.com/JungHoonGhae/coupang-ctl/internal/auth"
	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
	"github.com/JungHoonGhae/coupang-ctl/internal/browserbridge"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	coupangaccount "github.com/JungHoonGhae/coupang-ctl/internal/coupang/account"
	coupangproducts "github.com/JungHoonGhae/coupang-ctl/internal/coupang/products"
	coupangreceipts "github.com/JungHoonGhae/coupang-ctl/internal/coupang/receipts"
	"github.com/JungHoonGhae/coupang-ctl/internal/loginassist"
	"github.com/JungHoonGhae/coupang-ctl/internal/mcpserver"
	orderworkflow "github.com/JungHoonGhae/coupang-ctl/internal/orders"
	"github.com/JungHoonGhae/coupang-ctl/internal/partners"
	"github.com/JungHoonGhae/coupang-ctl/internal/platform"
	productworkflow "github.com/JungHoonGhae/coupang-ctl/internal/products"
	"github.com/JungHoonGhae/coupang-ctl/internal/recap"
	receiptworkflow "github.com/JungHoonGhae/coupang-ctl/internal/receipts"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
)

const orderSyncUsage = "usage: coupangctl orders sync [--max-pages N] [--headed|--current-browser|--ordinary-browser]"

func Run(ctx context.Context, args []string, stdout, stderr io.Writer, version string) error {
	if len(args) == 0 {
		return usage(stderr)
	}
	if strings.HasPrefix(args[0], "chrome-extension://") {
		return runChromeNativeHost(ctx, args, os.Stdin, stdout)
	}
	args = expandConvenienceCommand(args)
	if args[0] == "version" {
		return writeJSON(stdout, map[string]string{"name": "coupangctl", "version": version})
	}
	if args[0] == "capabilities" {
		if len(args) != 1 {
			return errors.New("usage: coupangctl capabilities")
		}
		return writeJSON(stdout, core.CurrentCapabilities())
	}
	if args[0] == "current-browser" {
		return runCurrentBrowser(ctx, args[1:], stdout, nativeCurrentBrowserStatusProvider{})
	}
	if len(args) >= 2 && args[0] == "products" && args[1] == "watch-schedule" {
		executable, err := os.Executable()
		if err != nil {
			return fmt.Errorf("resolve coupangctl executable: %w", err)
		}
		executable, err = filepath.Abs(executable)
		if err != nil {
			return fmt.Errorf("resolve absolute coupangctl executable: %w", err)
		}
		return runProductWatchSchedule(args[2:], stdout, executable)
	}
	if args[0] == "browser-bridge" {
		paths, err := platform.DefaultPaths()
		if err != nil {
			return err
		}
		executable, err := os.Executable()
		if err != nil {
			return fmt.Errorf("resolve coupangctl executable: %w", err)
		}
		executable, err = filepath.Abs(executable)
		if err != nil {
			return fmt.Errorf("resolve absolute coupangctl executable: %w", err)
		}
		executable, err = filepath.EvalSymlinks(executable)
		if err != nil {
			return fmt.Errorf("resolve coupangctl executable symlinks: %w", err)
		}
		return runBrowserBridge(args[1:], stdout, paths.StateDir, executable)
	}
	if conflictingOrderSyncBrowserModes(args) {
		return errors.New(orderSyncUsage)
	}

	paths, err := platform.DefaultPaths()
	if err != nil {
		return err
	}
	browserAdapter := browser.NewNative(paths.ProfileDir)
	if backgroundReadRequested(args) {
		browserAdapter = browser.NewNativeBackground(paths.ProfileDir)
	} else if currentBrowserReadRequested(args) {
		browserAdapter = browser.NewNativeCurrentBrowser()
	} else if headedReadRequested(args) {
		browserAdapter = browser.NewNativeHeadedSync(paths.ProfileDir)
	}
	defer browserAdapter.Close()
	authService := auth.NewService(browserAdapter)
	receiptService := receiptworkflow.New(coupangreceipts.New(browserAdapter))

	switch args[0] {
	case "doctor":
		return runDoctor(ctx, stdout, paths, authService)
	case "auth":
		return runAuth(ctx, args[1:], stdout, stderr, authService, loginassist.New(paths.ProfileDir), newTerminalLoginSecrets(os.Stdin, stderr))
	case "orders":
		ledger, err := store.Open(ctx, paths.Database)
		if err != nil {
			return err
		}
		defer ledger.Close()
		if ordinaryBrowserReadRequested(args) {
			bridge, err := browser.StartOrdinaryBrowserBridge(paths.StateDir)
			if err != nil {
				return err
			}
			defer bridge.Close()
			if _, err := fmt.Fprintln(stderr, "일반 Chrome에서 쿠팡 주문목록을 연 뒤 coupangctl 확장 버튼을 한 번 누르세요."); err != nil {
				return err
			}
			return runOrders(ctx, args[1:], stdout, orderworkflow.NewWithPageSource(ledger, bridge))
		}
		orderService := orderworkflow.New(ledger, browserAdapter)
		if currentBrowserReadRequested(args) {
			orderService = orderworkflow.NewWithSyncSource(ledger, browserAdapter, core.SyncSourceCurrentBrowser)
		}
		return runOrders(ctx, args[1:], stdout, orderService)
	case "products":
		ledger, err := store.Open(ctx, paths.Database)
		if err != nil {
			return err
		}
		defer ledger.Close()
		productService := productworkflow.NewWithAffiliateAndPrices(coupangproducts.New(browserAdapter), partners.NewFromEnvironment(os.Getenv), ledger)
		return runProducts(ctx, args[1:], stdout, productService)
	case "account":
		ledger, err := store.Open(ctx, paths.Database)
		if err != nil {
			return err
		}
		defer ledger.Close()
		accountService := accountworkflow.NewWithCosts(coupangaccount.New(browserAdapter), ledger)
		return runAccount(ctx, args[1:], stdout, accountService)
	case "receipts":
		return runReceipts(ctx, args[1:], stdout, receiptService)
	case "mcp":
		if len(args) != 1 {
			return errors.New("usage: coupangctl mcp")
		}
		ledger, err := store.Open(ctx, paths.Database)
		if err != nil {
			return err
		}
		defer ledger.Close()
		productService := productworkflow.NewWithAffiliateAndPrices(coupangproducts.New(browserAdapter), partners.NewFromEnvironment(os.Getenv), ledger)
		accountService := accountworkflow.NewWithCosts(coupangaccount.New(browserAdapter), ledger)
		return mcpserver.RunWithProviders(ctx, mcpserver.Providers{
			Auth:                 authService,
			Orders:               orderworkflow.New(ledger, browserAdapter),
			CurrentBrowserStatus: nativeCurrentBrowserStatusProvider{},
			CurrentBrowserOrders: currentBrowserOrderSync{ledger: ledger},
			OrdinaryOrders:       ordinaryBrowserOrderSync{ledger: ledger, stateDir: paths.StateDir},
			Products:             productService,
			Account:              accountService,
			Receipts:             receiptService,
		}, version)
	default:
		return usage(stderr)
	}
}

func expandConvenienceCommand(args []string) []string {
	if len(args) == 0 {
		return args
	}
	var subcommand string
	switch args[0] {
	case "sync":
		subcommand = "sync"
	case "recap":
		subcommand = "recap"
	default:
		return args
	}
	expanded := make([]string, 0, len(args)+1)
	expanded = append(expanded, "orders", subcommand)
	return append(expanded, args[1:]...)
}

func runChromeNativeHost(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) < 1 || len(args) > 2 {
		return browser.ErrOrdinaryNativeProtocol
	}
	if len(args) == 2 && !validParentWindowArgument(args[1]) {
		return browser.ErrOrdinaryNativeProtocol
	}
	paths, err := platform.DefaultPaths()
	if err != nil {
		return err
	}
	return browser.RunOrdinaryBrowserNativeHost(
		ctx,
		paths.StateDir,
		args[0],
		browser.OrdinaryBrowserExtensionID,
		stdin,
		stdout,
	)
}

func validParentWindowArgument(value string) bool {
	const prefix = "--parent-window="
	if !strings.HasPrefix(value, prefix) || len(value) == len(prefix) {
		return false
	}
	for _, character := range value[len(prefix):] {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

type receiptWorkflow interface {
	Status(context.Context) (core.ReceiptRequestStatusSnapshot, error)
	History(context.Context, core.ReceiptHistoryRequest) (core.ReceiptHistoryPage, error)
	Summary(context.Context, core.ReceiptSummaryRequest) (core.ReceiptSummary, error)
	Overview(context.Context, core.ReceiptOverviewRequest) (core.ReceiptOverview, error)
	Download(context.Context, core.ReceiptDownloadRequest) (receiptworkflow.Download, error)
	Vendor(context.Context, core.VendorReceiptRequest) (core.VendorReceiptSnapshot, error)
}

func runReceipts(ctx context.Context, args []string, stdout io.Writer, workflow receiptWorkflow) error {
	if len(args) == 0 {
		return errors.New("usage: coupangctl receipts <status|list|summary|overview|vendor|download>")
	}
	switch args[0] {
	case "status":
		flags := newFlagSet("receipts status")
		_ = flags.Bool("headed", false, "use a headed browser fallback")
		if err := parseFlags(flags, args[1:], "usage: coupangctl receipts status [--headed]"); err != nil {
			return err
		}
		result, err := workflow.Status(ctx)
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "overview":
		flags := newFlagSet("receipts overview")
		from := flags.String("from", "", "inclusive YYYY-MM-DD start date")
		to := flags.String("to", "", "inclusive YYYY-MM-DD end date")
		maxCards := flags.Int("max-cards", 20, "maximum observed card methods per calendar-year read")
		_ = flags.Bool("headed", false, "use a headed browser fallback")
		const commandUsage = "usage: coupangctl receipts overview --from YYYY-MM-DD --to YYYY-MM-DD [--max-cards N] [--headed]"
		if err := parseFlags(flags, args[1:], commandUsage); err != nil || *from == "" || *to == "" {
			return errors.New(commandUsage)
		}
		result, err := workflow.Overview(ctx, core.ReceiptOverviewRequest{From: *from, To: *to, MaxCards: *maxCards})
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "list":
		flags := newFlagSet("receipts list")
		kind := flags.String("kind", "", "receipt family: cash or card")
		page := flags.Int("page", 0, "zero-based request-history page")
		size := flags.Int("size", 5, "history rows per page")
		_ = flags.Bool("headed", false, "use a headed browser fallback")
		const commandUsage = "usage: coupangctl receipts list --kind <cash|card> [--page N] [--size N] [--headed]"
		if err := parseFlags(flags, args[1:], commandUsage); err != nil || *kind == "" {
			return errors.New(commandUsage)
		}
		result, err := workflow.History(ctx, core.ReceiptHistoryRequest{Kind: core.ReceiptKind(*kind), PageIndex: *page, PageSize: *size})
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "summary":
		flags := newFlagSet("receipts summary")
		kind := flags.String("kind", "", "receipt family: cash or card")
		from := flags.String("from", "", "inclusive YYYY-MM-DD start date")
		to := flags.String("to", "", "inclusive YYYY-MM-DD end date")
		maxCards := flags.Int("max-cards", 20, "maximum observed card methods to summarize")
		_ = flags.Bool("headed", false, "use a headed browser fallback")
		const commandUsage = "usage: coupangctl receipts summary --kind <cash|card> --from YYYY-MM-DD --to YYYY-MM-DD [--max-cards N] [--headed]"
		if err := parseFlags(flags, args[1:], commandUsage); err != nil || *kind == "" || *from == "" || *to == "" {
			return errors.New(commandUsage)
		}
		result, err := workflow.Summary(ctx, core.ReceiptSummaryRequest{Kind: core.ReceiptKind(*kind), From: *from, To: *to, MaxCards: *maxCards})
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "download":
		flags := newFlagSet("receipts download")
		kind := flags.String("kind", "", "receipt family: cash or card")
		page := flags.Int("page", 0, "zero-based request-history page")
		size := flags.Int("size", 5, "history rows per page")
		historyIndex := flags.Int("history-index", -1, "zero-based history row index")
		downloadIndex := flags.Int("download-index", 0, "zero-based file index within the history row")
		output := flags.String("output", "", "new private output file path")
		_ = flags.Bool("headed", false, "use a headed browser fallback")
		const commandUsage = "usage: coupangctl receipts download --kind <cash|card> --history-index N --output PATH [--download-index N] [--page N] [--size N] [--headed]"
		if err := parseFlags(flags, args[1:], commandUsage); err != nil || *kind == "" || *historyIndex < 0 || *output == "" {
			return errors.New(commandUsage)
		}
		download, err := workflow.Download(ctx, core.ReceiptDownloadRequest{
			Kind: core.ReceiptKind(*kind), PageIndex: *page, PageSize: *size,
			HistoryIndex: *historyIndex, DownloadIndex: *downloadIndex,
		})
		if err != nil {
			return err
		}
		outputPath, err := writePrivateReceipt(*output, download.Content)
		if err != nil {
			return err
		}
		download.Metadata.OutputPath = outputPath
		return writeJSON(stdout, download.Metadata)
	case "vendor":
		flags := newFlagSet("receipts vendor")
		sourceRef := flags.String("source-ref", "", "hashed source_ref returned by orders list")
		maxPages := flags.Int("max-pages", 1000, "maximum order pages searched in memory")
		_ = flags.Bool("headed", false, "use a headed browser fallback")
		const commandUsage = "usage: coupangctl receipts vendor --source-ref HASH [--max-pages N] [--headed]"
		if err := parseFlags(flags, args[1:], commandUsage); err != nil || *sourceRef == "" {
			return errors.New(commandUsage)
		}
		result, err := workflow.Vendor(ctx, core.VendorReceiptRequest{SourceRef: *sourceRef, MaxPages: *maxPages})
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	default:
		return errors.New("usage: coupangctl receipts <status|list|summary|overview|vendor|download>")
	}
}

func writePrivateReceipt(path string, content []byte) (string, error) {
	if len(content) == 0 {
		return "", errors.New("receipt download was empty")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve receipt output path: %w", err)
	}
	file, err := os.OpenFile(absolute, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("create receipt output without overwriting: %w", err)
	}
	keep := false
	defer func() {
		_ = file.Close()
		if !keep {
			_ = os.Remove(absolute)
		}
	}()
	if _, err := file.Write(content); err != nil {
		return "", fmt.Errorf("write receipt output: %w", err)
	}
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("sync receipt output: %w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close receipt output: %w", err)
	}
	keep = true
	return absolute, nil
}

type accountWorkflow interface {
	Snapshot(context.Context, core.AccountBenefitsRequest) (core.AccountBenefitsSnapshot, error)
}

func runAccount(ctx context.Context, args []string, stdout io.Writer, workflow accountWorkflow) error {
	if len(args) == 0 || args[0] != "benefits" {
		return errors.New("usage: coupangctl account benefits [--cash-pages N] [--headed]")
	}
	flags := newFlagSet("account benefits")
	cashPages := flags.Int("cash-pages", 50, "maximum Coupang Cash transaction pages")
	_ = flags.Bool("headed", false, "use a headed browser fallback")
	const commandUsage = "usage: coupangctl account benefits [--cash-pages N] [--headed]"
	if err := parseFlags(flags, args[1:], commandUsage); err != nil {
		return err
	}
	result, err := workflow.Snapshot(ctx, core.AccountBenefitsRequest{MaxCashTransactionPages: *cashPages})
	if err != nil {
		return err
	}
	return writeJSON(stdout, result)
}

type productWorkflow interface {
	Search(context.Context, core.ProductSearchRequest) (core.ProductSearchResult, error)
	Inspect(context.Context, core.ProductInspectRequest) (core.ProductInspection, error)
	PriceHistory(context.Context, core.ProductPriceHistoryRequest) (core.ProductPriceHistory, error)
	PurgePriceHistory(context.Context) (core.ProductPriceHistoryPurgeResult, error)
	AddPriceWatch(context.Context, core.ProductWatchRequest) (core.ProductWatchMutationResult, error)
	RemovePriceWatch(context.Context, core.ProductWatchRequest) (core.ProductWatchMutationResult, error)
	PriceWatchlist(context.Context) (core.ProductWatchList, error)
	RefreshPriceWatches(context.Context, core.ProductWatchRefreshRequest) (core.ProductWatchRefreshResult, error)
	ClearPriceWatches(context.Context) (core.ProductWatchClearResult, error)
	AddToCart(context.Context, core.CartAddRequest) (core.CartAddResult, error)
}

func runProducts(ctx context.Context, args []string, stdout io.Writer, workflow productWorkflow) error {
	if len(args) == 0 {
		return errors.New("usage: coupangctl products <search|inspect|price-history|price-history-purge|watch-add|watch-list|watch-remove|watch-clear|watch-refresh|watch-schedule|cart-add>")
	}
	switch args[0] {
	case "search":
		flags := newFlagSet("products search")
		query := flags.String("query", "", "natural-language product query")
		categoryID := flags.String("category-id", "", "source-native Coupang category identifier")
		limit := flags.Int("limit", 10, "maximum results")
		minPrice := flags.Int64("min-price", 0, "minimum current price in KRW")
		maxPrice := flags.Int64("max-price", 0, "maximum current price in KRW")
		minRating := flags.Float64("min-rating", 0, "minimum rating")
		minReviews := flags.Int("min-reviews", 0, "minimum review count")
		rocket := flags.Bool("rocket", false, "only Rocket items")
		freeShipping := flags.Bool("free-shipping", false, "only explicitly free-shipping items")
		excludeSponsored := flags.Bool("exclude-sponsored", false, "exclude sponsored items")
		minMemoryGB := flags.Int("min-memory-gb", 0, "minimum observed computer memory")
		minStorageGB := flags.Int("min-storage-gb", 0, "minimum observed computer storage")
		excludeUsed := flags.Bool("exclude-used", false, "exclude explicitly used, refurbished, or display-unit items")
		includeVariants := flags.Bool("include-variants", false, "return multiple options from the same product page")
		noAffiliate := flags.Bool("no-affiliate", false, "return canonical Coupang URLs only")
		sortOrder := flags.String("sort", "relevance", "relevance, coupang_ranking, sales, latest, price_asc, price_desc, rating, or review_count")
		_ = flags.Bool("headed", false, "use a headed browser fallback")
		const searchUsage = "usage: coupangctl products search (--query TEXT | --category-id ID) [--limit N] [--max-price KRW] [--min-rating N] [--min-reviews N] [--min-memory-gb N] [--min-storage-gb N] [--exclude-used] [--include-variants] [--rocket] [--free-shipping] [--exclude-sponsored] [--sort ORDER] [--no-affiliate] [--headed]"
		if err := parseFlags(flags, args[1:], searchUsage); err != nil || (strings.TrimSpace(*query) == "" && *categoryID == "") {
			return errors.New(searchUsage)
		}
		result, err := workflow.Search(ctx, core.ProductSearchRequest{
			Query: *query, CategoryID: *categoryID, Limit: *limit, MinPrice: *minPrice, MaxPrice: *maxPrice,
			MinRating: *minRating, MinReviewCount: *minReviews, RocketOnly: *rocket,
			FreeShippingOnly: *freeShipping, ExcludeSponsored: *excludeSponsored,
			MinMemoryGB: *minMemoryGB, MinStorageGB: *minStorageGB, ExcludeUsed: *excludeUsed,
			IncludeVariants: *includeVariants, DisableAffiliate: *noAffiliate, Sort: core.ProductSort(*sortOrder),
		})
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "inspect":
		flags := newFlagSet("products inspect")
		productID := flags.String("product-id", "", "product identifier returned by search")
		itemID := flags.String("item-id", "", "item identifier returned by search")
		vendorItemID := flags.String("vendor-item-id", "", "vendor item identifier returned by search")
		reviewLimit := flags.Int("review-limit", 5, "maximum sanitized reviews")
		imageLimit := flags.Int("detail-image-limit", 20, "maximum detailed images")
		noAffiliate := flags.Bool("no-affiliate", false, "return the canonical Coupang URL only")
		_ = flags.Bool("headed", false, "use a headed browser fallback")
		const inspectUsage = "usage: coupangctl products inspect --product-id ID [--item-id ID] [--vendor-item-id ID] [--review-limit N] [--detail-image-limit N] [--no-affiliate] [--headed]"
		if err := parseFlags(flags, args[1:], inspectUsage); err != nil || *productID == "" {
			return errors.New(inspectUsage)
		}
		result, err := workflow.Inspect(ctx, core.ProductInspectRequest{
			ProductID: *productID, ItemID: *itemID, VendorItemID: *vendorItemID,
			ReviewLimit: *reviewLimit, DetailImageLimit: *imageLimit, DisableAffiliate: *noAffiliate,
		})
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "price-history":
		flags := newFlagSet("products price-history")
		productID := flags.String("product-id", "", "product identifier returned by search")
		vendorItemID := flags.String("vendor-item-id", "", "optional exact vendor item identifier")
		limit := flags.Int("limit", 200, "maximum stored observations")
		const historyUsage = "usage: coupangctl products price-history --product-id ID [--vendor-item-id ID] [--limit N]"
		if err := parseFlags(flags, args[1:], historyUsage); err != nil || *productID == "" {
			return errors.New(historyUsage)
		}
		result, err := workflow.PriceHistory(ctx, core.ProductPriceHistoryRequest{ProductID: *productID, VendorItemID: *vendorItemID, Limit: *limit})
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "price-history-purge":
		flags := newFlagSet("products price-history-purge")
		confirmation := flags.String("confirm", "", "confirmation token")
		const purgeUsage = "usage: coupangctl products price-history-purge --confirm purge-product-price-history"
		if err := parseFlags(flags, args[1:], purgeUsage); err != nil || *confirmation != "purge-product-price-history" {
			return errors.New(purgeUsage)
		}
		result, err := workflow.PurgePriceHistory(ctx)
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "watch-add":
		flags := newFlagSet("products watch-add")
		productID := flags.String("product-id", "", "product identifier with an existing local price observation")
		vendorItemID := flags.String("vendor-item-id", "", "exact vendor item identifier")
		const watchAddUsage = "usage: coupangctl products watch-add --product-id ID [--vendor-item-id ID]"
		if err := parseFlags(flags, args[1:], watchAddUsage); err != nil || *productID == "" {
			return errors.New(watchAddUsage)
		}
		result, err := workflow.AddPriceWatch(ctx, core.ProductWatchRequest{ProductID: *productID, VendorItemID: *vendorItemID})
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "watch-list":
		if err := parseFlags(newFlagSet("products watch-list"), args[1:], "usage: coupangctl products watch-list"); err != nil {
			return err
		}
		result, err := workflow.PriceWatchlist(ctx)
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "watch-remove":
		flags := newFlagSet("products watch-remove")
		productID := flags.String("product-id", "", "watched product identifier")
		vendorItemID := flags.String("vendor-item-id", "", "exact watched vendor item identifier")
		const watchRemoveUsage = "usage: coupangctl products watch-remove --product-id ID [--vendor-item-id ID]"
		if err := parseFlags(flags, args[1:], watchRemoveUsage); err != nil || *productID == "" {
			return errors.New(watchRemoveUsage)
		}
		result, err := workflow.RemovePriceWatch(ctx, core.ProductWatchRequest{ProductID: *productID, VendorItemID: *vendorItemID})
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "watch-clear":
		flags := newFlagSet("products watch-clear")
		confirmation := flags.String("confirm", "", "confirmation token")
		const watchClearUsage = "usage: coupangctl products watch-clear --confirm clear-product-watchlist"
		if err := parseFlags(flags, args[1:], watchClearUsage); err != nil || *confirmation != "clear-product-watchlist" {
			return errors.New(watchClearUsage)
		}
		result, err := workflow.ClearPriceWatches(ctx)
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "watch-refresh":
		flags := newFlagSet("products watch-refresh")
		limit := flags.Int("limit", 10, "maximum due watch entries")
		staleHours := flags.Int("stale-hours", 24, "minimum hours since the last check")
		_ = flags.Bool("headed", false, "use a headed browser fallback")
		const watchRefreshUsage = "usage: coupangctl products watch-refresh [--limit N] [--stale-hours N] [--headed]"
		if err := parseFlags(flags, args[1:], watchRefreshUsage); err != nil {
			return err
		}
		result, err := workflow.RefreshPriceWatches(ctx, core.ProductWatchRefreshRequest{Limit: *limit, StaleHours: *staleHours})
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "cart-add":
		flags := newFlagSet("products cart-add")
		productID := flags.String("product-id", "", "product identifier returned by search")
		itemID := flags.String("item-id", "", "item identifier returned by search")
		vendorItemID := flags.String("vendor-item-id", "", "exact vendor item identifier returned by search")
		quantity := flags.Int("quantity", 1, "quantity to add")
		confirmed := flags.Bool("confirm-add-to-cart", false, "confirm this external cart change")
		_ = flags.Bool("headed", false, "use a headed browser fallback")
		const cartUsage = "usage: coupangctl products cart-add --product-id ID --vendor-item-id ID [--item-id ID] [--quantity N] --confirm-add-to-cart [--headed]"
		if err := parseFlags(flags, args[1:], cartUsage); err != nil || *productID == "" || *vendorItemID == "" || !*confirmed {
			return errors.New(cartUsage)
		}
		result, err := workflow.AddToCart(ctx, core.CartAddRequest{
			ProductID: *productID, ItemID: *itemID, VendorItemID: *vendorItemID,
			Quantity: *quantity, Confirmed: *confirmed,
		})
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	default:
		return errors.New("usage: coupangctl products <search|inspect|price-history|price-history-purge|watch-add|watch-list|watch-remove|watch-clear|watch-refresh|watch-schedule|cart-add>")
	}
}

type orderWorkflow interface {
	Sync(context.Context, core.SyncRequest) (core.SyncResult, error)
	EnrichCategories(context.Context, core.CategoryEnrichmentRequest) (core.CategoryEnrichmentResult, error)
	CategoryCatalog(context.Context, core.CategoryCatalogRequest) (core.CategoryCatalog, error)
	CategoryStability(context.Context) (core.CategoryStabilityReport, error)
	List(context.Context, core.OrderFilter) ([]core.Order, error)
	Spend(context.Context, core.OrderFilter) (core.SpendSummary, error)
	Stats(context.Context, core.OrderFilter) (core.OrderStats, error)
	Insights(context.Context, core.OrderFilter) (core.ShoppingInsights, error)
	ProductInsights(context.Context, core.OrderFilter) (core.ProductInsights, error)
	ReorderCandidates(context.Context, core.OrderFilter) ([]core.ReorderCandidate, error)
	Export(context.Context, core.OrderFilter) (core.OrderExport, error)
	Purge(context.Context) (core.PurgeResult, error)
	Import(context.Context, core.OrderExport) (core.UpsertResult, error)
}

func runOrders(ctx context.Context, args []string, stdout io.Writer, workflow orderWorkflow) error {
	if len(args) == 0 {
		return errors.New("usage: coupangctl orders <sync|categories|category-catalog|category-stability|list|spend|stats|insights|products|recap|recap-image|reorder|export|import|purge>")
	}
	switch args[0] {
	case "sync":
		flags := newFlagSet("orders sync")
		maxPages := flags.Int("max-pages", 100, "maximum pages to process")
		headed := flags.Bool("headed", false, "use headed browser fallback")
		currentBrowser := flags.Bool("current-browser", false, "use a user-approved connection to running Chrome")
		ordinaryBrowser := flags.Bool("ordinary-browser", false, "use the selected tab in ordinary Chrome")
		if err := parseFlags(flags, args[1:], orderSyncUsage); err != nil {
			return err
		}
		selectedBrowserModes := 0
		for _, selected := range []bool{*headed, *currentBrowser, *ordinaryBrowser} {
			if selected {
				selectedBrowserModes++
			}
		}
		if selectedBrowserModes > 1 {
			return errors.New(orderSyncUsage)
		}
		result, err := workflow.Sync(ctx, core.SyncRequest{MaxPages: *maxPages})
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "categories":
		flags := newFlagSet("orders categories")
		maxProducts := flags.Int("max-products", 25, "maximum uncached products to enrich")
		recheck := flags.Bool("recheck", false, "explicitly re-read cached breadcrumbs, oldest first")
		_ = flags.Bool("headed", false, "use a headed browser")
		if err := parseFlags(flags, args[1:], "usage: coupangctl orders categories [--max-products N] [--recheck] [--headed]"); err != nil {
			return err
		}
		result, err := workflow.EnrichCategories(ctx, core.CategoryEnrichmentRequest{MaxProducts: *maxProducts, Recheck: *recheck})
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "category-stability":
		if len(args) != 1 {
			return errors.New("usage: coupangctl orders category-stability")
		}
		result, err := workflow.CategoryStability(ctx)
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "category-catalog":
		flags := newFlagSet("orders category-catalog")
		query := flags.String("query", "", "optional observed category label text")
		limit := flags.Int("limit", 50, "maximum category paths")
		if err := parseFlags(flags, args[1:], "usage: coupangctl orders category-catalog [--query TEXT] [--limit N]"); err != nil {
			return err
		}
		result, err := workflow.CategoryCatalog(ctx, core.CategoryCatalogRequest{Query: *query, Limit: *limit})
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "list":
		filter, err := parseOrderFilter(args[1:], true, "usage: coupangctl orders list [--from YYYY-MM-DD] [--to YYYY-MM-DD] [--limit N]")
		if err != nil {
			return err
		}
		orders, err := workflow.List(ctx, filter)
		if err != nil {
			return err
		}
		return writeJSON(stdout, core.OrderList{Orders: orders})
	case "spend":
		filter, err := parseOrderFilter(args[1:], false, "usage: coupangctl orders spend [--from YYYY-MM-DD] [--to YYYY-MM-DD]")
		if err != nil {
			return err
		}
		result, err := workflow.Spend(ctx, filter)
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "stats":
		filter, err := parseOrderFilter(args[1:], false, "usage: coupangctl orders stats [--from YYYY-MM-DD] [--to YYYY-MM-DD]")
		if err != nil {
			return err
		}
		result, err := workflow.Stats(ctx, filter)
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "insights":
		filter, err := parseOrderFilter(args[1:], false, "usage: coupangctl orders insights [--from YYYY-MM-DD] [--to YYYY-MM-DD]")
		if err != nil {
			return err
		}
		result, err := workflow.Insights(ctx, filter)
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "products":
		filter, err := parseOrderFilter(args[1:], false, "usage: coupangctl orders products [--from YYYY-MM-DD] [--to YYYY-MM-DD]")
		if err != nil {
			return err
		}
		result, err := workflow.ProductInsights(ctx, filter)
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "recap":
		flags := newFlagSet("orders recap")
		from := flags.String("from", "", "start date")
		to := flags.String("to", "", "end date")
		output := flags.String("output", "", "new standalone HTML output file")
		includeProducts := flags.Bool("include-products", false, "include private product names, exact dates, and amounts")
		if err := parseFlags(flags, args[1:], "usage: coupangctl orders recap --output PATH [--from YYYY-MM-DD] [--to YYYY-MM-DD] [--include-products]"); err != nil || *output == "" {
			return errors.New("usage: coupangctl orders recap --output PATH [--from YYYY-MM-DD] [--to YYYY-MM-DD] [--include-products]")
		}
		filter := core.OrderFilter{From: *from, To: *to}
		result, err := workflow.Insights(ctx, filter)
		if err != nil {
			return err
		}
		options := recap.Options{}
		if *includeProducts {
			products, err := workflow.ProductInsights(ctx, filter)
			if err != nil {
				return err
			}
			options.Products = &products
		}
		written, err := recap.WriteNewFileWithOptions(*output, result, options)
		if err != nil {
			return err
		}
		return writeJSON(stdout, written)
	case "recap-image":
		flags := newFlagSet("orders recap-image")
		from := flags.String("from", "", "start date")
		to := flags.String("to", "", "end date")
		output := flags.String("output", "", "new public-safe PNG output file")
		confirmed := flags.Bool("confirm-public-safe-image", false, "confirm the exact previewed public-safe fields")
		const imageUsage = "usage: coupangctl orders recap-image [--from YYYY-MM-DD] [--to YYYY-MM-DD] [--output PATH --confirm-public-safe-image]"
		if err := parseFlags(flags, args[1:], imageUsage); err != nil {
			return err
		}
		result, err := workflow.Insights(ctx, core.OrderFilter{From: *from, To: *to})
		if err != nil {
			return err
		}
		preview := recap.PublicSharePreview(result)
		if !*confirmed {
			return writeJSON(stdout, preview)
		}
		if *output == "" {
			return errors.New(imageUsage)
		}
		written, err := recap.WritePublicShareImage(ctx, *output, result, browser.NewLocalPageRenderer())
		if err != nil {
			return err
		}
		return writeJSON(stdout, written)
	case "reorder":
		filter, err := parseOrderFilter(args[1:], true, "usage: coupangctl orders reorder [--from YYYY-MM-DD] [--to YYYY-MM-DD] [--limit N]")
		if err != nil {
			return err
		}
		candidates, err := workflow.ReorderCandidates(ctx, filter)
		if err != nil {
			return err
		}
		return writeJSON(stdout, core.NewReorderList(candidates))
	case "export":
		filter, err := parseOrderFilter(args[1:], true, "usage: coupangctl orders export [--from YYYY-MM-DD] [--to YYYY-MM-DD] [--limit N]")
		if err != nil {
			return err
		}
		result, err := workflow.Export(ctx, filter)
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "import":
		flags := newFlagSet("orders import")
		path := flags.String("file", "", "normalized export file")
		if err := parseFlags(flags, args[1:], "usage: coupangctl orders import --file PATH"); err != nil || *path == "" {
			return errors.New("usage: coupangctl orders import --file PATH")
		}
		exported, err := readOrderExport(*path)
		if err != nil {
			return err
		}
		result, err := workflow.Import(ctx, exported)
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "purge":
		flags := newFlagSet("orders purge")
		confirmation := flags.String("confirm", "", "confirmation token")
		if err := parseFlags(flags, args[1:], "usage: coupangctl orders purge --confirm purge-normalized-orders"); err != nil {
			return err
		}
		if *confirmation != "purge-normalized-orders" {
			return errors.New("usage: coupangctl orders purge --confirm purge-normalized-orders")
		}
		result, err := workflow.Purge(ctx)
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	default:
		return errors.New("usage: coupangctl orders <sync|categories|category-catalog|category-stability|list|spend|stats|insights|products|recap|recap-image|reorder|export|import|purge>")
	}
}

func readOrderExport(path string) (core.OrderExport, error) {
	file, err := os.Open(path)
	if err != nil {
		return core.OrderExport{}, errors.New("open normalized export file")
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, 64<<20))
	var exported core.OrderExport
	if err := decoder.Decode(&exported); err != nil {
		return core.OrderExport{}, errors.New("decode normalized export file")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return core.OrderExport{}, errors.New("normalized export file contains trailing data")
	}
	return exported, nil
}

func parseOrderFilter(args []string, allowLimit bool, usage string) (core.OrderFilter, error) {
	flags := newFlagSet("orders filter")
	from := flags.String("from", "", "start date")
	to := flags.String("to", "", "end date")
	var limit *int
	if allowLimit {
		limit = flags.Int("limit", 100, "maximum results")
	}
	if err := parseFlags(flags, args, usage); err != nil {
		return core.OrderFilter{}, err
	}
	filter := core.OrderFilter{From: *from, To: *to}
	if limit != nil {
		filter.Limit = *limit
	}
	return filter, nil
}

func newFlagSet(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags
}

func parseFlags(flags *flag.FlagSet, args []string, usage string) error {
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return errors.New(usage)
	}
	return nil
}

func headedReadRequested(args []string) bool {
	if len(args) < 2 {
		return false
	}
	eligible := (args[0] == "auth" && args[1] == "verify") ||
		(args[0] == "orders" && (args[1] == "sync" || args[1] == "categories")) ||
		(args[0] == "products" && (args[1] == "search" || args[1] == "inspect" || args[1] == "watch-refresh" || args[1] == "cart-add")) ||
		(args[0] == "account" && args[1] == "benefits") ||
		args[0] == "receipts"
	if !eligible {
		return false
	}
	for _, argument := range args[2:] {
		if argument == "--headed" || argument == "--headed=true" {
			return true
		}
	}
	return false
}

func ordinaryBrowserReadRequested(args []string) bool {
	if len(args) < 2 || args[0] != "orders" || args[1] != "sync" {
		return false
	}
	for _, argument := range args[2:] {
		if argument == "--ordinary-browser" || argument == "--ordinary-browser=true" {
			return true
		}
	}
	return false
}

func currentBrowserReadRequested(args []string) bool {
	if len(args) < 2 || args[0] != "orders" || args[1] != "sync" {
		return false
	}
	for _, argument := range args[2:] {
		if argument == "--current-browser" || argument == "--current-browser=true" {
			return true
		}
	}
	return false
}

func backgroundReadRequested(args []string) bool {
	if len(args) == 1 && args[0] == "mcp" {
		return true
	}
	return len(args) >= 2 && args[0] == "products" && args[1] == "watch-refresh" && !headedReadRequested(args)
}

func conflictingOrderSyncBrowserModes(args []string) bool {
	if len(args) < 2 || args[0] != "orders" || args[1] != "sync" {
		return false
	}
	selected := 0
	for _, requested := range []bool{headedReadRequested(args), currentBrowserReadRequested(args), ordinaryBrowserReadRequested(args)} {
		if requested {
			selected++
		}
	}
	return selected > 1
}

type resendAssistant interface {
	Resend(context.Context) (core.OTPResendResult, error)
}

type loginSecretSource interface {
	Phone(context.Context) (string, error)
	OTP(context.Context) (string, error)
}

func runAuth(ctx context.Context, args []string, stdout, stderr io.Writer, service *auth.Service, assistant resendAssistant, secrets loginSecretSource) error {
	if len(args) == 0 {
		return errors.New("usage: coupangctl auth <status|login|verify|resend>")
	}
	switch args[0] {
	case "status":
		if len(args) != 1 {
			return errors.New("usage: coupangctl auth status")
		}
		status, err := service.Status(ctx)
		if err != nil {
			return err
		}
		return writeJSON(stdout, status)
	case "login":
		flags := newFlagSet("auth login")
		qr := flags.Bool("qr", false, "use QR login")
		phone := flags.Bool("phone", false, "use phone-number login")
		qrOutput := flags.String("qr-output", "", "write the ephemeral QR page to a private PNG")
		link := flags.Bool("link", false, "print the ephemeral QR app link and approval number to stderr")
		const loginUsage = "usage: coupangctl auth login [--qr|--phone] [--qr-output PATH|--link]"
		if err := parseFlags(flags, args[1:], loginUsage); err != nil || (*qr && *phone) || (*phone && (*qrOutput != "" || *link)) || (*link && *qrOutput != "") {
			return errors.New(loginUsage)
		}
		mode := core.LoginModeQR
		request := core.LoginRequest{Mode: mode, QROutputPath: *qrOutput}
		if *link {
			request.PresentQRLink = func(_ context.Context, link core.QRLoginLink) error {
				if _, err := io.WriteString(stderr, "Ephemeral Coupang QR login link (do not share):\n"); err != nil {
					return err
				}
				if _, err := fmt.Fprintln(stderr, link.URL); err != nil {
					return err
				}
				_, err := fmt.Fprintf(stderr, "Approval number: %s\n", link.ApprovalCode)
				return err
			}
		}
		if *phone {
			mode = core.LoginModePhone
			phoneNumber, err := secrets.Phone(ctx)
			if err != nil {
				return err
			}
			request = core.LoginRequest{Mode: mode, Phone: phoneNumber, ReadOTP: secrets.OTP}
		}
		result, err := service.Login(ctx, request)
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "verify":
		if len(args) != 1 && !(len(args) == 2 && args[1] == "--headed") {
			return errors.New("usage: coupangctl auth verify [--headed]")
		}
		status, err := service.Verify(ctx)
		if err != nil {
			return err
		}
		return writeJSON(stdout, status)
	case "resend":
		if len(args) != 1 {
			return errors.New("usage: coupangctl auth resend")
		}
		result, err := assistant.Resend(ctx)
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	default:
		return errors.New("usage: coupangctl auth <status|login|verify|resend>")
	}
}

type authStatusProvider interface {
	Status(context.Context) (core.AuthStatus, error)
}

func runDoctor(ctx context.Context, stdout io.Writer, paths platform.Paths, authProvider authStatusProvider) error {
	report := core.DoctorReport{OK: true}
	status, err := authProvider.Status(ctx)
	if err != nil {
		report.OK = false
		report.Checks = append(report.Checks, core.Check{Name: "browser", Status: core.CheckError, Message: "a supported browser is unavailable or misconfigured"})
		report.Checks = append(report.Checks, core.Check{Name: "background_session", Status: core.CheckError, Message: "background session readiness could not be checked"})
	} else {
		report.Checks = append(report.Checks, core.Check{Name: "browser", Status: core.CheckOK})
		check := core.Check{Name: "background_session", Status: core.CheckError}
		switch status.State {
		case core.AuthVerified:
			check.Status = core.CheckOK
		case core.AuthNotConfigured:
			check.Message = "run coupangctl auth login on an interactive desktop"
		case core.AuthUnverified:
			check.Message = "the stored login must be renewed with coupangctl auth login"
		case core.AuthAccessBlocked:
			check.Message = "background access was denied; retry later or explicitly run coupangctl auth verify --headed"
		default:
			check.Message = "the background session returned an unknown readiness state"
		}
		if check.Status != core.CheckOK {
			report.OK = false
		}
		report.Checks = append(report.Checks, check)
	}

	db, err := store.Open(ctx, paths.Database)
	if err != nil {
		report.OK = false
		report.Checks = append(report.Checks, core.Check{Name: "sqlite", Status: core.CheckError, Message: "the local database could not be opened"})
	} else {
		defer db.Close()
		if err := db.Ping(ctx); err != nil {
			report.OK = false
			report.Checks = append(report.Checks, core.Check{Name: "sqlite", Status: core.CheckError, Message: "the local database did not respond"})
		} else {
			report.Checks = append(report.Checks, core.Check{Name: "sqlite", Status: core.CheckOK})
		}
	}
	return writeJSON(stdout, report)
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("write JSON: %w", err)
	}
	return nil
}

func usage(_ io.Writer) error {
	return errors.New("invalid command")
}

func WriteError(w io.Writer, err error) {
	code := "internal_error"
	message := "coupangctl could not complete the command"
	switch {
	case errors.Is(err, browser.ErrDesktopRequired):
		code = "desktop_required"
		message = "open this command from an interactive desktop"
	case errors.Is(err, browser.ErrBrowserNotFound):
		code = "browser_not_found"
		message = "install a supported Chrome-family browser or set COUPANGCTL_BROWSER_PATH"
	case errors.Is(err, browser.ErrCurrentBrowserUnavailable):
		code = "current_browser_unavailable"
		message = "in Chrome 144 or newer, enable remote debugging at chrome://inspect/#remote-debugging, keep Chrome running, approve the connection, then retry with --current-browser"
	case errors.Is(err, browser.ErrProfileInUse):
		code = "profile_in_use"
		message = "another coupangctl browser operation is using the dedicated profile; wait for it to finish and retry"
	case errors.Is(err, browser.ErrProfileIncompatible):
		code = "browser_profile_incompatible"
		message = "the dedicated profile belongs to another browser family or a newer major version; use the original browser, or deliberately create a new state directory and sign in again"
	case errors.Is(err, browser.ErrAuthenticationRequired):
		code = "authentication_required"
		message = "run coupangctl auth login on an interactive desktop first"
	case errors.Is(err, browser.ErrStructuredOrderDataMissing):
		code = "structured_order_data_missing"
		message = "the authenticated order document was not available"
	case errors.Is(err, browser.ErrStructuredAccountBenefitsDataMissing):
		code = "structured_account_benefits_data_missing"
		message = "the authenticated membership or reward document was not available"
	case errors.Is(err, browser.ErrStructuredReceiptDataMissing), errors.Is(err, coupangreceipts.ErrReceiptDataMissing):
		code = "structured_receipt_data_missing"
		message = "the authenticated receipt read endpoint did not expose the expected structure"
	case errors.Is(err, core.ErrVendorReceiptNotFound):
		code = "vendor_receipt_not_found"
		message = "the hashed order reference was not found within the requested page bound"
	case errors.Is(err, receiptworkflow.ErrSourceUnavailable):
		code = "receipt_source_unavailable"
		message = "the authenticated receipt source was temporarily unavailable"
	case errors.Is(err, browser.ErrStructuredProductDataMissing), errors.Is(err, coupangproducts.ErrProductDataMissing):
		code = "structured_product_data_missing"
		message = "the product search or detail document did not expose the expected structured fields"
	case errors.Is(err, browser.ErrBrowserAccessDenied):
		code = "browser_access_denied"
		message = "browser access was denied; retry later or explicitly choose a supported headed or current-browser mode"
	case errors.Is(err, browser.ErrOrdinaryRendezvous), errors.Is(err, browser.ErrOrdinaryBrowserUnavailable):
		code = "ordinary_browser_unavailable"
		message = "open the Coupang order-history page in ordinary Chrome and click the coupangctl extension before the pairing window expires"
	case errors.Is(err, browserbridge.ErrInstallationConflict):
		code = "browser_bridge_installation_conflict"
		message = "run coupangctl browser-bridge doctor and resolve the reported local installation conflict"
	case errors.Is(err, browser.ErrLocalPageRenderFailed):
		code = "recap_image_render_failed"
		message = "the installed browser could not render the local recap image"
	case errors.Is(err, productworkflow.ErrSourceUnavailable):
		code = "product_source_unavailable"
		message = "the public product source was temporarily unavailable; the request may be retried"
	case errors.Is(err, productworkflow.ErrPriceHistoryUnavailable):
		code = "product_price_history_unavailable"
		message = "the local product price history was unavailable"
	case errors.Is(err, productworkflow.ErrPriceWatchRequiresObservation):
		code = "product_price_watch_requires_observation"
		message = "observe this exact product option with search or inspect before adding it to the watchlist"
	case errors.Is(err, productworkflow.ErrPriceWatchUnavailable):
		code = "product_price_watch_unavailable"
		message = "the local product price watchlist was unavailable"
	case errors.Is(err, browser.ErrQRExpired):
		code = "qr_expired"
		message = "the QR login expired; run auth login again"
	case errors.Is(err, browser.ErrQRLoginTimedOut):
		code = "qr_login_timeout"
		message = "the QR login was not approved before timeout"
	case errors.Is(err, browser.ErrQRUnexpectedDestination):
		code = "qr_return_context_missing"
		message = "QR approval did not return to the protected order page"
	case errors.Is(err, browser.ErrQRLinkUnavailable):
		code = "qr_link_unavailable"
		message = "the ephemeral QR app link could not be decoded; retry without --link or use --qr-output"
	case errors.Is(err, browser.ErrPhoneRequestUnverified):
		code = "otp_request_unverified"
		message = "the SMS request was not confirmed by the login page"
	case errors.Is(err, browser.ErrPhoneSystemError):
		code = "phone_login_system_error"
		message = "the phone login page reported a system error"
	case errors.Is(err, browser.ErrPhoneVerificationFailed):
		code = "otp_verification_failed"
		message = "the SMS OTP was rejected or could not be submitted"
	case errors.Is(err, browser.ErrPhoneLoginTimedOut):
		code = "phone_login_timeout"
		message = "phone login did not complete before timeout"
	case errors.Is(err, browser.ErrPhoneUnexpectedDestination):
		code = "phone_return_context_missing"
		message = "phone login did not return to the protected order page"
	case errors.Is(err, loginassist.ErrAccessibilityPermissionRequired):
		code = "accessibility_permission_required"
		message = "allow accessibility control for coupangctl, then retry auth resend"
	case errors.Is(err, loginassist.ErrDedicatedBrowserNotRunning):
		code = "dedicated_browser_not_running"
		message = "run coupangctl auth login before requesting an OTP resend"
	case errors.Is(err, loginassist.ErrResendControlNotFound):
		code = "otp_resend_control_not_found"
		message = "open the phone-number login step in the dedicated browser first"
	case errors.Is(err, loginassist.ErrResendOutcomeUnverified):
		code = "otp_resend_outcome_unverified"
		message = "the OTP control was pressed but the browser did not remain on the OTP verification step"
	case errors.Is(err, loginassist.ErrUnsupported):
		code = "login_assistance_unsupported"
		message = "OTP resend assistance is not available on this platform"
	case errors.Is(err, orderworkflow.ErrDocumentSource):
		code = "document_source_unavailable"
		message = "the protected order document could not be loaded"
	case errors.Is(err, core.ErrInvalidOrderData):
		code = "invalid_order_data"
		message = "the upstream order document has an unsupported shape"
	case errors.Is(err, core.ErrInvalidLoginMode), errors.Is(err, core.ErrInvalidLoginRequest):
		code = "invalid_command"
		message = "usage: coupangctl auth login [--qr|--phone] [--qr-output PATH|--link]"
	case err != nil && strings.HasPrefix(err.Error(), "usage:"):
		code = "invalid_command"
		message = err.Error()
	case err != nil && err.Error() == "invalid command":
		code = "invalid_command"
		message = "see the usage message"
	}
	_ = writeJSON(w, core.ErrorResponse{Error: core.ErrorBody{Code: code, Message: message}})
}
