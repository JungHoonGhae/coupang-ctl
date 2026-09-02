package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	accountworkflow "github.com/JungHoonGhae/oss-coupangctl/internal/account"
	"github.com/JungHoonGhae/oss-coupangctl/internal/auth"
	"github.com/JungHoonGhae/oss-coupangctl/internal/browser"
	"github.com/JungHoonGhae/oss-coupangctl/internal/core"
	coupangaccount "github.com/JungHoonGhae/oss-coupangctl/internal/coupang/account"
	coupangproducts "github.com/JungHoonGhae/oss-coupangctl/internal/coupang/products"
	"github.com/JungHoonGhae/oss-coupangctl/internal/loginassist"
	"github.com/JungHoonGhae/oss-coupangctl/internal/mcpserver"
	orderworkflow "github.com/JungHoonGhae/oss-coupangctl/internal/orders"
	"github.com/JungHoonGhae/oss-coupangctl/internal/partners"
	"github.com/JungHoonGhae/oss-coupangctl/internal/platform"
	productworkflow "github.com/JungHoonGhae/oss-coupangctl/internal/products"
	"github.com/JungHoonGhae/oss-coupangctl/internal/recap"
	"github.com/JungHoonGhae/oss-coupangctl/internal/store"
)

func Run(ctx context.Context, args []string, stdout, stderr io.Writer, version string) error {
	if len(args) == 0 {
		return usage(stderr)
	}
	if args[0] == "version" {
		return writeJSON(stdout, map[string]string{"name": "coupangctl", "version": version})
	}
	if args[0] == "capabilities" {
		if len(args) != 1 {
			return errors.New("usage: coupangctl capabilities")
		}
		return writeJSON(stdout, core.CurrentCapabilities())
	}

	paths, err := platform.DefaultPaths()
	if err != nil {
		return err
	}
	browserAdapter := browser.NewNative(paths.ProfileDir)
	if headedReadRequested(args) {
		browserAdapter = browser.NewNativeHeadedSync(paths.ProfileDir)
	}
	defer browserAdapter.Close()
	authService := auth.NewService(browserAdapter)
	productService := productworkflow.NewWithAffiliate(coupangproducts.New(browserAdapter), partners.NewFromEnvironment(os.Getenv))
	accountService := accountworkflow.New(coupangaccount.New(browserAdapter))

	switch args[0] {
	case "doctor":
		return runDoctor(ctx, stdout, paths, browserAdapter)
	case "auth":
		return runAuth(ctx, args[1:], stdout, stderr, authService, loginassist.New(paths.ProfileDir), newTerminalLoginSecrets(os.Stdin, stderr))
	case "orders":
		ledger, err := store.Open(ctx, paths.Database)
		if err != nil {
			return err
		}
		defer ledger.Close()
		return runOrders(ctx, args[1:], stdout, orderworkflow.New(ledger, browserAdapter))
	case "products":
		return runProducts(ctx, args[1:], stdout, productService)
	case "account":
		return runAccount(ctx, args[1:], stdout, accountService)
	case "mcp":
		if len(args) != 1 {
			return errors.New("usage: coupangctl mcp")
		}
		ledger, err := store.Open(ctx, paths.Database)
		if err != nil {
			return err
		}
		defer ledger.Close()
		return mcpserver.RunWithAllFeatures(ctx, authService, orderworkflow.New(ledger, browserAdapter), productService, accountService, version)
	default:
		return usage(stderr)
	}
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
	AddToCart(context.Context, core.CartAddRequest) (core.CartAddResult, error)
}

func runProducts(ctx context.Context, args []string, stdout io.Writer, workflow productWorkflow) error {
	if len(args) == 0 {
		return errors.New("usage: coupangctl products <search|inspect|cart-add>")
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
		return errors.New("usage: coupangctl products <search|inspect|cart-add>")
	}
}

type orderWorkflow interface {
	Sync(context.Context, core.SyncRequest) (core.SyncResult, error)
	EnrichCategories(context.Context, core.CategoryEnrichmentRequest) (core.CategoryEnrichmentResult, error)
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
		return errors.New("usage: coupangctl orders <sync|categories|list|spend|stats|insights|products|recap|reorder|export|import|purge>")
	}
	switch args[0] {
	case "sync":
		flags := newFlagSet("orders sync")
		maxPages := flags.Int("max-pages", 100, "maximum pages to process")
		_ = flags.Bool("headed", false, "use headed browser fallback")
		if err := parseFlags(flags, args[1:], "usage: coupangctl orders sync [--max-pages N] [--headed]"); err != nil {
			return err
		}
		result, err := workflow.Sync(ctx, core.SyncRequest{MaxPages: *maxPages})
		if err != nil {
			return err
		}
		return writeJSON(stdout, result)
	case "categories":
		flags := newFlagSet("orders categories")
		maxProducts := flags.Int("max-products", 25, "maximum uncached products to enrich")
		_ = flags.Bool("headed", false, "use a headed browser")
		if err := parseFlags(flags, args[1:], "usage: coupangctl orders categories [--max-products N] [--headed]"); err != nil {
			return err
		}
		result, err := workflow.EnrichCategories(ctx, core.CategoryEnrichmentRequest{MaxProducts: *maxProducts})
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
	case "reorder":
		filter, err := parseOrderFilter(args[1:], true, "usage: coupangctl orders reorder [--from YYYY-MM-DD] [--to YYYY-MM-DD] [--limit N]")
		if err != nil {
			return err
		}
		candidates, err := workflow.ReorderCandidates(ctx, filter)
		if err != nil {
			return err
		}
		return writeJSON(stdout, core.ReorderList{Candidates: candidates})
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
		return errors.New("usage: coupangctl orders <sync|categories|list|spend|stats|insights|products|recap|reorder|export|import|purge>")
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
		(args[0] == "products" && (args[1] == "search" || args[1] == "inspect" || args[1] == "cart-add")) ||
		(args[0] == "account" && args[1] == "benefits")
	if !eligible {
		return false
	}
	for _, argument := range args[2:] {
		if argument == "--headed" {
			return true
		}
	}
	return false
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

func runDoctor(ctx context.Context, stdout io.Writer, paths platform.Paths, native *browser.Native) error {
	report := core.DoctorReport{OK: true}
	if _, err := native.Inspect(ctx); err != nil {
		report.OK = false
		report.Checks = append(report.Checks, core.Check{Name: "browser", Status: core.CheckError, Message: "a supported browser is unavailable or misconfigured"})
	} else {
		report.Checks = append(report.Checks, core.Check{Name: "browser", Status: core.CheckOK})
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
	case errors.Is(err, browser.ErrAuthenticationRequired):
		code = "authentication_required"
		message = "run coupangctl auth login on an interactive desktop first"
	case errors.Is(err, browser.ErrStructuredOrderDataMissing):
		code = "structured_order_data_missing"
		message = "the authenticated order document was not available"
	case errors.Is(err, browser.ErrStructuredAccountBenefitsDataMissing):
		code = "structured_account_benefits_data_missing"
		message = "the authenticated membership or reward document was not available"
	case errors.Is(err, browser.ErrStructuredProductDataMissing), errors.Is(err, coupangproducts.ErrProductDataMissing):
		code = "structured_product_data_missing"
		message = "the product search or detail document did not expose the expected structured fields"
	case errors.Is(err, browser.ErrBrowserAccessDenied):
		code = "browser_access_denied"
		message = "headless access was denied; retry the read-only operation with --headed"
	case errors.Is(err, productworkflow.ErrSourceUnavailable):
		code = "product_source_unavailable"
		message = "the public product source was temporarily unavailable; the request may be retried"
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
