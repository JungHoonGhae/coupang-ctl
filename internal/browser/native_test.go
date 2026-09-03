package browser

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestInspectUsesExplicitExecutableAndDedicatedProfile(t *testing.T) {
	executable := syntheticExecutable(t, "exit 0\n")
	profile := filepath.Join(t.TempDir(), "browser-profile")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	native := NewNative(profile)
	native.getenv = func(name string) string {
		if name == "COUPANGCTL_BROWSER_PATH" {
			return executable
		}
		return ""
	}

	got, err := native.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !got.ProfilePresent || got.Name != filepath.Base(executable) {
		t.Fatalf("unexpected browser status: %#v", got)
	}
}

type fakeDocumentSession struct {
	document         []byte
	err              error
	urls             []string
	categoryDocument []byte
	categoryErr      error
	categoryURLs     []string
	productDocument  []byte
	productErr       error
	productURLs      []string
	receiptDocument  []byte
	receiptErr       error
	receiptErrors    []error
	receiptReads     int
	receiptHistory   []core.ReceiptHistoryRequest
	receiptSummaries []core.ReceiptSummaryRequest
	receiptDownloads []core.ReceiptDownloadRequest
	vendorOrderIDs   []string
	vendorSourceRefs []string
	vendorPages      []int
	researchDocument []byte
	researchErr      error
	researchSamples  []ReceiptResearchOrderSample
	closed           bool
}

func (f *fakeDocumentSession) ReadOrderDocument(_ context.Context, targetURL string) ([]byte, error) {
	f.urls = append(f.urls, targetURL)
	return f.document, f.err
}

func (f *fakeDocumentSession) ReadProductCategoryDocument(_ context.Context, targetURL string) ([]byte, error) {
	f.categoryURLs = append(f.categoryURLs, targetURL)
	return f.categoryDocument, f.categoryErr
}

func (f *fakeDocumentSession) ReadProductSearchDocument(_ context.Context, targetURL string) ([]byte, error) {
	f.productURLs = append(f.productURLs, targetURL)
	return f.productDocument, f.productErr
}

func (f *fakeDocumentSession) ReadReceiptStatusDocument(context.Context) ([]byte, error) {
	return f.nextReceiptDocument()
}

func (f *fakeDocumentSession) ReadReceiptHistoryDocument(_ context.Context, request core.ReceiptHistoryRequest) ([]byte, error) {
	f.receiptHistory = append(f.receiptHistory, request)
	return f.nextReceiptDocument()
}

func (f *fakeDocumentSession) ReadReceiptSummaryDocument(_ context.Context, request core.ReceiptSummaryRequest) ([]byte, error) {
	f.receiptSummaries = append(f.receiptSummaries, request)
	return f.nextReceiptDocument()
}

func (f *fakeDocumentSession) ReadReceiptDownloadDocument(_ context.Context, request core.ReceiptDownloadRequest) ([]byte, error) {
	f.receiptDownloads = append(f.receiptDownloads, request)
	return f.nextReceiptDocument()
}

func (f *fakeDocumentSession) ReadVendorReceiptDocument(_ context.Context, orderID, sourceRef string, pagesScanned int) ([]byte, error) {
	f.vendorOrderIDs = append(f.vendorOrderIDs, orderID)
	f.vendorSourceRefs = append(f.vendorSourceRefs, sourceRef)
	f.vendorPages = append(f.vendorPages, pagesScanned)
	return f.nextReceiptDocument()
}

func (f *fakeDocumentSession) ReadReceiptResearchMetadata(_ context.Context, samples []ReceiptResearchOrderSample) ([]byte, error) {
	f.researchSamples = append([]ReceiptResearchOrderSample(nil), samples...)
	return f.researchDocument, f.researchErr
}

