package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/oss-coupangctl/internal/auth"
	"github.com/JungHoonGhae/oss-coupangctl/internal/browser"
	"github.com/JungHoonGhae/oss-coupangctl/internal/core"
	productworkflow "github.com/JungHoonGhae/oss-coupangctl/internal/products"
	"github.com/JungHoonGhae/oss-coupangctl/internal/store"
)

type loginModeBrowser struct {
	request    core.LoginRequest
	consumeOTP bool
}

func (b *loginModeBrowser) Inspect(context.Context) (auth.BrowserStatus, error) {
	return auth.BrowserStatus{Name: "synthetic"}, nil
}

func (b *loginModeBrowser) Login(ctx context.Context, request core.LoginRequest) error {
	b.request = request
	if request.PresentQRLink != nil {
		if err := request.PresentQRLink(ctx, core.QRLoginLink{
			URL:          "https://applink.coupang.com/open?url=https%3A%2F%2Flogin.coupang.com%2Flogin%2Fm%2Fqrcode%2Fbind.pang%3FqrCode%3Dsynthetic",
			ApprovalCode: "42",
		}); err != nil {
			return err
		}
	}
	if b.consumeOTP && request.ReadOTP != nil {
		_, _ = request.ReadOTP(context.Background())
	}
	return nil
}

func (b *loginModeBrowser) Verify(context.Context) error { return nil }

type noopResendAssistant struct{}

func (noopResendAssistant) Resend(context.Context) (core.OTPResendResult, error) {
	return core.OTPResendResult{}, nil
}

type fixedLoginSecrets struct {
	phoneCalls int
	otpCalls   int
}

type fixedProductWorkflow struct{}

type capturingProductWorkflow struct {
	searchRequest  core.ProductSearchRequest
	inspectRequest core.ProductInspectRequest
}

type fixedAccountWorkflow struct{}

func (fixedAccountWorkflow) Snapshot(_ context.Context, request core.AccountBenefitsRequest) (core.AccountBenefitsSnapshot, error) {
	return core.AccountBenefitsSnapshot{
		SchemaVersion: 1,
		Membership:    core.WowMembership{Status: "MEMBER", IsMember: true, CurrentMonthlyFeeKRW: 7890},
		Coverage:      core.AccountBenefitsCoverage{CashTransactionPagesRead: request.MaxCashTransactionPages},
	}, nil
}

func (fixedProductWorkflow) Search(_ context.Context, request core.ProductSearchRequest) (core.ProductSearchResult, error) {
	return core.ProductSearchResult{SchemaVersion: 1, Query: request.Query, Currency: "KRW", Items: []core.ProductCard{{Name: "Synthetic result"}}}, nil
}

func (fixedProductWorkflow) Inspect(_ context.Context, request core.ProductInspectRequest) (core.ProductInspection, error) {
	return core.ProductInspection{SchemaVersion: 1, Product: core.ProductCard{Reference: core.ProductReference{ProductID: request.ProductID}}}, nil
}

func (fixedProductWorkflow) AddToCart(_ context.Context, request core.CartAddRequest) (core.CartAddResult, error) {
	return core.CartAddResult{SchemaVersion: 1, Attempted: true, Added: true, Verified: true, Quantity: request.Quantity}, nil
}

func (w *capturingProductWorkflow) Search(_ context.Context, request core.ProductSearchRequest) (core.ProductSearchResult, error) {
	w.searchRequest = request
	return core.ProductSearchResult{SchemaVersion: core.ProductSchemaVersion, Query: request.Query}, nil
}

func (w *capturingProductWorkflow) Inspect(_ context.Context, request core.ProductInspectRequest) (core.ProductInspection, error) {
	w.inspectRequest = request
	return core.ProductInspection{SchemaVersion: core.ProductSchemaVersion}, nil
}

func (*capturingProductWorkflow) AddToCart(context.Context, core.CartAddRequest) (core.CartAddResult, error) {
	return core.CartAddResult{}, nil
}

