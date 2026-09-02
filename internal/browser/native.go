package browser

import (
	"context"
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

	"github.com/JungHoonGhae/oss-coupangctl/internal/auth"
	"github.com/JungHoonGhae/oss-coupangctl/internal/core"
)

const loginURL = "https://login.coupang.com/login/login.pang"
const orderListURL = "https://mc.coupang.com/ssr/desktop/order/list"
const orderModelURL = "https://mc.coupang.com/ssr/api/myorders/model"

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

type qrPresenter func(context.Context, string, string, string, string, core.QRLinkPresenter) error
type phonePresenter func(context.Context, string, string, string, string, core.OTPProvider) error

type Native struct {
	profileDir           string
	getenv               func(string) string
	lookPath             func(string) (string, error)
	sessionFactory       func(context.Context, string, string) (documentSession, error)
	headedSessionFactory func(context.Context, string, string) (documentSession, error)
	allowHeadedFallback  func() bool
	presentQR            qrPresenter
	presentPhone         phonePresenter
	mu                   sync.Mutex
	session              documentSession
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
	document, err := n.session.ReadOrderDocument(ctx, targetURL)
	if !errors.Is(err, ErrBrowserAccessDenied) || n.headedSessionFactory == nil || n.allowHeadedFallback == nil || !n.allowHeadedFallback() {
		return document, err
	}
	_ = n.session.Close()
	n.session = nil
	n.session, err = n.headedSessionFactory(ctx, path, n.profileDir)
	if err != nil {
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