func (f *fakeDocumentSession) nextReceiptDocument() ([]byte, error) {
	index := f.receiptReads
	f.receiptReads++
	if index < len(f.receiptErrors) {
		return f.receiptDocument, f.receiptErrors[index]
	}
	return f.receiptDocument, f.receiptErr
}

func (f *fakeDocumentSession) Close() error {
	f.closed = true
	return nil
}

func TestFetchAutomaticallyFallsBackToHeadedAndReusesIt(t *testing.T) {
	executable := syntheticExecutable(t, "exit 0\n")
	profile := filepath.Join(t.TempDir(), "browser-profile")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	headless := &fakeDocumentSession{err: ErrBrowserAccessDenied}
	headed := &fakeDocumentSession{document: []byte(`{"props":{}}`)}
	native := NewNative(profile)
	native.getenv = func(name string) string {
		if name == "COUPANGCTL_BROWSER_PATH" {
			return executable
		}
		return ""
	}
	native.allowHeadedFallback = func() bool { return true }
	primaryCalls := 0
	fallbackCalls := 0
	native.sessionFactory = func(context.Context, string, string) (documentSession, error) {
		primaryCalls++
		return headless, nil
	}
	native.headedSessionFactory = func(context.Context, string, string) (documentSession, error) {
		fallbackCalls++
		return headed, nil
	}

	for attempt := 0; attempt < 2; attempt++ {
		if _, err := native.Fetch(context.Background(), nil); err != nil {
			t.Fatal(err)
		}
	}
	if primaryCalls != 1 || fallbackCalls != 1 {
		t.Fatalf("session factories called headless=%d headed=%d", primaryCalls, fallbackCalls)
	}
	if !headless.closed {
		t.Fatal("denied headless session was not closed")
	}
	if len(headed.urls) != 2 {
		t.Fatalf("headed session reads = %d, want 2", len(headed.urls))
	}
}

func TestProductCategoryFetchTreatsHeadlessLoginRedirectAsAccessRejection(t *testing.T) {
	executable := syntheticExecutable(t, "exit 0\n")
	profile := filepath.Join(t.TempDir(), "browser-profile")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	headless := &fakeDocumentSession{categoryErr: ErrAuthenticationRequired}
	headed := &fakeDocumentSession{categoryDocument: []byte(`{"json_ld":[]}`)}
	native := NewNative(profile)
	native.getenv = func(name string) string {
		if name == "COUPANGCTL_BROWSER_PATH" {
			return executable
		}
		return ""
	}
	native.allowHeadedFallback = func() bool { return true }
	native.sessionFactory = func(context.Context, string, string) (documentSession, error) {
		return headless, nil
	}
	native.headedSessionFactory = func(context.Context, string, string) (documentSession, error) {
		return headed, nil
	}

	reference := core.ProductReference{ProductID: "101", VendorItemID: "201"}
	if _, err := native.FetchProductCategory(context.Background(), reference); err != nil {
		t.Fatal(err)
	}
	if !headless.closed || len(headed.categoryURLs) != 1 {
		t.Fatalf("fallback state: headless closed=%t headed reads=%d", headless.closed, len(headed.categoryURLs))
	}
}

func TestProductCategoryFetchReturnsTypedUnavailableAfterHeadedRejection(t *testing.T) {
	executable := syntheticExecutable(t, "exit 0\n")
	profile := filepath.Join(t.TempDir(), "browser-profile")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	headless := &fakeDocumentSession{categoryErr: ErrBrowserAccessDenied}
	headed := &fakeDocumentSession{categoryErr: ErrAuthenticationRequired}
	native := NewNative(profile)
	native.getenv = func(name string) string {
		if name == "COUPANGCTL_BROWSER_PATH" {
			return executable
		}
		return ""
	}
	native.allowHeadedFallback = func() bool { return true }
	native.sessionFactory = func(context.Context, string, string) (documentSession, error) { return headless, nil }
	native.headedSessionFactory = func(context.Context, string, string) (documentSession, error) { return headed, nil }

	_, err := native.FetchProductCategory(context.Background(), core.ProductReference{ProductID: "101", VendorItemID: "201"})
	if !errors.Is(err, core.ErrProductCategoryUnavailable) {
		t.Fatalf("category fetch error = %v, want typed unavailable", err)
	}
}