func TestAccountBenefitsCommandUsesTypedReadWorkflow(t *testing.T) {
	var stdout bytes.Buffer
	if err := runAccount(context.Background(), []string{"benefits", "--cash-pages", "7"}, &stdout, fixedAccountWorkflow{}); err != nil {
		t.Fatal(err)
	}
	var got core.AccountBenefitsSnapshot
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Membership.IsMember || got.Membership.CurrentMonthlyFeeKRW != 7890 || got.Coverage.CashTransactionPagesRead != 7 {
		t.Fatalf("unexpected account benefits response: %#v", got)
	}
}

func (s *fixedLoginSecrets) Phone(context.Context) (string, error) {
	s.phoneCalls++
	return "01000000000", nil
}

func (s *fixedLoginSecrets) OTP(context.Context) (string, error) {
	s.otpCalls++
	return "000000", nil
}

func TestVersionIsStructuredAndHasNoEnvironmentDependency(t *testing.T) {
	t.Setenv("COUPANGCTL_STATE_DIR", "relative-would-fail")
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"version"}, &stdout, &stderr, "v0.1.0-test"); err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["name"] != "coupangctl" || got["version"] != "v0.1.0-test" {
		t.Fatalf("unexpected version response: %#v", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestCapabilitiesAreStructuredAndOrderedByPriority(t *testing.T) {
	t.Setenv("COUPANGCTL_STATE_DIR", "relative-would-fail")
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"capabilities"}, &stdout, &stderr, "test"); err != nil {
		t.Fatal(err)
	}
	var report core.CapabilityReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != 1 || len(report.Capabilities) < 5 || report.Capabilities[0].Priority != "P0" {
		t.Fatalf("unexpected capability report: %#v", report)
	}
	if report.Capabilities[0].Status != core.CapabilityAvailable {
		t.Fatalf("unexpected leading capability status: %#v", report.Capabilities[0])
	}
}

func TestProductsSearchAcceptsNaturalLanguageAndWritesTypedJSON(t *testing.T) {
	var output bytes.Buffer
	err := runProducts(context.Background(), []string{
		"search", "--query", "후기 좋은 10만원 아래 맥북 허브", "--max-price", "100000", "--min-rating", "4.5",
	}, &output, fixedProductWorkflow{})
	if err != nil {
		t.Fatal(err)
	}
	var got core.ProductSearchResult
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Query != "후기 좋은 10만원 아래 맥북 허브" || len(got.Items) != 1 {
		t.Fatalf("unexpected product search output: %#v", got)
	}
}

func TestProductsAffiliateOptOutReachesTypedWorkflow(t *testing.T) {
	workflow := &capturingProductWorkflow{}
	if err := runProducts(context.Background(), []string{"search", "--query", "synthetic", "--no-affiliate"}, io.Discard, workflow); err != nil {
		t.Fatal(err)
	}
	if !workflow.searchRequest.DisableAffiliate {
		t.Fatal("search affiliate opt-out was not propagated")
	}
	if err := runProducts(context.Background(), []string{"inspect", "--product-id", "101", "--no-affiliate"}, io.Discard, workflow); err != nil {
		t.Fatal(err)
	}
	if !workflow.inspectRequest.DisableAffiliate {
		t.Fatal("inspection affiliate opt-out was not propagated")
	}
}

func TestProductsCartAddRequiresExplicitConfirmationFlag(t *testing.T) {
	args := []string{"cart-add", "--product-id", "101", "--vendor-item-id", "301"}
	if err := runProducts(context.Background(), args, io.Discard, fixedProductWorkflow{}); err == nil {
		t.Fatal("cart mutation ran without an explicit confirmation flag")
	}
	args = append(args, "--confirm-add-to-cart")
	if err := runProducts(context.Background(), args, io.Discard, fixedProductWorkflow{}); err != nil {
		t.Fatal(err)
	}
}

