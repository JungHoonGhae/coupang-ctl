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

	"github.com/JungHoonGhae/coupang-ctl/internal/auth"
	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/recap"
	receiptworkflow "github.com/JungHoonGhae/coupang-ctl/internal/receipts"
)

const orderSyncUsage = "usage: coupangctl orders sync [--max-pages N] [--restart-scan]"

type helpResponse struct {
	SchemaVersion int           `json:"schema_version"`
	Name          string        `json:"name"`
	Usage         string        `json:"usage"`
	Commands      []helpCommand `json:"commands"`
}

type helpCommand struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

func Run(ctx context.Context, args []string, stdout, stderr io.Writer, version string) error {
	if len(args) == 0 || (len(args) == 1 && isTopLevelHelp(args[0])) {
		return writeTopLevelHelp(stdout)
	}
	args = expandConvenienceCommand(args)
	if legacyBrowserRequested(args) {
		return errLegacyBrowserRetired
	}
	// Browser choice is no longer a configuration-dependent fallback chain.
	// Source reads always use the dedicated Camofox profile.
	switch args[0] {
	case "camofox":
		return runCamofoxSetup(ctx, args[1:], stdout)
	case "version":
		return writeJSON(stdout, map[string]string{"name": "coupangctl", "version": version})
	case "capabilities":
		if len(args) != 1 {
			return core.WithErrorCode("invalid_command", errors.New("usage: coupangctl capabilities"))
		}
		return writeJSON(stdout, core.CurrentCapabilities())
	case "products":
		if len(args) == 3 && isFlagHelp(args[2:]) {
			switch args[1] {
			case "search", "inspect", "recommend":
				return runProducts(ctx, args[1:], stdout, nil)
			}
		}
		if len(args) >= 2 && args[1] == "report" {
			return runProductReport(ctx, args[2:], os.Stdin, stdout)
		}
		if len(args) >= 2 && args[1] == "watch-schedule" {
			executable, err := os.Executable()
			if err != nil {
				return err
			}
			executable, err = filepath.Abs(executable)
			if err != nil {
				return err
			}
			return runProductWatchSchedule(args[2:], stdout, executable)
		}
		return runCamofox(ctx, args, stdout, stderr, version)
	case "orders":
		if len(args) == 3 && args[1] == "stats" && isFlagHelp(args[2:]) {
			return runOrders(ctx, args[1:], stdout, nil)
		}
		return runCamofox(ctx, args, stdout, stderr, version)
	case "auth", "mcp", "doctor", "account", "receipts":
		return runCamofox(ctx, args, stdout, stderr, version)
	default:
		return usage(stderr)
	}
}

var errLegacyBrowserRetired = core.NewError("browser_mode_retired")

func legacyBrowserRequested(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "current-browser", "browser-bridge", "shopping-connection":
		return true
	}
	if strings.HasPrefix(args[0], "chrome-extension://") {
		return true
	}
	for _, arg := range args {
		for _, flag := range []string{"--aside", "--headed", "--current-browser", "--ordinary-browser", "--apple-events"} {
			if arg == flag || strings.HasPrefix(arg, flag+"=") {
				return true
			}
		}
	}
	return false
}
func isTopLevelHelp(arg string) bool {
	return arg == "help" || arg == "--help" || arg == "-h"
}

