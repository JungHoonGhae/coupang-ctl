package browser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/auth"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

const loginURL = "https://login.coupang.com/login/login.pang"
const orderListURL = "https://mc.coupang.com/ssr/desktop/order/list"
const orderModelURL = "https://mc.coupang.com/ssr/api/myorders/model"
const orderAccessRetryDelay = 10 * time.Second

var ErrBrowserNotFound = errors.New("supported Chrome-family browser not found")
var ErrDesktopRequired = errors.New("interactive desktop required")
var ErrAuthenticationRequired = core.ErrAuthenticationRequired

type documentSession interface {
	ReadOrderDocument(context.Context, string) ([]byte, error)
	Close() error
}

type productCategoryDocumentSession interface {
	ReadProductCategoryDocument(context.Context, string) ([]byte, error)
}

type productSearchDocumentSession interface {
	ReadProductSearchDocument(context.Context, string) ([]byte, error)
}

type productInspectionDocumentSession interface {
	ReadProductInspectionDocument(context.Context, string, core.ProductInspectRequest) ([]byte, error)
}

type productCartDocumentSession interface {
	ReadProductCartAddDocument(context.Context, string, core.CartAddRequest) ([]byte, error)
}

type accountBenefitsDocumentSession interface {
	ReadAccountBenefitsDocument(context.Context, int) ([]byte, error)
}

type receiptDocumentSession interface {
	ReadReceiptStatusDocument(context.Context) ([]byte, error)
	ReadReceiptHistoryDocument(context.Context, core.ReceiptHistoryRequest) ([]byte, error)
	ReadReceiptSummaryDocument(context.Context, core.ReceiptSummaryRequest) ([]byte, error)
	ReadReceiptDownloadDocument(context.Context, core.ReceiptDownloadRequest) ([]byte, error)
	ReadVendorReceiptDocument(context.Context, string, string, int) ([]byte, error)
}

type receiptResearchDocumentSession interface {
	ReadReceiptResearchMetadata(context.Context, []ReceiptResearchOrderSample) ([]byte, error)
}

type ReceiptResearchOrderSample struct {
	OrderID string `json:"order_id"`
	State   string `json:"state"`
}

type qrPresenter func(context.Context, string, string, string, string, core.QRLinkPresenter) error
type phonePresenter func(context.Context, string, string, string, string, core.OTPProvider) error

type Native struct {
	profileDir            string
	getenv                func(string) string
	lookPath              func(string) (string, error)
	sessionFactory        func(context.Context, string, string) (documentSession, error)
	headedSessionFactory  func(context.Context, string, string) (documentSession, error)
	allowHeadedFallback   func() bool
	presentQR             qrPresenter
	presentPhone          phonePresenter
	waitBeforeAccessRetry func(context.Context) error
	mu                    sync.Mutex
	session               documentSession
}