func TestProductSearchRetriesMissingHeadlessDocumentOnceInHeadedBrowser(t *testing.T) {
	executable := syntheticExecutable(t, "exit 0\n")
	profile := filepath.Join(t.TempDir(), "browser-profile")
	headless := &fakeDocumentSession{productErr: ErrStructuredProductDataMissing}
	headed := &fakeDocumentSession{productDocument: []byte(`{"items":[{"product_id":"101"}]}`)}
	native := NewNative(profile)
	native.getenv = func(name string) string {
		if name == "COUPANGCTL_BROWSER_PATH" {
			return executable
		}
		return ""
	}
	native.allowHeadedFallback = func() bool { return true }
	native.sessionFactory = func(context.Context, string, string) (documentSession, error) { return headless, nil }
	native.headedSessionFactory = func(context.Context, string, string) (documentSession, error) { return headed, nil }

	document, err := native.FetchProductSearch(context.Background(), core.ProductSearchRequest{Query: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	if string(document) != string(headed.productDocument) || !headless.closed || len(headless.productURLs) != 1 || len(headed.productURLs) != 1 {
		t.Fatalf("unexpected product fallback: document=%s closed=%t headless=%#v headed=%#v", document, headless.closed, headless.productURLs, headed.productURLs)
	}
}

func TestExplicitHeadedCategoryFetchNormalizesMissingDocument(t *testing.T) {
	executable := syntheticExecutable(t, "exit 0\n")
	profile := filepath.Join(t.TempDir(), "browser-profile")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	native := NewNative(profile)
	native.getenv = func(name string) string {
		if name == "COUPANGCTL_BROWSER_PATH" {
			return executable
		}
		return ""
	}
	native.sessionFactory = func(context.Context, string, string) (documentSession, error) {
		return &fakeDocumentSession{categoryErr: ErrStructuredCategoryDataMissing}, nil
	}
	native.headedSessionFactory = nil

	_, err := native.FetchProductCategory(context.Background(), core.ProductReference{ProductID: "101", VendorItemID: "201"})
	if !errors.Is(err, core.ErrProductCategoryUnavailable) {
		t.Fatalf("category fetch error = %v, want typed unavailable", err)
	}
}

func TestFetchUsesReadOnlyCursorURLAndReusableSession(t *testing.T) {
	executable := syntheticExecutable(t, "exit 0\n")
	profile := filepath.Join(t.TempDir(), "browser-profile")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	session := &fakeDocumentSession{document: []byte(`{"props":{}}`)}
	native := NewNative(profile)
	native.getenv = func(name string) string {
		if name == "COUPANGCTL_BROWSER_PATH" {
			return executable
		}
		if name == "DISPLAY" {
			return ":synthetic"
		}
		return ""
	}
	native.sessionFactory = func(context.Context, string, string) (documentSession, error) {
		return session, nil
	}

	for _, cursor := range []*core.OrderCursor{nil, {Year: 2026, Page: 2}} {
		if _, err := native.Fetch(context.Background(), cursor); err != nil {
			t.Fatal(err)
		}
	}
	if len(session.urls) != 2 || session.urls[0] != orderListURL || session.urls[1] != orderModelURL+"?pageIndex=2&requestYear=2026&size=5" {
		t.Fatalf("unexpected read-only URLs: %#v", session.urls)
	}
	if err := native.Close(); err != nil {
		t.Fatal(err)
	}
	if !session.closed {
		t.Fatal("document session was not closed")
	}
}

func TestReceiptReadsDefaultBoundsBeforeReachingNarrowSession(t *testing.T) {
	executable := syntheticExecutable(t, "exit 0\n")
	profile := filepath.Join(t.TempDir(), "browser-profile")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	session := &fakeDocumentSession{receiptDocument: []byte(`{"success":true}`)}
	native := NewNative(profile)
	native.getenv = func(name string) string {
		if name == "COUPANGCTL_BROWSER_PATH" {
			return executable
		}
		return ""
	}
	native.sessionFactory = func(context.Context, string, string) (documentSession, error) { return session, nil }

	if _, err := native.FetchReceiptHistory(context.Background(), core.ReceiptHistoryRequest{Kind: core.ReceiptKindCard}); err != nil {
		t.Fatal(err)
	}
	if _, err := native.FetchReceiptSummary(context.Background(), core.ReceiptSummaryRequest{Kind: core.ReceiptKindCard, From: "2026-01-01", To: "2026-09-03"}); err != nil {
		t.Fatal(err)
	}
	if _, err := native.FetchReceiptDownload(context.Background(), core.ReceiptDownloadRequest{Kind: core.ReceiptKindCash, HistoryIndex: 0}); err != nil {
		t.Fatal(err)
	}
	if len(session.receiptHistory) != 1 || session.receiptHistory[0].PageSize != 5 || len(session.receiptSummaries) != 1 || session.receiptSummaries[0].MaxCards != 20 || len(session.receiptDownloads) != 1 || session.receiptDownloads[0].PageSize != 5 {
		t.Fatalf("unexpected receipt bounds: history=%#v summary=%#v download=%#v", session.receiptHistory, session.receiptSummaries, session.receiptDownloads)
	}
}

func TestFetchVendorReceiptResolvesOnlyHashedOrderReference(t *testing.T) {
	executable := syntheticExecutable(t, "exit 0\n")
	profile := filepath.Join(t.TempDir(), "browser-profile")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	rawOrderID := "1234567890"
	sourceRef := core.OrderSourceReference(rawOrderID)
	session := &fakeDocumentSession{
		document:        []byte(`{"orderList":[{"orderId":"` + rawOrderID + `"}],"hasNext":false}`),
		receiptDocument: []byte(`{"found":true,"source_ref":"` + sourceRef + `","pages_scanned":1,"vendors":[]}`),
	}
	native := NewNative(profile)
	native.getenv = func(name string) string {
		if name == "COUPANGCTL_BROWSER_PATH" {
			return executable
		}
		return ""
	}
	native.sessionFactory = func(context.Context, string, string) (documentSession, error) { return session, nil }

	document, err := native.FetchVendorReceipt(context.Background(), core.VendorReceiptRequest{SourceRef: sourceRef, MaxPages: 3})
	if err != nil || string(document) != string(session.receiptDocument) {
		t.Fatalf("vendor receipt document = %q, %v", document, err)
	}
	if len(session.vendorOrderIDs) != 1 || session.vendorOrderIDs[0] != rawOrderID || session.vendorSourceRefs[0] != sourceRef || session.vendorPages[0] != 1 {
		t.Fatalf("unexpected vendor lookup: ids=%#v refs=%#v pages=%#v", session.vendorOrderIDs, session.vendorSourceRefs, session.vendorPages)
	}
}

func TestLocateOrderReferenceUsesStructuredPaginationWithoutExposingID(t *testing.T) {
	rawOrderID := "9876543210"
	sourceRef := core.OrderSourceReference(rawOrderID)
	document := []byte(`{"orderList":[{"orderId":"` + rawOrderID + `"}],"orderPagination":{"hasNext":true,"nextYear":2025,"nextPageIndex":2}}`)
	orderID, next, found, err := locateOrderReference(document, sourceRef)
	if err != nil || !found || orderID != rawOrderID || next != nil {
		t.Fatalf("matching lookup = %q %#v %v %v", orderID, next, found, err)
	}
	_, next, found, err = locateOrderReference(document, core.OrderSourceReference("different"))
	if err != nil || found || next == nil || next.Year != 2025 || next.Page != 2 {
		t.Fatalf("pagination lookup = %#v %v %v", next, found, err)
	}
}

func TestReceiptResearchMetadataUsesDedicatedNarrowSession(t *testing.T) {
	executable := syntheticExecutable(t, "exit 0\n")
	profile := filepath.Join(t.TempDir(), "browser-profile")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	session := &fakeDocumentSession{researchDocument: []byte(`{"schema_version":1,"operation":"read_only_get_metadata"}`)}
	native := NewNative(profile)
	native.getenv = func(name string) string {
		if name == "COUPANGCTL_BROWSER_PATH" {
			return executable
		}
		return ""
	}
	native.sessionFactory = func(context.Context, string, string) (documentSession, error) { return session, nil }

	document, err := native.FetchReceiptResearchMetadata(context.Background(), []ReceiptResearchOrderSample{{OrderID: "12345", State: "ordinary"}})
	if err != nil || string(document) != string(session.researchDocument) {
		t.Fatalf("research metadata = %q, %v", document, err)
	}
	if len(session.researchSamples) != 1 || session.researchSamples[0].OrderID != "12345" || session.researchSamples[0].State != "ordinary" {
		t.Fatalf("research samples = %#v", session.researchSamples)
	}
	if _, err := native.FetchReceiptResearchMetadata(context.Background(), []ReceiptResearchOrderSample{{OrderID: "not-numeric", State: "ordinary"}}); err == nil {
		t.Fatal("invalid research order reference was accepted")
	}
	if _, err := native.FetchReceiptResearchMetadata(context.Background(), []ReceiptResearchOrderSample{{OrderID: "12345", State: "private-value"}}); err == nil {
		t.Fatal("invalid research state was accepted")
	}
}

func TestReceiptReadRetriesOneTransientStructuredMiss(t *testing.T) {
	executable := syntheticExecutable(t, "exit 0\n")
	profile := filepath.Join(t.TempDir(), "browser-profile")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	session := &fakeDocumentSession{
		receiptDocument: []byte(`{"kind":"card"}`),
		receiptErrors:   []error{ErrStructuredReceiptDataMissing, nil},
	}
	native := NewNative(profile)
	native.getenv = func(name string) string {
		if name == "COUPANGCTL_BROWSER_PATH" {
			return executable
		}
		return ""
	}
	native.sessionFactory = func(context.Context, string, string) (documentSession, error) { return session, nil }

	request := core.ReceiptSummaryRequest{Kind: core.ReceiptKindCard, From: "2026-01-01", To: "2026-09-03"}
	document, err := native.FetchReceiptSummary(context.Background(), request)
	if err != nil || string(document) != `{"kind":"card"}` || len(session.receiptSummaries) != 2 {
		t.Fatalf("transient read was not retried once: document=%q reads=%d err=%v", document, len(session.receiptSummaries), err)
	}
}

func TestValidateOrderTargetRestrictsEachReadOnlyEndpoint(t *testing.T) {
	for _, target := range []string{
		orderListURL,
		orderListURL + "?pageIndex=9&periodYear=2026",
		orderModelURL + "?pageIndex=0&requestYear=2025&size=5",
	} {
		if err := validateOrderTarget(target); err != nil {
			t.Fatalf("valid target %q rejected: %v", target, err)
		}
	}
	for _, target := range []string{
		orderListURL + "?requestYear=2025",
		orderModelURL + "?pageIndex=0&periodYear=2025&size=5",
		orderModelURL + "?pageIndex=0&requestYear=2025&size=100",
		"https://example.test/ssr/api/myorders/model?pageIndex=0&requestYear=2025&size=5",
	} {
		if err := validateOrderTarget(target); err == nil {
			t.Fatalf("invalid target %q accepted", target)
		}
	}
}

func TestProductSearchURLUsesObservedNativeSortersAndCategoryPaths(t *testing.T) {
	for _, test := range []struct {
		request core.ProductSearchRequest
		want    string
	}{
		{request: core.ProductSearchRequest{Query: "synthetic laptop", Sort: core.ProductSortCoupangRanking}, want: "https://www.coupang.com/np/search?q=synthetic+laptop&sorter=scoreDesc"},
		{request: core.ProductSearchRequest{Query: "synthetic laptop", Sort: core.ProductSortSales}, want: "https://www.coupang.com/np/search?q=synthetic+laptop&sorter=saleCountDesc"},
		{request: core.ProductSearchRequest{CategoryID: "12345", Sort: core.ProductSortLatest}, want: "https://www.coupang.com/np/categories/12345?sorter=latestAsc"},
	} {
		got, err := productSearchURL(test.request)
		if err != nil || got != test.want {
			t.Fatalf("productSearchURL(%#v) = %q, %v; want %q", test.request, got, err, test.want)
		}
	}
}

func TestSyncBrowserIsHeadlessWhileManualLoginRemainsHeaded(t *testing.T) {
	arguments := strings.Join(syncBrowserArguments("/synthetic/profile", 49152, true), " ")
	if !strings.Contains(arguments, "--headless=new") {
		t.Fatalf("sync browser arguments are not headless: %s", arguments)
	}
	if !strings.Contains(arguments, "--remote-debugging-address=127.0.0.1") {
		t.Fatalf("sync browser control is not restricted to loopback: %s", arguments)
	}
	headedArguments := strings.Join(syncBrowserArguments("/synthetic/profile", 49152, false), " ")
	if strings.Contains(headedArguments, "--headless") {
		t.Fatalf("headed fallback unexpectedly contains a headless flag: %s", headedArguments)
	}
}

func TestMacOSBundleExtraction(t *testing.T) {
	got := macOSAppBundle("/Applications/Google Chrome.app/Contents/MacOS/Google Chrome")
	if got != "/Applications/Google Chrome.app" {
		t.Fatalf("bundle = %q", got)
	}
	if got := macOSAppBundle("/synthetic/browser"); got != "" {
		t.Fatalf("non-bundle executable produced %q", got)
	}
}

func TestBrowserCommandLaunchesResolvedExecutableDirectly(t *testing.T) {
	executable := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	command := browserCommand(context.Background(), executable, "--user-data-dir=/synthetic/profile", "about:blank")
	if command.Path != executable {
		t.Fatalf("browser command path = %q, want %q", command.Path, executable)
	}
	if len(command.Args) != 3 || command.Args[1] != "--user-data-dir=/synthetic/profile" || command.Args[2] != "about:blank" {
		t.Fatalf("unexpected direct browser argv: %#v", command.Args)
	}
}

func TestLoginTargetPreservesProtectedReturnContext(t *testing.T) {
	for _, test := range []struct {
		mode core.LoginMode
		want string
	}{
		{mode: core.LoginModeQR, want: orderListURL},
		{mode: core.LoginModePhone, want: orderListURL},
	} {
		if got := loginTargetURL(core.LoginRequest{Mode: test.mode}); got != test.want {
			t.Fatalf("login target for %q = %q, want %q", test.mode, got, test.want)
		}
	}
}

func TestQROutputUsesNarrowPresenterWithoutManualBrowserLaunch(t *testing.T) {
	executable := syntheticExecutable(t, "exit 88\n")
	profile := filepath.Join(t.TempDir(), "browser-profile")
	output := filepath.Join(t.TempDir(), "login-qr.png")
	native := NewNative(profile)
	native.getenv = func(name string) string {
		if name == "COUPANGCTL_BROWSER_PATH" {
			return executable
		}
		if name == "DISPLAY" {
			return ":synthetic"
		}
		return ""
	}
	var gotExecutable, gotProfile, gotTarget, gotOutput string
	native.presentQR = func(_ context.Context, executable, profile, target, output string, _ core.QRLinkPresenter) error {
		gotExecutable, gotProfile, gotTarget, gotOutput = executable, profile, target, output
		return nil
	}
	request := core.LoginRequest{Mode: core.LoginModeQR, QROutputPath: output}
	if err := native.Login(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if gotExecutable != executable || gotProfile != profile || gotTarget != orderListURL || gotOutput != output {
		t.Fatalf("unexpected QR presenter arguments: %q %q %q %q", gotExecutable, gotProfile, gotTarget, gotOutput)
	}
}

func TestDefaultQRUsesNarrowPresenterWithoutWritingAnImage(t *testing.T) {
	executable := syntheticExecutable(t, "exit 88\n")
	profile := filepath.Join(t.TempDir(), "browser-profile")
	native := NewNative(profile)
	native.getenv = func(name string) string {
		if name == "COUPANGCTL_BROWSER_PATH" {
			return executable
		}
		if name == "DISPLAY" {
			return ":synthetic"
		}
		return ""
	}
	called := false
	native.presentQR = func(_ context.Context, _, _, target, output string, _ core.QRLinkPresenter) error {
		called = true
		if target != orderListURL || output != "" {
			t.Fatalf("target/output = %q/%q", target, output)
		}
		return nil
	}
	if err := native.Login(context.Background(), core.LoginRequest{Mode: core.LoginModeQR}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("QR presenter was not called")
	}
}

func TestQRLinkPresenterStaysInsideNarrowBrowserAdapter(t *testing.T) {
	executable := syntheticExecutable(t, "exit 88\n")
	profile := filepath.Join(t.TempDir(), "browser-profile")
	native := NewNative(profile)
	native.getenv = func(name string) string {
		if name == "COUPANGCTL_BROWSER_PATH" {
			return executable
		}
		if name == "DISPLAY" {
			return ":synthetic"
		}
		return ""
	}
	presenter := func(context.Context, core.QRLoginLink) error { return nil }
	native.presentQR = func(_ context.Context, _, _, target, output string, got core.QRLinkPresenter) error {
		if target != orderListURL || output != "" || got == nil {
			t.Fatalf("presenter target/output/callback was not preserved")
		}
		return nil
	}
	if err := native.Login(context.Background(), core.LoginRequest{Mode: core.LoginModeQR, PresentQRLink: presenter}); err != nil {
		t.Fatal(err)
	}
}

func TestAutomatedPhoneLoginUsesNarrowPresenter(t *testing.T) {
	executable := syntheticExecutable(t, "exit 88\n")
	profile := filepath.Join(t.TempDir(), "browser-profile")
	native := NewNative(profile)
	native.getenv = func(name string) string {
		if name == "COUPANGCTL_BROWSER_PATH" {
			return executable
		}
		if name == "DISPLAY" {
			return ":synthetic"
		}
		return ""
	}
	called := false
	native.presentPhone = func(_ context.Context, gotExecutable, gotProfile, target, phone string, readOTP core.OTPProvider) error {
		called = true
		if gotExecutable != executable || gotProfile != profile || target != orderListURL {
			t.Fatalf("unexpected presenter target: %q %q %q", gotExecutable, gotProfile, target)
		}
		if phone != "01000000000" || readOTP == nil {
			t.Fatal("phone presenter did not receive private challenge inputs")
		}
		return nil
	}
	request := core.LoginRequest{
		Mode:  core.LoginModePhone,
		Phone: "01000000000",
		ReadOTP: func(context.Context) (string, error) {
			return "000000", nil
		},
	}
	if err := native.Login(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("phone presenter was not called")
	}
}

func syntheticExecutable(t *testing.T, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("synthetic shell executable is Unix-specific")
	}
	path := filepath.Join(t.TempDir(), "synthetic-browser")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