func writeTopLevelHelp(w io.Writer) error {
	return writeJSON(w, helpResponse{
		SchemaVersion: 1,
		Name:          "coupangctl",
		Usage:         "coupangctl <command> [options]",
		Commands: []helpCommand{
			{Name: "auth", Summary: "sign in, verify, or assist an interactive login"},
			{Name: "camofox", Summary: "configure the dedicated default Camofox runtime without starting a browser"},
			{Name: "orders", Summary: "preview current orders without a database, sync orders, or analyze local history"},
			{Name: "products", Summary: "search, inspect, research recommendations, render reports, and watch prices through Camofox"},
			{Name: "doctor", Summary: "check local Camofox installation without starting a browser or checking authentication"},
			{Name: "capabilities", Summary: "report implemented and externally blocked capabilities"},
			{Name: "version", Summary: "print the installed coupangctl version"},
			{Name: "mcp", Summary: "run the MCP adapter over standard input and output"},
		},
	})
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
	case "login":
		expanded := make([]string, 0, len(args)+1)
		expanded = append(expanded, "auth", "ensure")
		return append(expanded, args[1:]...)
	default:
		return args
	}
	expanded := make([]string, 0, len(args)+1)
	expanded = append(expanded, "orders", subcommand)
	return append(expanded, args[1:]...)
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
		return core.WithErrorCode("invalid_command", errors.New("usage: coupangctl receipts <status|list|summary|overview|vendor|download>"))
	}
	switch args[0] {
	case "status":
		flags := newFlagSet("receipts status")
		if err := parseFlags(flags, args[1:], "usage: coupangctl receipts status"); err != nil {
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
		const commandUsage = "usage: coupangctl receipts overview --from YYYY-MM-DD --to YYYY-MM-DD [--max-cards N]"
		if err := parseFlags(flags, args[1:], commandUsage); err != nil || *from == "" || *to == "" {
			return core.WithErrorCode("invalid_command", errors.New(commandUsage))
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
		const commandUsage = "usage: coupangctl receipts list --kind <cash|card> [--page N] [--size N]"
		if err := parseFlags(flags, args[1:], commandUsage); err != nil || *kind == "" {
			return core.WithErrorCode("invalid_command", errors.New(commandUsage))
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
		const commandUsage = "usage: coupangctl receipts summary --kind <cash|card> --from YYYY-MM-DD --to YYYY-MM-DD [--max-cards N]"
		if err := parseFlags(flags, args[1:], commandUsage); err != nil || *kind == "" || *from == "" || *to == "" {
			return core.WithErrorCode("invalid_command", errors.New(commandUsage))
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
		const commandUsage = "usage: coupangctl receipts download --kind <cash|card> --history-index N --output PATH [--download-index N] [--page N] [--size N]"
		if err := parseFlags(flags, args[1:], commandUsage); err != nil || *kind == "" || *historyIndex < 0 || *output == "" {
			return core.WithErrorCode("invalid_command", errors.New(commandUsage))
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
		const commandUsage = "usage: coupangctl receipts vendor --source-ref HASH [--max-pages N]"
		if err := parseFlags(flags, args[1:], commandUsage); err != nil || *sourceRef == "" {
			return core.WithErrorCode("invalid_command", errors.New(commandUsage))
		}
		result, err := workflow.Vendor(ctx, core.VendorReceiptRequest{SourceRef: *sourceRef, MaxPages: *maxPages})
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	default:
		return core.WithErrorCode("invalid_command", errors.New("usage: coupangctl receipts <status|list|summary|overview|vendor|download>"))
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
		return core.WithErrorCode("invalid_command", errors.New("usage: coupangctl account benefits [--cash-pages N]"))
	}
	flags := newFlagSet("account benefits")
	cashPages := flags.Int("cash-pages", 50, "maximum Coupang Cash transaction pages")
	const commandUsage = "usage: coupangctl account benefits [--cash-pages N]"
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
		return core.WithErrorCode("invalid_command", errors.New("usage: coupangctl products <search|recommend|report|inspect|price-history|price-history-purge|watch-add|watch-list|watch-remove|watch-clear|watch-refresh|watch-schedule|cart-add>"))
	}
	switch args[0] {
	case "recommend":
		return runProductRecommendation(ctx, args[1:], stdout, workflow)
	case "report":
		return runProductReport(ctx, args[1:], os.Stdin, stdout)
	case "search":
		flags := newFlagSet("products search")
		query := flags.String("query", "", "natural-language product query")
		var categoryTrail []string
		flags.Func("category-trail", "repeatable previously observed category label; replay before the active --facet 카테고리=LABEL on query searches", func(value string) error {
			categoryTrail = append(categoryTrail, value)
			return nil
		})
		var facetSelections []core.ProductFacetSelection
		flags.Func("facet", "repeatable observed sidebar choice GROUP=LABEL; discover coverage.facets first (Camofox)", func(value string) error {
			parts := strings.SplitN(value, "=", 2)
			if len(parts) != 2 {
				return errors.New("facet must be GROUP=LABEL")
			}
			facetSelections = append(facetSelections, core.ProductFacetSelection{Name: parts[0], Label: parts[1]})
			return nil
		})
		categoryID := flags.String("category-id", "", "source-native Coupang category identifier")
		limit := flags.Int("limit", 10, "maximum results")
		minPrice := flags.Int64("min-price", 0, "minimum current price in KRW")
		maxPrice := flags.Int64("max-price", 0, "maximum current price in KRW")
		minRating := flags.Float64("min-rating", 0, "minimum rating")
		minReviews := flags.Int("min-reviews", 0, "minimum review count")
		rocket := flags.Bool("rocket", false, "only Rocket items")
		freeShipping := flags.Bool("free-shipping", false, "only explicitly free-shipping items")
		excludeSponsored := flags.Bool("exclude-sponsored", false, "exclude sponsored items")
		minMemoryGB := flags.Int("min-memory-gb", 0, "title-heuristic memory discovery prefilter in GB; verify the exact option")
		minStorageGB := flags.Int("min-storage-gb", 0, "title-heuristic storage discovery prefilter in GB; verify the exact option")
		excludeUsed := flags.Bool("exclude-used", false, "exclude explicitly used, refurbished, or display-unit items")
		includeVariants := flags.Bool("include-variants", false, "return multiple options from the same product page")
		noAffiliate := flags.Bool("no-affiliate", false, "return canonical Coupang URLs only")
		sortOrder := flags.String("sort", "relevance", "relevance, coupang_ranking, sales, latest, price_asc, price_desc, rating, or review_count")
		const searchUsage = "usage: coupangctl products search (--query TEXT | --category-id ID) [--category-trail LABEL] [--facet GROUP=LABEL] [--limit N] [--max-price KRW] [--min-rating N] [--min-reviews N] [--min-memory-gb N] [--min-storage-gb N] [--exclude-used] [--include-variants] [--rocket] [--free-shipping] [--exclude-sponsored] [--sort ORDER] [--no-affiliate]"
		if isFlagHelp(args[1:]) {
			return writeCommandFlagHelp(stdout, flags, searchUsage)
		}
		if err := parseFlags(flags, args[1:], searchUsage); err != nil || (strings.TrimSpace(*query) == "" && *categoryID == "") {
			return core.WithErrorCode("invalid_command", errors.New(searchUsage))
		}
		result, err := workflow.Search(ctx, core.ProductSearchRequest{
			CategoryTrail:   categoryTrail,
			FacetSelections: facetSelections,
			Query:           *query, CategoryID: *categoryID, Limit: *limit, MinPrice: *minPrice, MaxPrice: *maxPrice,
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
		const inspectUsage = "usage: coupangctl products inspect --product-id ID [--item-id ID] [--vendor-item-id ID] [--review-limit N] [--detail-image-limit N] [--no-affiliate]"
		if isFlagHelp(args[1:]) {
			return writeCommandFlagHelp(stdout, flags, inspectUsage)
		}
		if err := parseFlags(flags, args[1:], inspectUsage); err != nil || *productID == "" {
			return core.WithErrorCode("invalid_command", errors.New(inspectUsage))
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
			return core.WithErrorCode("invalid_command", errors.New(historyUsage))
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
			return core.WithErrorCode("invalid_command", errors.New(purgeUsage))
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
			return core.WithErrorCode("invalid_command", errors.New(watchAddUsage))
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
			return core.WithErrorCode("invalid_command", errors.New(watchRemoveUsage))
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
			return core.WithErrorCode("invalid_command", errors.New(watchClearUsage))
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
		const watchRefreshUsage = "usage: coupangctl products watch-refresh [--limit N] [--stale-hours N]"
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
		const cartUsage = "usage: coupangctl products cart-add --product-id ID --vendor-item-id ID [--item-id ID] [--quantity N] --confirm-add-to-cart"
		if err := parseFlags(flags, args[1:], cartUsage); err != nil || *productID == "" || *vendorItemID == "" || !*confirmed {
			return core.WithErrorCode("invalid_command", errors.New(cartUsage))
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
		return core.WithErrorCode("invalid_command", errors.New("usage: coupangctl products <search|recommend|report|inspect|price-history|price-history-purge|watch-add|watch-list|watch-remove|watch-clear|watch-refresh|watch-schedule|cart-add>"))
	}
}

type orderWorkflow interface {
	Sync(context.Context, core.SyncRequest) (core.SyncResult, error)
	SyncStatus(context.Context) (core.SyncStatus, error)
	EnrichCategories(context.Context, core.CategoryEnrichmentRequest) (core.CategoryEnrichmentResult, error)
	CategoryCatalog(context.Context, core.CategoryCatalogRequest) (core.CategoryCatalog, error)
	CategoryStability(context.Context) (core.CategoryStabilityReport, error)
	List(context.Context, core.OrderFilter) ([]core.Order, error)
	Spend(context.Context, core.OrderFilter) (core.SpendSummary, error)
	Stats(context.Context, core.OrderFilter) (core.OrderStats, error)
	Insights(context.Context, core.OrderFilter) (core.ShoppingInsights, error)
	ShoppingAnalysis(context.Context, core.OrderFilter, bool) (core.ShoppingAnalysis, error)
	ProductInsights(context.Context, core.OrderFilter) (core.ProductInsights, error)
	ReorderCandidates(context.Context, core.OrderFilter) ([]core.ReorderCandidate, error)
	Export(context.Context, core.OrderFilter) (core.OrderExport, error)
	Purge(context.Context) (core.PurgeResult, error)
	Import(context.Context, core.OrderExport) (core.UpsertResult, error)
}

func runOrders(ctx context.Context, args []string, stdout io.Writer, workflow orderWorkflow) error {
	if len(args) == 0 {
		return core.WithErrorCode("invalid_command", errors.New("usage: coupangctl orders <preview|sync|sync-status|categories|category-catalog|category-stability|list|spend|stats|insights|products|recap|recap-image|reorder|export|import|purge>"))
	}
	switch args[0] {
	case "sync":
		flags := newFlagSet("orders sync")
		maxPages := flags.Int("max-pages", 100, "maximum pages to process")
		restartScan := flags.Bool("restart-scan", false, "explicitly begin a new scan; retain existing orders and prior scan evidence")
		if err := parseFlags(flags, args[1:], orderSyncUsage); err != nil {
			return err
		}
		result, err := workflow.Sync(ctx, core.SyncRequest{MaxPages: *maxPages, RestartScan: *restartScan})
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "sync-status":
		if len(args) != 1 {
			return core.WithErrorCode("invalid_command", errors.New("usage: coupangctl orders sync-status"))
		}
		result, err := workflow.SyncStatus(ctx)
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "categories":
		flags := newFlagSet("orders categories")
		maxProducts := flags.Int("max-products", 25, "maximum uncached products to enrich")
		recheck := flags.Bool("recheck", false, "explicitly re-read cached breadcrumbs, oldest first")
		if err := parseFlags(flags, args[1:], "usage: coupangctl orders categories [--max-products N] [--recheck]"); err != nil {
			return err
		}
		result, err := workflow.EnrichCategories(ctx, core.CategoryEnrichmentRequest{MaxProducts: *maxProducts, Recheck: *recheck})
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "category-stability":
		if len(args) != 1 {
			return core.WithErrorCode("invalid_command", errors.New("usage: coupangctl orders category-stability"))
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
		const statsUsage = "usage: coupangctl orders stats [--from YYYY-MM-DD] [--to YYYY-MM-DD]"
		if isFlagHelp(args[1:]) {
			flags, _ := orderFilterFlags(false)
			return writeCommandFlagHelp(stdout, flags, statsUsage)
		}
		filter, err := parseOrderFilter(args[1:], false, statsUsage)
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
			return core.WithErrorCode("invalid_command", errors.New("usage: coupangctl orders recap --output PATH [--from YYYY-MM-DD] [--to YYYY-MM-DD] [--include-products]"))
		}
		filter := core.OrderFilter{From: *from, To: *to}
		analysis, err := workflow.ShoppingAnalysis(ctx, filter, *includeProducts)
		if err != nil {
			return err
		}
		options := recap.Options{Products: analysis.Products}
		written, err := recap.WriteNewFileWithOptions(*output, analysis.Insights, options)
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
			return core.WithErrorCode("invalid_command", errors.New(imageUsage))
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
			return core.WithErrorCode("invalid_command", errors.New("usage: coupangctl orders import --file PATH"))
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
			return core.WithErrorCode("invalid_command", errors.New("usage: coupangctl orders purge --confirm purge-normalized-orders"))
		}
		result, err := workflow.Purge(ctx)
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	default:
		return core.WithErrorCode("invalid_command", errors.New("usage: coupangctl orders <preview|sync|sync-status|categories|category-catalog|category-stability|list|spend|stats|insights|products|recap|recap-image|reorder|export|import|purge>"))
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

func orderFilterFlags(allowLimit bool) (*flag.FlagSet, *core.OrderFilter) {
	flags := newFlagSet("orders filter")
	filter := &core.OrderFilter{}
	flags.StringVar(&filter.From, "from", "", "start date")
	flags.StringVar(&filter.To, "to", "", "end date")
	if allowLimit {
		flags.IntVar(&filter.Limit, "limit", 100, "maximum results")
	}
	return flags, filter
}

func parseOrderFilter(args []string, allowLimit bool, usage string) (core.OrderFilter, error) {
	flags, filter := orderFilterFlags(allowLimit)
	if err := parseFlags(flags, args, usage); err != nil {
		return core.OrderFilter{}, err
	}
	return *filter, nil
}

func newFlagSet(name string) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	return flags
}

func isFlagHelp(args []string) bool {
	return len(args) == 1 && (args[0] == "--help" || args[0] == "-h")
}

// Render the registered options instead of maintaining a parallel help catalog.
func writeCommandFlagHelp(w io.Writer, flags *flag.FlagSet, usage string) error {
	type optionHelp struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Default     string `json:"default"`
	}
	options := []optionHelp{}
	flags.VisitAll(func(f *flag.Flag) {
		options = append(options, optionHelp{Name: "--" + f.Name, Description: f.Usage, Default: f.DefValue})
	})
	return writeJSON(w, map[string]any{"schema_version": 1, "name": "coupangctl", "usage": strings.TrimPrefix(usage, "usage: "), "options": options})
}

func parseFlags(flags *flag.FlagSet, args []string, usage string) error {
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return core.WithErrorCode("invalid_command", errors.New(usage))
	}
	return nil
}

func runAuth(ctx context.Context, args []string, stdout io.Writer, service *auth.Service) error {
	if len(args) != 1 {
		return core.WithErrorCode("invalid_command", errors.New("usage: coupangctl auth <status|verify>"))
	}
	var status core.AuthStatus
	var err error
	switch args[0] {
	case "status":
		status, err = service.Status(ctx)
	case "verify":
		status, err = service.Verify(ctx)
	default:
		return core.WithErrorCode("invalid_command", errors.New("usage: coupangctl auth <status|verify>"))
	}
	if err != nil {
		return err
	}
	return writeJSON(stdout, status)
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
	return core.WithErrorCode("invalid_command", errors.New("invalid command"))
}