func NewNative(profileDir string) *Native {
	native := &Native{
		profileDir:           profileDir,
		getenv:               os.Getenv,
		lookPath:             exec.LookPath,
		sessionFactory:       newChromeSession,
		headedSessionFactory: newHeadedChromeSession,
		presentQR:            presentQRLogin,
		presentPhone:         presentPhoneLogin,
		waitBeforeAccessRetry: func(ctx context.Context) error {
			timer := time.NewTimer(orderAccessRetryDelay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}
	native.allowHeadedFallback = func() bool { return desktopAvailable(native.getenv) }
	return native
}

func NewNativeHeadedSync(profileDir string) *Native {
	native := NewNative(profileDir)
	native.sessionFactory = newHeadedChromeSession
	native.headedSessionFactory = nil
	return native
}

func (n *Native) Verify(ctx context.Context) error {
	_, err := n.Fetch(ctx, nil)
	closeErr := n.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func (n *Native) Fetch(ctx context.Context, cursor *core.OrderCursor) ([]byte, error) {
	present, err := directoryPresent(n.profileDir)
	if err != nil {
		return nil, fmt.Errorf("inspect dedicated browser profile: %w", err)
	}
	if !present {
		return nil, ErrAuthenticationRequired
	}
	path, err := n.discover()
	if err != nil {
		return nil, err
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	if n.session == nil {
		n.session, err = n.sessionFactory(ctx, path, n.profileDir)
		if err != nil {
			return nil, err
		}
	}
	targetURL := orderDocumentURL(cursor)
	document, err := n.readOrderDocument(ctx, targetURL)
	if !errors.Is(err, ErrBrowserAccessDenied) || n.headedSessionFactory == nil || n.allowHeadedFallback == nil || !n.allowHeadedFallback() {
		return document, err
	}
	_ = n.session.Close()
	n.session = nil
	n.session, err = n.headedSessionFactory(ctx, path, n.profileDir)
	if err != nil {
		return nil, err
	}
	return n.readOrderDocument(ctx, targetURL)
}

func (n *Native) readOrderDocument(ctx context.Context, targetURL string) ([]byte, error) {
	document, err := n.session.ReadOrderDocument(ctx, targetURL)
	if !errors.Is(err, ErrBrowserAccessDenied) || n.waitBeforeAccessRetry == nil {
		return document, err
	}
	if err := n.waitBeforeAccessRetry(ctx); err != nil {
		return nil, err
	}
	return n.session.ReadOrderDocument(ctx, targetURL)
}

func (n *Native) FetchProductCategory(ctx context.Context, reference core.ProductReference) ([]byte, error) {
	if !numericReference(reference.ProductID) || !numericReference(reference.VendorItemID) {
		return nil, errors.New("invalid product category reference")
	}
	present, err := directoryPresent(n.profileDir)
	if err != nil {
		return nil, fmt.Errorf("inspect dedicated browser profile: %w", err)
	}
	if !present {
		return nil, ErrAuthenticationRequired
	}
	path, err := n.discover()
	if err != nil {
		return nil, err
	}
	targetURL := fmt.Sprintf("https://www.coupang.com/vp/products/%s?vendorItemId=%s", reference.ProductID, reference.VendorItemID)

	n.mu.Lock()
	defer n.mu.Unlock()
	if n.session == nil {
		n.session, err = n.sessionFactory(ctx, path, n.profileDir)
		if err != nil {
			return nil, err
		}
	}
	read := func() ([]byte, error) {
		categorySession, ok := n.session.(productCategoryDocumentSession)
		if !ok {
			return nil, ErrStructuredCategoryDataMissing
		}
		return categorySession.ReadProductCategoryDocument(ctx, targetURL)
	}
	document, err := read()
	headlessRejected := errors.Is(err, ErrBrowserAccessDenied) || errors.Is(err, ErrAuthenticationRequired)
	if !headlessRejected || n.headedSessionFactory == nil || n.allowHeadedFallback == nil || !n.allowHeadedFallback() {
		if productCategoryUnavailable(err) {
			return nil, core.ErrProductCategoryUnavailable
		}
		return document, err
	}
	_ = n.session.Close()
	n.session = nil
	n.session, err = n.headedSessionFactory(ctx, path, n.profileDir)
	if err != nil {
		return nil, err
	}
	document, err = read()
	if productCategoryUnavailable(err) {
		return nil, core.ErrProductCategoryUnavailable
	}
	return document, err
}

func (n *Native) FetchProductSearch(ctx context.Context, request core.ProductSearchRequest) ([]byte, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	targetURL, err := productSearchURL(request)
	if err != nil {
		return nil, err
	}
	return n.fetchPublicProductDocument(ctx, true, func(session documentSession) ([]byte, error) {
		productSession, ok := session.(productSearchDocumentSession)
		if !ok {
			return nil, ErrStructuredProductDataMissing
		}
		return productSession.ReadProductSearchDocument(ctx, targetURL)
	})
}

func (n *Native) FetchProductInspection(ctx context.Context, request core.ProductInspectRequest) ([]byte, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	query := url.Values{}
	if request.ItemID != "" {
		query.Set("itemId", request.ItemID)
	}
	if request.VendorItemID != "" {
		query.Set("vendorItemId", request.VendorItemID)
	}
	targetURL := "https://www.coupang.com/vp/products/" + request.ProductID
	if encoded := query.Encode(); encoded != "" {
		targetURL += "?" + encoded
	}
	return n.fetchPublicProductDocument(ctx, true, func(session documentSession) ([]byte, error) {
		productSession, ok := session.(productInspectionDocumentSession)
		if !ok {
			return nil, ErrStructuredProductDataMissing
		}
		return productSession.ReadProductInspectionDocument(ctx, targetURL, request)
	})
}

func (n *Native) AddProductToCart(ctx context.Context, request core.CartAddRequest) ([]byte, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	query := url.Values{"vendorItemId": []string{request.VendorItemID}}
	if request.ItemID != "" {
		query.Set("itemId", request.ItemID)
	}
	targetURL := "https://www.coupang.com/vp/products/" + request.ProductID + "?" + query.Encode()
	return n.fetchPublicProductDocument(ctx, false, func(session documentSession) ([]byte, error) {
		productSession, ok := session.(productCartDocumentSession)
		if !ok {
			return nil, ErrStructuredProductDataMissing
		}
		return productSession.ReadProductCartAddDocument(ctx, targetURL, request)
	})
}

func (n *Native) FetchAccountBenefits(ctx context.Context, request core.AccountBenefitsRequest) ([]byte, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if request.MaxCashTransactionPages == 0 {
		request.MaxCashTransactionPages = 50
	}
	present, err := directoryPresent(n.profileDir)
	if err != nil {
		return nil, fmt.Errorf("inspect dedicated browser profile: %w", err)
	}
	if !present {
		return nil, ErrAuthenticationRequired
	}
	path, err := n.discover()
	if err != nil {
		return nil, err
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	if n.session == nil {
		n.session, err = n.sessionFactory(ctx, path, n.profileDir)
		if err != nil {
			return nil, err
		}
	}
	read := func() ([]byte, error) {
		session, ok := n.session.(accountBenefitsDocumentSession)
		if !ok {
			return nil, ErrStructuredAccountBenefitsDataMissing
		}
		return session.ReadAccountBenefitsDocument(ctx, request.MaxCashTransactionPages)
	}
	document, err := read()
	if !errors.Is(err, ErrBrowserAccessDenied) || n.headedSessionFactory == nil || n.allowHeadedFallback == nil || !n.allowHeadedFallback() {
		return document, err
	}
	_ = n.session.Close()
	n.session = nil
	n.session, err = n.headedSessionFactory(ctx, path, n.profileDir)
	if err != nil {
		return nil, err
	}
	return read()
}

func (n *Native) FetchReceiptStatus(ctx context.Context) ([]byte, error) {
	return n.fetchReceiptDocument(ctx, func(session receiptDocumentSession) ([]byte, error) {
		return session.ReadReceiptStatusDocument(ctx)
	})
}

func (n *Native) FetchReceiptHistory(ctx context.Context, request core.ReceiptHistoryRequest) ([]byte, error) {
	if request.PageSize == 0 {
		request.PageSize = 5
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	return n.fetchReceiptDocument(ctx, func(session receiptDocumentSession) ([]byte, error) {
		return session.ReadReceiptHistoryDocument(ctx, request)
	})
}

func (n *Native) FetchReceiptSummary(ctx context.Context, request core.ReceiptSummaryRequest) ([]byte, error) {
	if request.MaxCards == 0 {
		request.MaxCards = 20
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	return n.fetchReceiptDocument(ctx, func(session receiptDocumentSession) ([]byte, error) {
		return session.ReadReceiptSummaryDocument(ctx, request)
	})
}

func (n *Native) FetchReceiptDownload(ctx context.Context, request core.ReceiptDownloadRequest) ([]byte, error) {
	if request.PageSize == 0 {
		request.PageSize = 5
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	return n.fetchReceiptDocument(ctx, func(session receiptDocumentSession) ([]byte, error) {
		return session.ReadReceiptDownloadDocument(ctx, request)
	})
}

func (n *Native) FetchVendorReceipt(ctx context.Context, request core.VendorReceiptRequest) ([]byte, error) {
	if request.MaxPages == 0 {
		request.MaxPages = 1000
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	present, err := directoryPresent(n.profileDir)
	if err != nil {
		return nil, fmt.Errorf("inspect dedicated browser profile: %w", err)
	}
	if !present {
		return nil, ErrAuthenticationRequired
	}
	path, err := n.discover()
	if err != nil {
		return nil, err
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	if n.session == nil {
		n.session, err = n.sessionFactory(ctx, path, n.profileDir)
		if err != nil {
			return nil, err
		}
	}
	read := func() ([]byte, error) {
		return readVendorReceiptFromSession(ctx, n.session, request)
	}
	document, err := read()
	if !errors.Is(err, ErrBrowserAccessDenied) || n.headedSessionFactory == nil || n.allowHeadedFallback == nil || !n.allowHeadedFallback() {
		return document, err
	}
	_ = n.session.Close()
	n.session = nil
	n.session, err = n.headedSessionFactory(ctx, path, n.profileDir)
	if err != nil {
		return nil, err
	}
	return read()
}

func readVendorReceiptFromSession(ctx context.Context, session documentSession, request core.VendorReceiptRequest) ([]byte, error) {
	receipts, ok := session.(receiptDocumentSession)
	if !ok {
		return nil, ErrStructuredReceiptDataMissing
	}
	var cursor *core.OrderCursor
	seen := map[string]bool{}
	for pagesScanned := 1; pagesScanned <= request.MaxPages; pagesScanned++ {
		key := "initial"
		if cursor != nil {
			key = fmt.Sprintf("%d/%d", cursor.Year, cursor.Page)
		}
		if seen[key] {
			return nil, ErrStructuredOrderDataMissing
		}
		seen[key] = true
		document, err := session.ReadOrderDocument(ctx, orderDocumentURL(cursor))
		if err != nil {
			return nil, err
		}
		orderID, next, found, err := locateOrderReference(document, request.SourceRef)
		if err != nil {
			return nil, ErrStructuredOrderDataMissing
		}
		if found {
			return receipts.ReadVendorReceiptDocument(ctx, orderID, request.SourceRef, pagesScanned)
		}
		if next == nil {
			return nil, core.ErrVendorReceiptNotFound
		}
		cursor = next
	}
	return nil, core.ErrVendorReceiptNotFound
}

func locateOrderReference(document []byte, sourceRef string) (string, *core.OrderCursor, bool, error) {
	payload := bytes.TrimSpace(document)
	if len(payload) == 0 {
		return "", nil, false, errors.New("empty order document")
	}
	if payload[0] != '{' {
		var err error
		payload, err = embeddedNextData(payload)
		if err != nil {
			return "", nil, false, err
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var root map[string]any
	if err := decoder.Decode(&root); err != nil {
		return "", nil, false, err
	}
	domain := root
	for _, key := range []string{"props", "pageProps", "domains", "desktopOrder"} {
		next, ok := domain[key].(map[string]any)
		if !ok {
			if _, modelOK := root["orderList"]; modelOK {
				domain = root
				break
			}
			return "", nil, false, errors.New("order domain missing")
		}
		domain = next
	}
	orders, ok := domain["orderList"].([]any)
	if !ok {
		return "", nil, false, errors.New("order list missing")
	}
	for _, raw := range orders {
		order, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		orderID := scalarOrderID(order["orderId"])
		if orderID != "" && core.OrderSourceReference(orderID) == sourceRef {
			return orderID, nil, true, nil
		}
	}
	pagination := domain
	if embedded, ok := domain["orderPagination"].(map[string]any); ok {
		pagination = embedded
	}
	hasNext, _ := pagination["hasNext"].(bool)
	if !hasNext {
		return "", nil, false, nil
	}
	year, yearOK := browserInteger(pagination["nextYear"])
	page, pageOK := browserInteger(pagination["nextPageIndex"])
	if !yearOK || !pageOK || year < 2000 || page < 0 {
		return "", nil, false, errors.New("invalid order cursor")
	}
	return "", &core.OrderCursor{Year: year, Page: page}, false, nil
}

func embeddedNextData(document []byte) ([]byte, error) {
	remaining := document
	for {
		start := bytes.Index(remaining, []byte("<script"))
		if start < 0 {
			return nil, errors.New("next data missing")
		}
		remaining = remaining[start:]
		tagEnd := bytes.IndexByte(remaining, '>')
		if tagEnd < 0 {
			return nil, errors.New("script tag malformed")
		}
		tag := remaining[:tagEnd+1]
		body := remaining[tagEnd+1:]
		end := bytes.Index(body, []byte("</script>"))
		if end < 0 {
			return nil, errors.New("script body malformed")
		}
		if bytes.Contains(tag, []byte(`id="__NEXT_DATA__"`)) || bytes.Contains(tag, []byte(`id='__NEXT_DATA__'`)) {
			return bytes.TrimSpace(body[:end]), nil
		}
		remaining = body[end+len("</script>"):]
	}
}

func scalarOrderID(value any) string {
	switch typed := value.(type) {
	case string:
		if numericReference(typed) {
			return typed
		}
	case json.Number:
		if _, err := typed.Int64(); err == nil && numericReference(typed.String()) {
			return typed.String()
		}
	}
	return ""
}

func browserInteger(value any) (int, bool) {
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Int64()
		return int(parsed), err == nil && parsed == int64(int(parsed))
	case float64:
		return int(typed), typed == float64(int(typed))
	default:
		return 0, false
	}
}

// FetchReceiptResearchMetadata is intentionally outside the typed receipt
// product adapter. It returns a browser-sanitized key/path report used to
// decide whether an unstable read contract is safe to adopt.
func (n *Native) FetchReceiptResearchMetadata(ctx context.Context, samples []ReceiptResearchOrderSample) ([]byte, error) {
	if len(samples) > 5 {
		return nil, errors.New("receipt research sample cannot exceed five orders")
	}
	allowedStates := map[string]bool{
		"ordinary": true, "fully_canceled": true, "canceled_units": true,
		"returned_units": true, "canceled_and_returned_units": true,
	}
	for _, sample := range samples {
		if !numericReference(sample.OrderID) || !allowedStates[sample.State] {
			return nil, errors.New("receipt research sample contains an invalid order reference")
		}
	}
	return n.fetchReceiptResearchDocument(ctx, func(session receiptResearchDocumentSession) ([]byte, error) {
		return session.ReadReceiptResearchMetadata(ctx, samples)
	})
}

func (n *Native) fetchReceiptResearchDocument(ctx context.Context, read func(receiptResearchDocumentSession) ([]byte, error)) ([]byte, error) {
	present, err := directoryPresent(n.profileDir)
	if err != nil {
		return nil, fmt.Errorf("inspect dedicated browser profile: %w", err)
	}
	if !present {
		return nil, ErrAuthenticationRequired
	}
	path, err := n.discover()
	if err != nil {
		return nil, err
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	if n.session == nil {
		n.session, err = n.sessionFactory(ctx, path, n.profileDir)
		if err != nil {
			return nil, err
		}
	}
	invoke := func() ([]byte, error) {
		session, ok := n.session.(receiptResearchDocumentSession)
		if !ok {
			return nil, ErrStructuredReceiptDataMissing
		}
		return read(session)
	}
	document, err := invoke()
	if !errors.Is(err, ErrBrowserAccessDenied) || n.headedSessionFactory == nil || n.allowHeadedFallback == nil || !n.allowHeadedFallback() {
		return document, err
	}
	_ = n.session.Close()
	n.session = nil
	n.session, err = n.headedSessionFactory(ctx, path, n.profileDir)
	if err != nil {
		return nil, err
	}
	return invoke()
}

func (n *Native) fetchReceiptDocument(ctx context.Context, read func(receiptDocumentSession) ([]byte, error)) ([]byte, error) {
	present, err := directoryPresent(n.profileDir)
	if err != nil {
		return nil, fmt.Errorf("inspect dedicated browser profile: %w", err)
	}
	if !present {
		return nil, ErrAuthenticationRequired
	}
	path, err := n.discover()
	if err != nil {
		return nil, err
	}

	n.mu.Lock()
	defer n.mu.Unlock()
	if n.session == nil {
		n.session, err = n.sessionFactory(ctx, path, n.profileDir)
		if err != nil {
			return nil, err
		}
	}
	readCurrent := func() ([]byte, error) {
		session, ok := n.session.(receiptDocumentSession)
		if !ok {
			return nil, ErrStructuredReceiptDataMissing
		}
		return read(session)
	}
	document, err := readCurrent()
	if errors.Is(err, ErrStructuredReceiptDataMissing) {
		document, err = readCurrent()
	}
	if !errors.Is(err, ErrBrowserAccessDenied) || n.headedSessionFactory == nil || n.allowHeadedFallback == nil || !n.allowHeadedFallback() {
		return document, err
	}
	_ = n.session.Close()
	n.session = nil
	n.session, err = n.headedSessionFactory(ctx, path, n.profileDir)
	if err != nil {
		return nil, err
	}
	return readCurrent()
}

func (n *Native) fetchPublicProductDocument(ctx context.Context, retryMissingRead bool, read func(documentSession) ([]byte, error)) ([]byte, error) {
	if err := os.MkdirAll(n.profileDir, 0o700); err != nil {
		return nil, fmt.Errorf("create product browser profile: %w", err)
	}
	path, err := n.discover()
	if err != nil {
		return nil, err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.session == nil {
		n.session, err = n.sessionFactory(ctx, path, n.profileDir)
		if err != nil {
			return nil, err
		}
	}
	document, err := read(n.session)
	retryable := errors.Is(err, ErrBrowserAccessDenied) || (retryMissingRead && errors.Is(err, ErrStructuredProductDataMissing))
	if !retryable || n.headedSessionFactory == nil || n.allowHeadedFallback == nil || !n.allowHeadedFallback() {
		return document, err
	}
	_ = n.session.Close()
	n.session = nil
	n.session, err = n.headedSessionFactory(ctx, path, n.profileDir)
	if err != nil {
		return nil, err
	}
	return read(n.session)
}

func productSearchURL(request core.ProductSearchRequest) (string, error) {
	if err := request.Validate(); err != nil {
		return "", err
	}
	path := "/np/search"
	query := url.Values{}
	if request.CategoryID != "" {
		path = "/np/categories/" + request.CategoryID
	} else {
		query.Set("q", strings.TrimSpace(request.Query))
	}
	sorter := map[core.ProductSort]string{
		core.ProductSortCoupangRanking: "scoreDesc",
		core.ProductSortSales:          "saleCountDesc",
		core.ProductSortLatest:         "latestAsc",
		core.ProductSortPriceAsc:       "salePriceAsc",
		core.ProductSortPriceDesc:      "salePriceDesc",
	}[request.Sort]
	if sorter != "" {
		query.Set("sorter", sorter)
	}
	target := "https://www.coupang.com" + path
	if encoded := query.Encode(); encoded != "" {
		target += "?" + encoded
	}
	return target, nil
}

func productCategoryUnavailable(err error) bool {
	return errors.Is(err, ErrBrowserAccessDenied) || errors.Is(err, ErrAuthenticationRequired) || errors.Is(err, ErrStructuredCategoryDataMissing)
}

func numericReference(value string) bool {
	if value == "" || len(value) > 24 {
		return false
	}
	_, err := strconv.ParseUint(value, 10, 64)
	return err == nil
}

func (n *Native) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.session == nil {
		return nil
	}
	err := n.session.Close()
	n.session = nil
	return err
}

func orderDocumentURL(cursor *core.OrderCursor) string {
	if cursor == nil {
		return orderListURL
	}
	return fmt.Sprintf("%s?pageIndex=%d&requestYear=%d&size=5", orderModelURL, cursor.Page, cursor.Year)
}

func (n *Native) Inspect(ctx context.Context) (auth.BrowserStatus, error) {
	path, err := n.discover()
	if err != nil {
		return auth.BrowserStatus{}, err
	}
	present, err := directoryPresent(n.profileDir)
	if err != nil {
		return auth.BrowserStatus{}, fmt.Errorf("inspect browser profile: %w", err)
	}
	return auth.BrowserStatus{Name: filepath.Base(path), ProfilePresent: present}, nil
}

func (n *Native) Login(ctx context.Context, request core.LoginRequest) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if !desktopAvailable(n.getenv) {
		return ErrDesktopRequired
	}
	path, err := n.discover()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(n.profileDir, 0o700); err != nil {
		return fmt.Errorf("create browser profile: %w", err)
	}
	if err := os.Chmod(n.profileDir, 0o700); err != nil && runtime.GOOS != "windows" {
		return fmt.Errorf("secure browser profile: %w", err)
	}
	if request.Mode == core.LoginModeQR {
		return n.presentQR(ctx, path, n.profileDir, loginTargetURL(request), request.QROutputPath, request.PresentQRLink)
	}
	if request.Mode == core.LoginModePhone {
		return n.presentPhone(ctx, path, n.profileDir, orderListURL, request.Phone, request.ReadOTP)
	}
	return core.ErrInvalidLoginMode
}

func loginTargetURL(core.LoginRequest) string {
	return orderListURL
}

func browserCommand(ctx context.Context, executable string, arguments ...string) *exec.Cmd {
	return exec.CommandContext(ctx, executable, arguments...)
}

func macOSAppBundle(executable string) string {
	const marker = ".app/Contents/MacOS/"
	index := strings.Index(executable, marker)
	if index < 0 {
		return ""
	}
	return executable[:index+len(".app")]
}

func (n *Native) discover() (string, error) {
	if override := n.getenv("COUPANGCTL_BROWSER_PATH"); override != "" {
		if err := validateExecutable(override); err != nil {
			return "", fmt.Errorf("COUPANGCTL_BROWSER_PATH: %w", err)
		}
		return override, nil
	}

	for _, candidate := range browserCandidates(n.getenv) {
		if filepath.IsAbs(candidate) {
			if validateExecutable(candidate) == nil {
				return candidate, nil
			}
			continue
		}
		if path, err := n.lookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", ErrBrowserNotFound
}

func browserCandidates(getenv func(string) string) []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
	case "windows":
		roots := []string{getenv("LOCALAPPDATA"), getenv("PROGRAMFILES"), getenv("PROGRAMFILES(X86)")}
		var paths []string
		for _, root := range roots {
			if root == "" {
				continue
			}
			paths = append(paths,
				filepath.Join(root, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(root, "Microsoft", "Edge", "Application", "msedge.exe"),
			)
		}
		return paths
	default:
		return []string{"google-chrome-stable", "google-chrome", "chromium", "chromium-browser", "microsoft-edge-stable"}
	}
}

func desktopAvailable(getenv func(string) string) bool {
	if runtime.GOOS != "linux" {
		return true
	}
	return strings.TrimSpace(getenv("DISPLAY")) != "" || strings.TrimSpace(getenv("WAYLAND_DISPLAY")) != ""
}

func validateExecutable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		return fmt.Errorf("not executable")
	}
	return nil
}

func directoryPresent(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}