func TestOrdersListReturnsDocumentedObjectShape(t *testing.T) {
	ctx := context.Background()
	stateDir := t.TempDir()
	t.Setenv("COUPANGCTL_STATE_DIR", stateDir)
	ledger, err := store.Open(ctx, filepath.Join(stateDir, "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{{
		SourceRef: "synthetic-hash", PurchasedAt: "2026-08-29", TotalAmount: 1200,
		Currency: "KRW", Items: []core.OrderItem{},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	err = Run(ctx, []string{"orders", "list", "--limit", "10"}, &stdout, &stderr, "test")
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Orders []core.Order `json:"orders"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Orders) != 1 || got.Orders[0].TotalAmount != 1200 {
		t.Fatalf("unexpected orders response: %#v", got)
	}
}

func TestOrdersStatsReturnsCancellationAndReturnRates(t *testing.T) {
	ctx := context.Background()
	stateDir := t.TempDir()
	t.Setenv("COUPANGCTL_STATE_DIR", stateDir)
	ledger, err := store.Open(ctx, filepath.Join(stateDir, "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{{
		SourceRef: "synthetic-return", PurchasedAt: "2026-08-29", TotalAmount: 1200,
		Currency: "KRW", Items: []core.OrderItem{{
			Name: "Synthetic returned item", Quantity: 2, ReturnedQuantity: 1,
			UnitPrice: 600, PaidPrice: 1200, DeliveryStatus: "returned",
		}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Run(ctx, []string{"orders", "stats"}, &stdout, &stderr, "test"); err != nil {
		t.Fatal(err)
	}
	var got core.OrderStats
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ReturnedItemLineCount != 1 || got.ReturnedUnits != 1 || got.ReturnedUnitRate != 0.5 {
		t.Fatalf("unexpected stats response: %#v", got)
	}
}

func TestOrdersInsightsReturnsShareableAggregateShape(t *testing.T) {
	ctx := context.Background()
	stateDir := t.TempDir()
	t.Setenv("COUPANGCTL_STATE_DIR", stateDir)
	ledger, err := store.Open(ctx, filepath.Join(stateDir, "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{
		{
			SourceRef: "synthetic-one", PurchasedAt: "2026-08-29",
			TotalAmount: 1200, Currency: "KRW", Items: []core.OrderItem{},
		},
		{
			SourceRef: "synthetic-two", PurchasedAt: "2026-08-29",
			TotalAmount: 800, Currency: "KRW", Items: []core.OrderItem{},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Run(ctx, []string{"orders", "insights"}, &stdout, &stderr, "test"); err != nil {
		t.Fatal(err)
	}
	var got core.ShoppingInsights
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.OrderCount != 2 || got.DistinctOrderDays != 1 || got.MaxOrdersInOneDay != 2 {
		t.Fatalf("unexpected insights response: %#v", got)
	}
	if got.Definitions.RateScale != "0_to_1" {
		t.Fatalf("unexpected insight definitions: %#v", got.Definitions)
	}
}

func TestOrdersProductsReturnsPrivateLocalProductInsightShape(t *testing.T) {
	ctx := context.Background()
	stateDir := t.TempDir()
	t.Setenv("COUPANGCTL_STATE_DIR", stateDir)
	ledger, err := store.Open(ctx, filepath.Join(stateDir, "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{{
		SourceRef: "synthetic-product-insight", PurchasedAt: "2026-08-29", TotalAmount: 4200,
		Currency: "KRW", Items: []core.OrderItem{{
			VendorItemID: "synthetic-vendor-item", Name: "Synthetic private product", Quantity: 2,
			UnitPrice: 2500, PaidPrice: 4200, DeliveryStatus: "delivered",
		}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Run(ctx, []string{"orders", "products"}, &stdout, &stderr, "test"); err != nil {
		t.Fatal(err)
	}
	var got core.ProductInsights
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Visibility != "private_local" || got.TopByUnits.Name != "Synthetic private product" || got.TopByUnits.UnitCount != 2 || got.HighestSpendDay.TotalAmount != 4200 {
		t.Fatalf("unexpected product insights response: %#v", got)
	}
}

func TestOrdersRecapWritesPrivateStandaloneHTMLWithoutEchoingPath(t *testing.T) {
	ctx := context.Background()
	stateDir := t.TempDir()
	t.Setenv("COUPANGCTL_STATE_DIR", stateDir)
	ledger, err := store.Open(ctx, filepath.Join(stateDir, "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{{
		SourceRef: "synthetic-recap", PurchasedAt: "2026-08-29", TotalAmount: 1200,
		Currency: "KRW", Items: []core.OrderItem{},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(t.TempDir(), "shopping-recap.html")
	var stdout, stderr bytes.Buffer
	if err := Run(ctx, []string{"orders", "recap", "--output", outputPath}, &stdout, &stderr, "test"); err != nil {
		t.Fatal(err)
	}
	var got core.RecapWriteResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Written || got.Format != "html" || got.Visibility != "public_safe" || got.Bytes == 0 {
		t.Fatalf("unexpected recap result: %#v", got)
	}
	if bytes.Contains(stdout.Bytes(), []byte(outputPath)) {
		t.Fatalf("structured output exposed local output path: %s", stdout.Bytes())
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("recap permissions = %o, want 600", info.Mode().Perm())
	}
	html, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(html, []byte("<!doctype html>")) {
		t.Fatal("recap output is not standalone HTML")
	}
}

func TestOrdersRecapCanExplicitlyIncludePrivateProductDetails(t *testing.T) {
	ctx := context.Background()
	stateDir := t.TempDir()
	t.Setenv("COUPANGCTL_STATE_DIR", stateDir)
	ledger, err := store.Open(ctx, filepath.Join(stateDir, "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{{
		SourceRef: "synthetic-private-recap", PurchasedAt: "2026-08-29", TotalAmount: 4200,
		Currency: "KRW", Items: []core.OrderItem{{
			VendorItemID: "synthetic-private-product", Name: "Synthetic private recap product",
			Quantity: 2, UnitPrice: 2500, PaidPrice: 4200, DeliveryStatus: "delivered",
		}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(t.TempDir(), "private-shopping-recap.html")
	var stdout, stderr bytes.Buffer
	if err := Run(ctx, []string{"orders", "recap", "--output", outputPath, "--include-products"}, &stdout, &stderr, "test"); err != nil {
		t.Fatal(err)
	}
	var got core.RecapWriteResult
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Visibility != "private_products" {
		t.Fatalf("recap visibility = %q, want private_products", got.Visibility)
	}
	html, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(html, []byte("Synthetic private recap product")) || !bytes.Contains(html, []byte("PRIVATE PRODUCT RECEIPTS")) {
		t.Fatal("private product recap omitted product details")
	}
}

func TestWriteErrorUsesStableSanitizedShape(t *testing.T) {
	var output bytes.Buffer
	WriteError(&output, errors.Join(browser.ErrBrowserNotFound, errors.New("sensitive local path")))
	var got core.ErrorResponse
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Error.Code != "browser_not_found" {
		t.Fatalf("error code = %q", got.Error.Code)
	}
	if bytes.Contains(output.Bytes(), []byte("sensitive local path")) {
		t.Fatalf("error output exposed an internal cause: %s", output.Bytes())
	}
}

func TestWriteErrorUsesTypedProductSourceFailureWithoutLeakingCause(t *testing.T) {
	var output bytes.Buffer
	WriteError(&output, errors.Join(productworkflow.ErrSourceUnavailable, errors.New("sensitive upstream detail")))
	var got core.ErrorResponse
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Error.Code != "product_source_unavailable" {
		t.Fatalf("error code = %q", got.Error.Code)
	}
	if bytes.Contains(output.Bytes(), []byte("sensitive upstream detail")) {
		t.Fatalf("error output exposed an internal cause: %s", output.Bytes())
	}
}

func TestAuthLoginDefaultsToQRAndAcceptsPhoneFallback(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want core.LoginMode
	}{
		{name: "default", args: []string{"login"}, want: core.LoginModeQR},
		{name: "explicit qr", args: []string{"login", "--qr"}, want: core.LoginModeQR},
		{name: "phone fallback", args: []string{"login", "--phone"}, want: core.LoginModePhone},
	} {
		t.Run(test.name, func(t *testing.T) {
			browser := &loginModeBrowser{}
			service := auth.NewService(browser)
			var output bytes.Buffer
			if err := runAuth(context.Background(), test.args, &output, io.Discard, service, noopResendAssistant{}, &fixedLoginSecrets{}); err != nil {
				t.Fatal(err)
			}
			if browser.request.Mode != test.want {
				t.Fatalf("mode = %q, want %q", browser.request.Mode, test.want)
			}
			var result core.LoginResult
			if err := json.Unmarshal(output.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Mode != test.want {
				t.Fatalf("result mode = %q, want %q", result.Mode, test.want)
			}
		})
	}
}

func TestAuthLoginRejectsConflictingModes(t *testing.T) {
	service := auth.NewService(&loginModeBrowser{})
	if err := runAuth(context.Background(), []string{"login", "--qr", "--phone"}, io.Discard, io.Discard, service, noopResendAssistant{}, &fixedLoginSecrets{}); err == nil {
		t.Fatal("conflicting login modes were accepted")
	}
}

func TestAuthLoginPassesQROutputWithoutEchoingLocalPath(t *testing.T) {
	browser := &loginModeBrowser{}
	service := auth.NewService(browser)
	var output bytes.Buffer
	path := filepath.Join(t.TempDir(), "login-qr.png")
	if err := runAuth(context.Background(), []string{"login", "--qr-output", path}, &output, io.Discard, service, noopResendAssistant{}, &fixedLoginSecrets{}); err != nil {
		t.Fatal(err)
	}
	if browser.request.Mode != core.LoginModeQR || browser.request.QROutputPath != path {
		t.Fatalf("unexpected request: %#v", browser.request)
	}
	if bytes.Contains(output.Bytes(), []byte(path)) {
		t.Fatalf("structured output exposed local QR path: %s", output.Bytes())
	}
}

func TestAuthLoginLinkWritesEphemeralMaterialOnlyToExplicitStderr(t *testing.T) {
	browser := &loginModeBrowser{}
	service := auth.NewService(browser)
	var stdout, stderr bytes.Buffer
	if err := runAuth(context.Background(), []string{"login", "--link"}, &stdout, &stderr, service, noopResendAssistant{}, &fixedLoginSecrets{}); err != nil {
		t.Fatal(err)
	}
	if browser.request.PresentQRLink == nil {
		t.Fatal("QR link presenter was not configured")
	}
	if !bytes.Contains(stderr.Bytes(), []byte("applink.coupang.com")) || !bytes.Contains(stderr.Bytes(), []byte("Approval number: 42")) {
		t.Fatalf("explicit QR link presentation is incomplete")
	}
	if bytes.Contains(stdout.Bytes(), []byte("applink.coupang.com")) || bytes.Contains(stdout.Bytes(), []byte("42")) {
		t.Fatalf("structured output exposed ephemeral QR material: %s", stdout.Bytes())
	}
	var result core.LoginResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result.State != core.AuthVerified {
		t.Fatalf("unexpected structured login result: %v %#v", err, result)
	}
}

func TestAuthLoginRejectsMultipleQRPresentationChannels(t *testing.T) {
	service := auth.NewService(&loginModeBrowser{})
	if err := runAuth(context.Background(), []string{"login", "--link", "--qr-output", "synthetic.png"}, io.Discard, io.Discard, service, noopResendAssistant{}, &fixedLoginSecrets{}); err == nil {
		t.Fatal("multiple QR presentation channels were accepted")
	}
}

func TestPhoneLoginReadsPhoneBeforeBrowserAndOTPOnlyOnBrowserRequest(t *testing.T) {
	secrets := &fixedLoginSecrets{}
	browser := &loginModeBrowser{consumeOTP: true}
	service := auth.NewService(browser)
	var output bytes.Buffer
	if err := runAuth(context.Background(), []string{"login", "--phone"}, &output, io.Discard, service, noopResendAssistant{}, secrets); err != nil {
		t.Fatal(err)
	}
	if secrets.phoneCalls != 1 || secrets.otpCalls != 1 {
		t.Fatalf("secret reads = phone:%d otp:%d", secrets.phoneCalls, secrets.otpCalls)
	}
	if browser.request.Phone == "" || browser.request.ReadOTP == nil {
		t.Fatal("browser did not receive private phone login inputs")
	}
	if bytes.Contains(output.Bytes(), []byte("01000000000")) || bytes.Contains(output.Bytes(), []byte("000000")) {
		t.Fatalf("structured output exposed phone login secrets: %s", output.Bytes())
	}
}

func TestWriteErrorClassifiesQRExpiryWithoutInternalDetails(t *testing.T) {
	var output bytes.Buffer
	WriteError(&output, errors.Join(browser.ErrQRExpired, errors.New("sensitive QR material")))
	var got core.ErrorResponse
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Error.Code != "qr_expired" {
		t.Fatalf("error code = %q", got.Error.Code)
	}
	if bytes.Contains(output.Bytes(), []byte("sensitive QR material")) {
		t.Fatalf("error output exposed internal details: %s", output.Bytes())
	}
}
