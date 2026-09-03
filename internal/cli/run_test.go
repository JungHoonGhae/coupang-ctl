package cli

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/auth"
	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
	"github.com/JungHoonGhae/coupang-ctl/internal/browserbridge"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	orderworkflow "github.com/JungHoonGhae/coupang-ctl/internal/orders"
	"github.com/JungHoonGhae/coupang-ctl/internal/platform"
	productworkflow "github.com/JungHoonGhae/coupang-ctl/internal/products"
	receiptworkflow "github.com/JungHoonGhae/coupang-ctl/internal/receipts"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
)

type loginModeBrowser struct {
	request        core.LoginRequest
	consumeOTP     bool
	profilePresent bool
	verifyErr      error
	loginCalls     int
	verifyCalls    int
}

func (b *loginModeBrowser) Inspect(context.Context) (auth.BrowserStatus, error) {
	return auth.BrowserStatus{Name: "synthetic", ProfilePresent: b.profilePresent}, nil
}

func (b *loginModeBrowser) Login(ctx context.Context, request core.LoginRequest) error {
	b.loginCalls++
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

func (b *loginModeBrowser) Verify(context.Context) error {
	b.verifyCalls++
	return b.verifyErr
}

type blockedStatusBrowser struct{}

func (blockedStatusBrowser) Inspect(context.Context) (auth.BrowserStatus, error) {
	return auth.BrowserStatus{Name: "Synthetic Chrome", ProfilePresent: true}, nil
}

func (blockedStatusBrowser) Login(context.Context, core.LoginRequest) error { return nil }

func (blockedStatusBrowser) Verify(context.Context) error { return core.ErrBrowserAccessDenied }

type fixedDoctorAuthStatus struct {
	status core.AuthStatus
	err    error
}

type fixedCurrentBrowserStatusProvider struct {
	status core.CurrentBrowserStatus
}

func (f fixedCurrentBrowserStatusProvider) Status(context.Context) (core.CurrentBrowserStatus, error) {
	return f.status, nil
}

func (f fixedDoctorAuthStatus) Status(context.Context) (core.AuthStatus, error) {
	return f.status, f.err
}

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

type fixedReceiptWorkflow struct{}

func (fixedReceiptWorkflow) Status(context.Context) (core.ReceiptRequestStatusSnapshot, error) {
	return core.ReceiptRequestStatusSnapshot{SchemaVersion: core.ReceiptSchemaVersion, Statuses: []core.ReceiptRequestStatus{{Kind: core.ReceiptKindCard, Availability: "possible", CanRequestNew: true, RequestInProgressStatus: "unavailable"}}}, nil
}

func (fixedReceiptWorkflow) History(_ context.Context, request core.ReceiptHistoryRequest) (core.ReceiptHistoryPage, error) {
	return core.ReceiptHistoryPage{SchemaVersion: core.ReceiptSchemaVersion, Kind: request.Kind, PageSize: request.PageSize}, nil
}

func (fixedReceiptWorkflow) Summary(_ context.Context, request core.ReceiptSummaryRequest) (core.ReceiptSummary, error) {
	return core.ReceiptSummary{SchemaVersion: core.ReceiptSchemaVersion, Kind: request.Kind, From: request.From, To: request.To}, nil
}

func (fixedReceiptWorkflow) Overview(_ context.Context, request core.ReceiptOverviewRequest) (core.ReceiptOverview, error) {
	return core.ReceiptOverview{
		SchemaVersion: core.ReceiptSchemaVersion, Visibility: "private_local", From: request.From, To: request.To,
		Totals: []core.ReceiptOverviewKindTotal{{Kind: core.ReceiptKindCard, TotalCount: 6, TotalAmountKRW: 70000}},
	}, nil
}

func (fixedReceiptWorkflow) Download(_ context.Context, request core.ReceiptDownloadRequest) (receiptworkflow.Download, error) {
	return receiptworkflow.Download{
		Metadata: core.ReceiptDownloadMetadata{SchemaVersion: core.ReceiptSchemaVersion, Kind: request.Kind, Filename: "receipt.pdf", ContentType: "application/pdf"},
		Content:  []byte("synthetic receipt"),
	}, nil
}

func (fixedReceiptWorkflow) Vendor(_ context.Context, request core.VendorReceiptRequest) (core.VendorReceiptSnapshot, error) {
	return core.VendorReceiptSnapshot{SchemaVersion: core.ReceiptSchemaVersion, Visibility: "private_local", SourceRef: request.SourceRef, VendorCount: 1}, nil
}

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

func (fixedProductWorkflow) PriceHistory(_ context.Context, request core.ProductPriceHistoryRequest) (core.ProductPriceHistory, error) {
	return core.ProductPriceHistory{SchemaVersion: 1, Visibility: "private_local", ProductID: request.ProductID, VendorItemID: request.VendorItemID, ObservationCount: 2}, nil
}

func (fixedProductWorkflow) PurgePriceHistory(context.Context) (core.ProductPriceHistoryPurgeResult, error) {
	return core.ProductPriceHistoryPurgeResult{ObservationsDeleted: 2}, nil
}

func (fixedProductWorkflow) AddPriceWatch(_ context.Context, request core.ProductWatchRequest) (core.ProductWatchMutationResult, error) {
	return core.ProductWatchMutationResult{SchemaVersion: 1, Visibility: "private_local", Changed: true, Entry: core.ProductWatchEntry{Reference: core.ProductReference{ProductID: request.ProductID, VendorItemID: request.VendorItemID}}}, nil
}

func (fixedProductWorkflow) RemovePriceWatch(_ context.Context, request core.ProductWatchRequest) (core.ProductWatchMutationResult, error) {
	return core.ProductWatchMutationResult{SchemaVersion: 1, Visibility: "private_local", Changed: true, Entry: core.ProductWatchEntry{Reference: core.ProductReference{ProductID: request.ProductID, VendorItemID: request.VendorItemID}}}, nil
}

func (fixedProductWorkflow) PriceWatchlist(context.Context) (core.ProductWatchList, error) {
	return core.ProductWatchList{SchemaVersion: 1, Visibility: "private_local", Count: 1, Items: []core.ProductWatchEntry{{Identity: "vendor:201"}}}, nil
}

func (fixedProductWorkflow) RefreshPriceWatches(context.Context, core.ProductWatchRefreshRequest) (core.ProductWatchRefreshResult, error) {
	return core.ProductWatchRefreshResult{SchemaVersion: 1, Visibility: "private_local", Attempted: 1, Observed: 1}, nil
}

func (fixedProductWorkflow) ClearPriceWatches(context.Context) (core.ProductWatchClearResult, error) {
	return core.ProductWatchClearResult{WatchesDeleted: 2}, nil
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

func (*capturingProductWorkflow) PriceHistory(_ context.Context, request core.ProductPriceHistoryRequest) (core.ProductPriceHistory, error) {
	return core.ProductPriceHistory{SchemaVersion: 1, ProductID: request.ProductID}, nil
}

func (*capturingProductWorkflow) PurgePriceHistory(context.Context) (core.ProductPriceHistoryPurgeResult, error) {
	return core.ProductPriceHistoryPurgeResult{}, nil
}

func (*capturingProductWorkflow) AddPriceWatch(context.Context, core.ProductWatchRequest) (core.ProductWatchMutationResult, error) {
	return core.ProductWatchMutationResult{}, nil
}

func (*capturingProductWorkflow) RemovePriceWatch(context.Context, core.ProductWatchRequest) (core.ProductWatchMutationResult, error) {
	return core.ProductWatchMutationResult{}, nil
}

func (*capturingProductWorkflow) PriceWatchlist(context.Context) (core.ProductWatchList, error) {
	return core.ProductWatchList{}, nil
}

func (*capturingProductWorkflow) RefreshPriceWatches(context.Context, core.ProductWatchRefreshRequest) (core.ProductWatchRefreshResult, error) {
	return core.ProductWatchRefreshResult{}, nil
}

func (*capturingProductWorkflow) ClearPriceWatches(context.Context) (core.ProductWatchClearResult, error) {
	return core.ProductWatchClearResult{}, nil
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

func TestReceiptCommandsUseTypedReadsAndPrivateNonOverwritingDownload(t *testing.T) {
	var output bytes.Buffer
	if err := runReceipts(context.Background(), []string{"list", "--kind", "card", "--size", "7"}, &output, fixedReceiptWorkflow{}); err != nil {
		t.Fatal(err)
	}
	var history core.ReceiptHistoryPage
	if err := json.Unmarshal(output.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if history.Kind != core.ReceiptKindCard || history.PageSize != 7 {
		t.Fatalf("unexpected receipt history: %#v", history)
	}

	output.Reset()
	if err := runReceipts(context.Background(), []string{"overview", "--from", "2024-12-01", "--to", "2026-01-15"}, &output, fixedReceiptWorkflow{}); err != nil {
		t.Fatal(err)
	}
	var overview core.ReceiptOverview
	if err := json.Unmarshal(output.Bytes(), &overview); err != nil || overview.From != "2024-12-01" || overview.To != "2026-01-15" || overview.Totals[0].TotalAmountKRW != 70000 {
		t.Fatalf("unexpected receipt overview: %#v %v", overview, err)
	}

	path := filepath.Join(t.TempDir(), "receipt.pdf")
	output.Reset()
	if err := runReceipts(context.Background(), []string{"download", "--kind", "cash", "--history-index", "0", "--output", path}, &output, fixedReceiptWorkflow{}); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil || string(content) != "synthetic receipt" {
		t.Fatalf("unexpected receipt file: %q %v", content, err)
	}
	if err := runReceipts(context.Background(), []string{"download", "--kind", "cash", "--history-index", "0", "--output", path}, io.Discard, fixedReceiptWorkflow{}); err == nil {
		t.Fatal("receipt download overwrote an existing file")
	}

	sourceRef := core.OrderSourceReference("synthetic-order")
	output.Reset()
	if err := runReceipts(context.Background(), []string{"vendor", "--source-ref", sourceRef, "--max-pages", "7"}, &output, fixedReceiptWorkflow{}); err != nil {
		t.Fatal(err)
	}
	var vendor core.VendorReceiptSnapshot
	if err := json.Unmarshal(output.Bytes(), &vendor); err != nil || vendor.SourceRef != sourceRef || vendor.VendorCount != 1 {
		t.Fatalf("unexpected vendor receipt: %#v %v", vendor, err)
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

func TestTopLevelHelpIsStructuredAndHasNoEnvironmentDependency(t *testing.T) {
	t.Setenv("COUPANGCTL_STATE_DIR", "relative-would-fail")
	for _, args := range [][]string{nil, {"help"}, {"--help"}, {"-h"}} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if err := Run(context.Background(), args, &stdout, &stderr, "v0.1.0-test"); err != nil {
				t.Fatal(err)
			}
			var got struct {
				SchemaVersion int `json:"schema_version"`
				Name          string
				Usage         string
				Commands      []struct {
					Name    string
					Summary string
				} `json:"commands"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.SchemaVersion != 1 || got.Name != "coupangctl" || got.Usage == "" || len(got.Commands) < 10 {
				t.Fatalf("unexpected help response: %#v", got)
			}
			if got.Commands[0].Name != "auth" || got.Commands[0].Summary == "" {
				t.Fatalf("commands are not documented and ordered: %#v", got.Commands)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q", stderr.String())
			}
		})
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
	if report.SchemaVersion != 3 || report.Summary.Total != len(report.Capabilities) || report.Summary.ImplementationNextSteps != 0 || report.Summary.ValidationOrCoordinationNextSteps == 0 || len(report.Capabilities) < 5 || report.Capabilities[0].Priority != "P0" {
		t.Fatalf("unexpected capability report: %#v", report)
	}
	if report.Capabilities[0].Status != core.CapabilityAvailable || len(report.Capabilities[0].Implemented) == 0 || report.Capabilities[0].NextStepKind == "" {
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

func TestProductsPriceHistoryWritesTypedPrivateLocalResponse(t *testing.T) {
	var output bytes.Buffer
	if err := runProducts(context.Background(), []string{"price-history", "--product-id", "101", "--vendor-item-id", "201"}, &output, fixedProductWorkflow{}); err != nil {
		t.Fatal(err)
	}
	var got core.ProductPriceHistory
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Visibility != "private_local" || got.ProductID != "101" || got.VendorItemID != "201" || got.ObservationCount != 2 {
		t.Fatalf("unexpected price history output: %#v", got)
	}
}

func TestProductsPriceHistoryPurgeRequiresExactConfirmation(t *testing.T) {
	if err := runProducts(context.Background(), []string{"price-history-purge"}, io.Discard, fixedProductWorkflow{}); err == nil {
		t.Fatal("price history purge ran without confirmation")
	}
	var output bytes.Buffer
	if err := runProducts(context.Background(), []string{"price-history-purge", "--confirm", "purge-product-price-history"}, &output, fixedProductWorkflow{}); err != nil {
		t.Fatal(err)
	}
	var got core.ProductPriceHistoryPurgeResult
	if err := json.Unmarshal(output.Bytes(), &got); err != nil || got.ObservationsDeleted != 2 {
		t.Fatalf("unexpected purge result: %#v err=%v", got, err)
	}
}

func TestProductsWatchCommandsExposeTypedLocalWorkflow(t *testing.T) {
	var output bytes.Buffer
	if err := runProducts(context.Background(), []string{"watch-add", "--product-id", "101", "--vendor-item-id", "201"}, &output, fixedProductWorkflow{}); err != nil {
		t.Fatal(err)
	}
	var mutation core.ProductWatchMutationResult
	if err := json.Unmarshal(output.Bytes(), &mutation); err != nil || !mutation.Changed || mutation.Entry.Reference.VendorItemID != "201" {
		t.Fatalf("unexpected watch mutation: %#v err=%v", mutation, err)
	}
	output.Reset()
	if err := runProducts(context.Background(), []string{"watch-refresh", "--limit", "3", "--stale-hours", "12"}, &output, fixedProductWorkflow{}); err != nil {
		t.Fatal(err)
	}
	var refresh core.ProductWatchRefreshResult
	if err := json.Unmarshal(output.Bytes(), &refresh); err != nil || refresh.Attempted != 1 || refresh.Observed != 1 {
		t.Fatalf("unexpected watch refresh: %#v err=%v", refresh, err)
	}
}

func TestProductsWatchClearRequiresExactConfirmation(t *testing.T) {
	if err := runProducts(context.Background(), []string{"watch-clear"}, io.Discard, fixedProductWorkflow{}); err == nil {
		t.Fatal("watchlist clear ran without confirmation")
	}
	var output bytes.Buffer
	if err := runProducts(context.Background(), []string{"watch-clear", "--confirm", "clear-product-watchlist"}, &output, fixedProductWorkflow{}); err != nil {
		t.Fatal(err)
	}
	var got core.ProductWatchClearResult
	if err := json.Unmarshal(output.Bytes(), &got); err != nil || got.WatchesDeleted != 2 {
		t.Fatalf("unexpected watch clear result: %#v err=%v", got, err)
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

func TestOrdersSyncStatusReturnsNeverRunAsTypedJSON(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	var output bytes.Buffer
	if err := runOrders(ctx, []string{"sync-status"}, &output, orderworkflow.New(ledger, nil)); err != nil {
		t.Fatal(err)
	}
	var got core.SyncStatus
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != core.SyncStatusSchemaVersion || got.State != core.SyncRunNeverRun || got.Source != "" || got.Visibility != "private_local" || len(got.Limitations) == 0 {
		t.Fatalf("unexpected never-run sync status: %#v", got)
	}
}

func TestOrdersSyncCanUseTheExplicitlySelectedOrdinaryBrowser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stateDir := t.TempDir()
	t.Setenv("COUPANGCTL_STATE_DIR", stateDir)
	var stdout, stderr bytes.Buffer
	cliResult := make(chan error, 1)
	go func() {
		cliResult <- Run(ctx, []string{"orders", "sync", "--max-pages", "1", "--ordinary-browser"}, &stdout, &stderr, "test")
	}()

	rendezvousPath := filepath.Join(stateDir, "ordinary-browser-rendezvous.json")
	for {
		if _, err := os.Stat(rendezvousPath); err == nil {
			break
		}
		select {
		case err := <-cliResult:
			t.Fatalf("CLI exited before browser rendezvous: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}

	extensionToHostReader, extensionToHostWriter := io.Pipe()
	hostToExtensionReader, hostToExtensionWriter := io.Pipe()
	hostResult := make(chan error, 1)
	go func() {
		hostResult <- browser.RunOrdinaryBrowserNativeHost(
			ctx,
			stateDir,
			"chrome-extension://"+browser.OrdinaryBrowserExtensionID+"/",
			browser.OrdinaryBrowserExtensionID,
			extensionToHostReader,
			hostToExtensionWriter,
		)
	}()
	requestPayload, err := readSyntheticNativeFrame(hostToExtensionReader)
	if err != nil {
		t.Fatal(err)
	}
	var request browser.OrdinaryBridgeRequest
	if err := json.Unmarshal(requestPayload, &request); err != nil {
		t.Fatal(err)
	}
	if request.Operation != browser.OrdinaryBridgeReadOrders || request.Cursor != nil {
		t.Fatalf("ordinary-browser request = %#v", request)
	}
	responsePayload, err := json.Marshal(browser.OrdinaryBridgeResponse{
		SchemaVersion: browser.OrdinaryBridgeSchemaVersion,
		RequestID:     request.RequestID,
		Status:        browser.OrdinaryBridgeOK,
		Page:          &core.OrderPage{Orders: []core.Order{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := writeSyntheticNativeFrame(extensionToHostWriter, responsePayload); err != nil {
		t.Fatal(err)
	}
	if err := <-cliResult; err != nil {
		t.Fatal(err)
	}
	if err := <-hostResult; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "Chrome") {
		t.Fatalf("ordinary-browser instruction missing: %q", stderr.String())
	}
	var result core.SyncResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.PagesProcessed != 1 || result.OrdersSeen != 0 || result.Source != core.SyncSourceOrdinaryBrowser || result.Provenance != core.SyncProvenanceObservedStructuredOrderDocument {
		t.Fatalf("ordinary-browser sync result = %#v", result)
	}
}

func TestCurrentBrowserReadRequestedOnlyForExplicitOrderSync(t *testing.T) {
	if !currentBrowserReadRequested([]string{"orders", "sync", "--current-browser"}) {
		t.Fatal("explicit current-browser order sync was not selected")
	}
	if !currentBrowserReadRequested([]string{"orders", "sync", "--current-browser=true"}) {
		t.Fatal("explicit true current-browser order sync was not selected")
	}
	for _, args := range [][]string{
		{"orders", "sync"},
		{"orders", "list", "--current-browser"},
		{"products", "search", "--current-browser"},
		{"orders", "sync", "--current-browser=false"},
	} {
		if currentBrowserReadRequested(args) {
			t.Fatalf("unexpected current-browser selection for %q", args)
		}
	}
}

func TestBackgroundBrowserPolicyIsLimitedToUnattendedEntryPoints(t *testing.T) {
	for _, test := range []struct {
		args []string
		want bool
	}{
		{args: []string{"mcp"}, want: true},
		{args: []string{"products", "watch-refresh"}, want: true},
		{args: []string{"products", "watch-refresh", "--headed"}, want: false},
		{args: []string{"products", "search", "synthetic"}, want: false},
		{args: []string{"orders", "sync"}, want: false},
	} {
		if got := backgroundReadRequested(test.args); got != test.want {
			t.Fatalf("backgroundReadRequested(%#v) = %t, want %t", test.args, got, test.want)
		}
	}
}

func TestConvenienceCommandsExpandWithoutMutatingInput(t *testing.T) {
	for _, test := range []struct {
		input []string
		want  []string
	}{
		{[]string{"sync", "--max-pages", "2"}, []string{"orders", "sync", "--max-pages", "2"}},
		{[]string{"recap", "--output", "recap.html"}, []string{"orders", "recap", "--output", "recap.html"}},
		{[]string{"login"}, []string{"auth", "ensure"}},
		{[]string{"orders", "sync"}, []string{"orders", "sync"}},
	} {
		original := append([]string(nil), test.input...)
		got := expandConvenienceCommand(test.input)
		if strings.Join(got, "\x00") != strings.Join(test.want, "\x00") {
			t.Fatalf("expandConvenienceCommand(%q) = %q, want %q", test.input, got, test.want)
		}
		if strings.Join(test.input, "\x00") != strings.Join(original, "\x00") {
			t.Fatalf("expandConvenienceCommand mutated input: got %q want %q", test.input, original)
		}
	}
}

func TestOrdersSyncRejectsConflictingBrowserModes(t *testing.T) {
	for _, args := range [][]string{
		{"sync", "--headed", "--current-browser"},
		{"sync", "--headed", "--ordinary-browser"},
		{"sync", "--current-browser", "--ordinary-browser"},
	} {
		err := runOrders(context.Background(), args, io.Discard, nil)
		if err == nil || !strings.Contains(err.Error(), "--current-browser") {
			t.Fatalf("runOrders(%q) error = %v", args, err)
		}
	}
}

func TestRunRejectsConflictingBrowserModesBeforeStartingBridge(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("COUPANGCTL_STATE_DIR", stateDir)
	err := Run(
		context.Background(),
		[]string{"orders", "sync", "--current-browser", "--ordinary-browser"},
		io.Discard,
		io.Discard,
		"test",
	)
	if err == nil || err.Error() != orderSyncUsage {
		t.Fatalf("conflicting browser modes error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "ordinary-browser-rendezvous.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("conflicting modes created a browser rendezvous: %v", err)
	}
}

func TestSyncAliasRejectsConflictingBrowserModesBeforeStartingBridge(t *testing.T) {
	stateDir := t.TempDir()
	t.Setenv("COUPANGCTL_STATE_DIR", stateDir)
	err := Run(
		context.Background(),
		[]string{"sync", "--current-browser", "--ordinary-browser"},
		io.Discard,
		io.Discard,
		"test",
	)
	if err == nil || err.Error() != orderSyncUsage {
		t.Fatalf("sync alias conflicting browser modes error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(stateDir, "ordinary-browser-rendezvous.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("sync alias created a browser rendezvous: %v", err)
	}
}

func TestChromeNativeHostInvocationRejectsEveryOtherExtensionOrigin(t *testing.T) {
	t.Setenv("COUPANGCTL_STATE_DIR", t.TempDir())
	var stdout, stderr bytes.Buffer
	err := Run(
		context.Background(),
		[]string{"chrome-extension://" + strings.Repeat("a", 32) + "/"},
		&stdout,
		&stderr,
		"test",
	)
	if !errors.Is(err, browser.ErrOrdinaryNativeOrigin) {
		t.Fatalf("native host error = %v, want ErrOrdinaryNativeOrigin", err)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("native host wrote non-frame output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func readSyntheticNativeFrame(reader io.Reader) ([]byte, error) {
	header := make([]byte, 4)
	if _, err := io.ReadFull(reader, header); err != nil {
		return nil, err
	}
	payload := make([]byte, binary.NativeEndian.Uint32(header))
	_, err := io.ReadFull(reader, payload)
	return payload, err
}

func writeSyntheticNativeFrame(writer io.Writer, payload []byte) error {
	header := make([]byte, 4)
	binary.NativeEndian.PutUint32(header, uint32(len(payload)))
	if _, err := writer.Write(header); err != nil {
		return err
	}
	_, err := writer.Write(payload)
	return err
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

func TestOrdersCategoryCatalogReturnsSourceNativeSearchID(t *testing.T) {
	ctx := context.Background()
	stateDir := t.TempDir()
	t.Setenv("COUPANGCTL_STATE_DIR", stateDir)
	ledger, err := store.Open(ctx, filepath.Join(stateDir, "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{{
		SourceRef: "synthetic-category-catalog", PurchasedAt: "2026-08-29", TotalAmount: 1200, Currency: "KRW",
		Items: []core.OrderItem{{
			ProductID: "101", VendorItemID: "201", Name: "Synthetic private product", Quantity: 1, UnitPrice: 1200, PaidPrice: 1200,
		}},
	}}}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.SaveProductCategory(ctx, core.ProductReference{VendorItemID: "201"}, core.ProductCategory{
		Source: core.CategorySourceProductJSONLDBreadcrumb,
		Path:   []core.ProductCategoryNode{{ID: "200", Name: "Synthetic category", Position: 2}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Run(ctx, []string{"orders", "category-catalog", "--query", "Synthetic"}, &stdout, &stderr, "test"); err != nil {
		t.Fatal(err)
	}
	var got core.CategoryCatalog
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Visibility != "private_local" || got.ReturnedCategoryCount != 1 || got.Categories[0].CategoryID != "200" || got.Categories[0].MatchKind != "prefix_label" {
		t.Fatalf("unexpected category catalog response: %#v", got)
	}
}

func TestOrdersCategoryStabilityReturnsLongitudinalEvidence(t *testing.T) {
	ctx := context.Background()
	stateDir := t.TempDir()
	t.Setenv("COUPANGCTL_STATE_DIR", stateDir)
	ledger, err := store.Open(ctx, filepath.Join(stateDir, "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{{
		SourceRef: "synthetic-category-stability", PurchasedAt: "2026-08-29", TotalAmount: 1200, Currency: "KRW",
		Items: []core.OrderItem{{ProductID: "101", VendorItemID: "201", Name: "Synthetic private product", Quantity: 1, UnitPrice: 1200, PaidPrice: 1200}},
	}}}); err != nil {
		t.Fatal(err)
	}
	category := core.ProductCategory{Source: core.CategorySourceProductJSONLDBreadcrumb, Path: []core.ProductCategoryNode{{ID: "200", Name: "Synthetic category", Position: 2}}}
	if err := ledger.SaveProductCategory(ctx, core.ProductReference{VendorItemID: "201"}, category); err != nil {
		t.Fatal(err)
	}
	if err := ledger.SaveProductCategory(ctx, core.ProductReference{VendorItemID: "201"}, category); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if err := Run(ctx, []string{"orders", "category-stability"}, &stdout, &stderr, "test"); err != nil {
		t.Fatal(err)
	}
	var got core.CategoryStabilityReport
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Assessment != "insufficient_distinct_days" || got.RecheckedProductCount != 1 || got.StableProductCount != 1 || got.ObservationCount != 2 || got.Provenance.Assessment != "derived" {
		t.Fatalf("unexpected category stability response: %#v", got)
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

func TestOrdersRecapImagePreviewsExactPublicFieldsBeforeWriting(t *testing.T) {
	ctx := context.Background()
	stateDir := t.TempDir()
	t.Setenv("COUPANGCTL_STATE_DIR", stateDir)
	ledger, err := store.Open(ctx, filepath.Join(stateDir, "coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{{
		SourceRef: "synthetic-image-preview", PurchasedAt: "2026-08-29", TotalAmount: 1200, Currency: "KRW",
		Items: []core.OrderItem{{ProductID: "101", VendorItemID: "201", Name: "Synthetic private image product", Quantity: 1, UnitPrice: 1200, PaidPrice: 1200}},
	}}}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		t.Fatal(err)
	}

	outputPath := filepath.Join(t.TempDir(), "shopping-recap.png")
	var stdout, stderr bytes.Buffer
	if err := Run(ctx, []string{"orders", "recap-image", "--output", outputPath}, &stdout, &stderr, "test"); err != nil {
		t.Fatal(err)
	}
	var preview core.RecapSharePreview
	if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Visibility != "public_safe" || preview.Format != "png" || preview.ConfirmationFlag != "--confirm-public-safe-image" || len(preview.Fields) == 0 {
		t.Fatalf("unexpected recap image preview: %#v", preview)
	}
	if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
		t.Fatalf("recap image was written without confirmation: %v", err)
	}
	if bytes.Contains(stdout.Bytes(), []byte("Synthetic private image product")) || bytes.Contains(stdout.Bytes(), []byte("2026-08-29")) {
		t.Fatalf("public image preview exposed a private field: %s", stdout.Bytes())
	}
	stdout.Reset()
	if err := Run(ctx, []string{"orders", "recap-image", "--confirm-public-safe-image"}, &stdout, &stderr, "test"); err == nil {
		t.Fatal("confirmed recap image without an output path was accepted")
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

func TestWriteErrorClassifiesUnavailableCurrentBrowserWithoutLocalDetails(t *testing.T) {
	var output bytes.Buffer
	WriteError(&output, errors.Join(browser.ErrCurrentBrowserUnavailable, errors.New("sensitive profile path")))
	var got core.ErrorResponse
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Error.Code != "current_browser_unavailable" || !strings.Contains(got.Error.Message, "chrome://inspect/#remote-debugging") {
		t.Fatalf("unexpected current-browser error: %#v", got)
	}
	if bytes.Contains(output.Bytes(), []byte("sensitive profile path")) {
		t.Fatalf("error output exposed an internal cause: %s", output.Bytes())
	}
}

func TestWriteErrorClassifiesProfileInUseWithoutLocalDetails(t *testing.T) {
	var output bytes.Buffer
	WriteError(&output, errors.Join(browser.ErrProfileInUse, errors.New("sensitive lock path")))
	var got core.ErrorResponse
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Error.Code != "profile_in_use" || strings.Contains(got.Error.Message, "sensitive") {
		t.Fatalf("unexpected profile lock error: %#v", got)
	}
}

func TestWriteErrorClassifiesIncompatibleProfileWithoutLocalDetails(t *testing.T) {
	var output bytes.Buffer
	WriteError(&output, errors.Join(browser.ErrProfileIncompatible, errors.New("sensitive executable path")))
	var got core.ErrorResponse
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Error.Code != "browser_profile_incompatible" || strings.Contains(got.Error.Message, "sensitive") {
		t.Fatalf("unexpected profile compatibility error: %#v", got)
	}
}

func TestWriteErrorClassifiesBrowserBridgeOwnershipConflictWithoutPath(t *testing.T) {
	var output bytes.Buffer
	WriteError(&output, errors.Join(browserbridge.ErrInstallationConflict, errors.New("sensitive local path")))
	var got struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Error.Code != "browser_bridge_installation_conflict" || strings.Contains(got.Error.Message, "sensitive") {
		t.Fatalf("unexpected browser bridge error: %#v", got.Error)
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

func TestWriteErrorClassifiesMissingVendorReceiptWithoutReference(t *testing.T) {
	var output bytes.Buffer
	WriteError(&output, errors.Join(receiptworkflow.ErrSourceUnavailable, core.ErrVendorReceiptNotFound, errors.New("sensitive order reference")))
	var envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Code != "vendor_receipt_not_found" || strings.Contains(envelope.Error.Message, "sensitive") {
		t.Fatalf("unexpected error envelope: %#v", envelope)
	}
}

func TestWriteErrorDoesNotClaimAnAccessDeniedReadWasHeadless(t *testing.T) {
	var output bytes.Buffer
	WriteError(&output, browser.ErrBrowserAccessDenied)
	var got core.ErrorResponse
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Error.Code != "browser_access_denied" || strings.HasPrefix(got.Error.Message, "headless access") {
		t.Fatalf("misleading browser access error: %#v", got)
	}
}

func TestWriteCommandErrorDoesNotRecommendHeadedModeAfterHeadedDenial(t *testing.T) {
	var output bytes.Buffer
	WriteCommandError(&output, []string{"auth", "verify", "--headed"}, browser.ErrBrowserAccessDenied)
	var got core.ErrorResponse
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Error.Code != "headed_browser_access_denied" || strings.Contains(got.Error.Message, "choose a supported headed") || !strings.Contains(got.Error.Message, "retry later") {
		t.Fatalf("circular headed access error: %#v", got)
	}

	output.Reset()
	WriteCommandError(&output, []string{"auth", "verify"}, browser.ErrBrowserAccessDenied)
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Error.Code != "browser_access_denied" {
		t.Fatalf("quiet denial lost its generic remediation: %#v", got)
	}

	output.Reset()
	WriteCommandError(&output, []string{"products", "search", "--query", "synthetic"}, browser.ErrBrowserAccessDenied)
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Error.Code != "browser_access_denied" || !strings.Contains(got.Error.Message, "--headed") || strings.Contains(got.Error.Message, "current-browser") {
		t.Fatalf("product denial recommended an unsupported recovery mode: %#v", got)
	}

	output.Reset()
	WriteCommandError(&output, []string{"sync", "--max-pages", "1"}, browser.ErrBrowserAccessDenied)
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Error.Code != "browser_access_denied" || !strings.Contains(got.Error.Message, "--headed") || !strings.Contains(got.Error.Message, "--current-browser") {
		t.Fatalf("order sync denial omitted a supported recovery mode: %#v", got)
	}

	output.Reset()
	WriteCommandError(&output, []string{"sync", "--current-browser"}, browser.ErrBrowserAccessDenied)
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Error.Code != "current_browser_access_denied" || strings.Contains(got.Error.Message, "use --current-browser") || !strings.Contains(got.Error.Message, "retry later") {
		t.Fatalf("current-browser denial recommended the already-failed mode: %#v", got)
	}
}

func TestWriteErrorClassifiesLocalRecapImageRenderFailure(t *testing.T) {
	var output bytes.Buffer
	WriteError(&output, browser.ErrLocalPageRenderFailed)
	var got core.ErrorResponse
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Error.Code != "recap_image_render_failed" {
		t.Fatalf("unexpected recap image render error: %#v", got)
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

func TestAuthEnsureReusesReadySessionWithoutOpeningLogin(t *testing.T) {
	browser := &loginModeBrowser{profilePresent: true}
	service := auth.NewService(browser)
	var output bytes.Buffer
	if err := runAuth(context.Background(), []string{"ensure"}, &output, io.Discard, service, noopResendAssistant{}, &fixedLoginSecrets{}); err != nil {
		t.Fatal(err)
	}
	var got core.AuthRecoveryResult
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.State != core.AuthVerified || got.VisibleBrowserOpened || browser.loginCalls != 0 || browser.verifyCalls != 1 {
		t.Fatalf("ready session was not reused: result=%#v browser=%#v", got, browser)
	}
}

func TestAuthEnsureOpensQRLoginOnlyForExpiredSession(t *testing.T) {
	browser := &loginModeBrowser{profilePresent: true, verifyErr: core.ErrAuthenticationRequired}
	service := auth.NewService(browser)
	var output bytes.Buffer
	if err := runAuth(context.Background(), []string{"ensure"}, &output, io.Discard, service, noopResendAssistant{}, &fixedLoginSecrets{}); err != nil {
		t.Fatal(err)
	}
	var got core.AuthRecoveryResult
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.BeforeState != core.AuthUnverified || got.State != core.AuthVerified || !got.VisibleBrowserOpened || got.Mode != core.LoginModeQR || browser.loginCalls != 1 {
		t.Fatalf("expired session was not recovered once: result=%#v browser=%#v", got, browser)
	}
}

func TestAuthEnsureDoesNotOpenLoginForTemporaryAccessBlock(t *testing.T) {
	browser := &loginModeBrowser{profilePresent: true, verifyErr: core.ErrBrowserAccessDenied}
	service := auth.NewService(browser)
	var output bytes.Buffer
	if err := runAuth(context.Background(), []string{"ensure"}, &output, io.Discard, service, noopResendAssistant{}, &fixedLoginSecrets{}); err != nil {
		t.Fatal(err)
	}
	var got core.AuthRecoveryResult
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.State != core.AuthAccessBlocked || got.VisibleBrowserOpened || browser.loginCalls != 0 {
		t.Fatalf("temporary access block opened unnecessary login: result=%#v browser=%#v", got, browser)
	}
}

func TestAuthStatusWritesBackgroundAccessBlockAsTypedJSON(t *testing.T) {
	service := auth.NewService(blockedStatusBrowser{})
	var output bytes.Buffer
	if err := runAuth(context.Background(), []string{"status"}, &output, io.Discard, service, noopResendAssistant{}, &fixedLoginSecrets{}); err != nil {
		t.Fatal(err)
	}
	var got core.AuthStatus
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.State != core.AuthAccessBlocked || !got.ProfilePresent || got.Browser != "Synthetic Chrome" {
		t.Fatalf("unexpected auth status: %#v", got)
	}
}

func TestDoctorSeparatesBrowserInstallationFromBackgroundSessionReadiness(t *testing.T) {
	service := auth.NewService(blockedStatusBrowser{})
	paths := platform.Paths{Database: filepath.Join(t.TempDir(), "coupangctl.sqlite3")}
	var output bytes.Buffer
	if err := runDoctor(context.Background(), &output, paths, service); err != nil {
		t.Fatal(err)
	}
	var got core.DoctorReport
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.OK {
		t.Fatal("doctor reported background access as ready")
	}
	want := map[string]core.CheckStatus{
		"browser":            core.CheckOK,
		"background_session": core.CheckError,
		"sqlite":             core.CheckOK,
	}
	for _, check := range got.Checks {
		if status, ok := want[check.Name]; ok {
			if check.Status != status {
				t.Fatalf("check %q status = %q, want %q", check.Name, check.Status, status)
			}
			delete(want, check.Name)
		}
	}
	if len(want) != 0 {
		t.Fatalf("doctor omitted checks: %#v", want)
	}
}

func TestDoctorReportsVerifiedBackgroundSessionAsReady(t *testing.T) {
	provider := fixedDoctorAuthStatus{status: core.AuthStatus{State: core.AuthVerified, Browser: "Synthetic Chrome", ProfilePresent: true}}
	paths := platform.Paths{Database: filepath.Join(t.TempDir(), "coupangctl.sqlite3")}
	var output bytes.Buffer
	if err := runDoctor(context.Background(), &output, paths, provider); err != nil {
		t.Fatal(err)
	}
	var got core.DoctorReport
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.OK {
		t.Fatalf("doctor did not report verified background session ready: %#v", got)
	}
	for _, check := range got.Checks {
		if check.Status != core.CheckOK {
			t.Fatalf("unexpected failed check: %#v", check)
		}
	}
}

func TestCurrentBrowserStatusWritesPassiveTypedReadiness(t *testing.T) {
	want := core.CurrentBrowserStatus{
		SchemaVersion:              core.CurrentBrowserStatusSchemaVersion,
		State:                      core.CurrentBrowserEndpointAvailable,
		Browser:                    "Synthetic Chrome",
		EndpointAvailable:          true,
		ConnectionApprovalVerified: false,
		CheckedAt:                  time.Date(2026, time.September, 3, 9, 0, 0, 0, time.UTC),
		NextAction:                 "run `coupangctl sync --current-browser` and approve Chrome's connection prompt",
	}
	var output bytes.Buffer
	if err := runCurrentBrowser(context.Background(), []string{"status"}, &output, fixedCurrentBrowserStatusProvider{status: want}); err != nil {
		t.Fatal(err)
	}
	var got core.CurrentBrowserStatus
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("status = %#v, want %#v", got, want)
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
