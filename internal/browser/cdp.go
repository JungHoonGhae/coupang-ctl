package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"

	"github.com/JungHoonGhae/coupang-ctl/internal/browser/sessionbridge"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	sessionstate "github.com/JungHoonGhae/coupang-ctl/internal/session"
)

const (
	cdpStartupTimeout       = 15 * time.Second
	pageLoadTimeout         = 25 * time.Second
	maxCDPMessageSize       = 10 << 20
	maxProductDocumentBytes = 3 << 20
	maxReceiptDownloadBytes = 5 << 20
)

var ErrStructuredOrderDataMissing = errors.New("structured order data missing")
var ErrStructuredCategoryDataMissing = errors.New("structured category data missing")
var ErrStructuredProductDataMissing = errors.New("structured product data missing")
var ErrStructuredAccountBenefitsDataMissing = errors.New("structured account benefits data missing")
var ErrStructuredReceiptDataMissing = errors.New("structured receipt data missing")
var ErrBrowserAccessDenied = errors.New("browser access denied")

type chromeSession struct {
	command    *exec.Cmd
	baseURL    string
	httpClient *http.Client
	browser    *cdpClient
	sessions   *sessionbridge.Bridge
	closeOnce  sync.Once
}

type cdpClient struct {
	connection *websocket.Conn
	nextID     int64
	mu         sync.Mutex
}

type cdpRequest struct {
	ID     int64  `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params,omitempty"`
}

type cdpResponse struct {
	ID     int64           `json:"id,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type versionMetadata struct {
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

type targetMetadata struct {
	ID                   string `json:"id"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

const (
	wowMembershipURL  = "https://loyalty.coupang.com/loyalty/management/home"
	coupangCashURL    = "https://cash.coupang.com/coupang-cash/home"
	paymentReceiptURL = "https://mc.coupang.com/ssr/desktop/payment-receipt"
)

func newChromeSession(ctx context.Context, executable, profileDir string) (documentSession, error) {
	return startChromeSession(ctx, executable, profileDir, true)
}

func newHeadedChromeSession(ctx context.Context, executable, profileDir string) (documentSession, error) {
	return startChromeSession(ctx, executable, profileDir, false)
}

func startChromeSession(ctx context.Context, executable, profileDir string, headless bool) (documentSession, error) {
	port, err := availableLoopbackPort()
	if err != nil {
		return nil, fmt.Errorf("allocate local browser control port: %w", err)
	}
	command := browserCommand(ctx, executable, syncBrowserArguments(profileDir, port, headless)...)
	command.Stdout = io.Discard
	command.Stderr = io.Discard
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start read-only browser session: %w", err)
	}

	transport := &http.Transport{Proxy: nil}
	httpClient := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	session := &chromeSession{
		command:    command,
		baseURL:    "http://127.0.0.1:" + strconv.Itoa(port),
		httpClient: httpClient,
	}
	startupCtx, cancel := context.WithTimeout(ctx, cdpStartupTimeout)
	defer cancel()
	metadata, err := session.waitForVersion(startupCtx)
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		transport.CloseIdleConnections()
		return nil, err
	}
	session.browser, err = dialCDP(startupCtx, metadata.WebSocketDebuggerURL)
	if err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		transport.CloseIdleConnections()
		return nil, fmt.Errorf("connect local browser control channel: %w", err)
	}
	session.sessions = sessionbridge.New(
		sessionstate.NewFileStore(filepath.Join(filepath.Dir(profileDir), "session.json")),
		time.Now,
	)
	if err := session.sessions.Restore(startupCtx, session.browser); err != nil {
		_ = session.browser.Close()
		_ = command.Process.Kill()
		_ = command.Wait()
		transport.CloseIdleConnections()
		return nil, err
	}
	return session, nil
}

func syncBrowserArguments(profileDir string, port int, headless bool) []string {
	arguments := []string{
		"--user-data-dir=" + profileDir,
		"--remote-debugging-address=127.0.0.1",
		"--remote-debugging-port=" + strconv.Itoa(port),
		"--no-first-run",
		"--no-default-browser-check",
		"about:blank",
	}
	if headless {
		arguments = append(arguments[:1], append([]string{"--headless=new"}, arguments[1:]...)...)
	}
	return arguments
}

func (s *chromeSession) ReadOrderDocument(ctx context.Context, targetURL string) ([]byte, error) {
	if err := validateOrderTarget(targetURL); err != nil {
		return nil, err
	}
	navigationURL := targetURL
	readStructuredModel := isOrderModelURL(targetURL)
	if readStructuredModel {
		// The private model route is an authenticated same-origin fetch used by
		// the order UI, not a standalone document route.
		navigationURL = orderListURL
	}
	var created struct {
		TargetID string `json:"targetId"`
	}
	if err := s.browser.Call(ctx, "Target.createTarget", map[string]any{"url": "about:blank"}, &created); err != nil {
		return nil, fmt.Errorf("create read-only browser page: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.browser.Call(closeCtx, "Target.closeTarget", map[string]any{"targetId": created.TargetID}, nil)
	}()

	target, err := s.waitForTarget(ctx, created.TargetID)
	if err != nil {
		return nil, err
	}
	page, err := dialCDP(ctx, target.WebSocketDebuggerURL)
	if err != nil {
		return nil, fmt.Errorf("connect read-only browser page: %w", err)
	}
	defer page.Close()
	if err := page.Call(ctx, "Page.enable", struct{}{}, nil); err != nil {
		return nil, fmt.Errorf("enable read-only browser page: %w", err)
	}
	if err := page.Call(ctx, "Page.navigate", map[string]any{"url": navigationURL}, nil); err != nil {
		return nil, fmt.Errorf("navigate read-only browser page: %w", err)
	}
	document, err := waitForOrderDocument(ctx, page)
	if err != nil {
		return nil, err
	}
	if readStructuredModel {
		document, err = fetchOrderModelDocument(ctx, page, targetURL)
		if err != nil {
			return nil, err
		}
	}
	if err := s.persistBrowserSession(ctx); err != nil {
		return nil, err
	}
	return document, nil
}

func (s *chromeSession) ReadProductCategoryDocument(ctx context.Context, targetURL string) ([]byte, error) {
	if err := validateProductCategoryTarget(targetURL); err != nil {
		return nil, err
	}
	var created struct {
		TargetID string `json:"targetId"`
	}
	if err := s.browser.Call(ctx, "Target.createTarget", map[string]any{"url": "about:blank"}, &created); err != nil {
		return nil, fmt.Errorf("create product category page: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.browser.Call(closeCtx, "Target.closeTarget", map[string]any{"targetId": created.TargetID}, nil)
	}()

	target, err := s.waitForTarget(ctx, created.TargetID)
	if err != nil {
		return nil, err
	}
	page, err := dialCDP(ctx, target.WebSocketDebuggerURL)
	if err != nil {
		return nil, fmt.Errorf("connect product category page: %w", err)
	}
	defer page.Close()
	if err := page.Call(ctx, "Page.enable", struct{}{}, nil); err != nil {
		return nil, fmt.Errorf("enable product category page: %w", err)
	}
	if err := page.Call(ctx, "Page.navigate", map[string]any{"url": targetURL}, nil); err != nil {
		return nil, fmt.Errorf("navigate product category page: %w", err)
	}
	document, err := waitForProductCategoryDocument(ctx, page)
	if err != nil {
		return nil, err
	}
	if err := s.persistBrowserSession(ctx); err != nil {
		return nil, err
	}
	return document, nil
}

func (s *chromeSession) ReadProductSearchDocument(ctx context.Context, targetURL string) ([]byte, error) {
	if err := validateProductSearchTarget(targetURL); err != nil {
		return nil, err
	}
	return s.readProductPage(ctx, targetURL, waitForProductSearchDocument)
}

func (s *chromeSession) ReadProductInspectionDocument(ctx context.Context, targetURL string, request core.ProductInspectRequest) ([]byte, error) {
	if err := validateProductInspectionTarget(targetURL, request); err != nil {
		return nil, err
	}
	return s.readProductPage(ctx, targetURL, func(ctx context.Context, page *cdpClient) ([]byte, error) {
		if err := waitForProductDetailReady(ctx, page); err != nil {
			return nil, err
		}
		return extractProductInspection(ctx, page, request)
	})
}

func (s *chromeSession) ReadProductCartAddDocument(ctx context.Context, targetURL string, request core.CartAddRequest) ([]byte, error) {
	if err := validateProductCartTarget(targetURL, request); err != nil {
		return nil, err
	}
	return s.readProductPage(ctx, targetURL, func(ctx context.Context, page *cdpClient) ([]byte, error) {
		if err := waitForProductDetailReady(ctx, page); err != nil {
			return nil, err
		}
		return pressProductCartControl(ctx, page, request)
	})
}

func (s *chromeSession) ReadAccountBenefitsDocument(ctx context.Context, maxCashPages int) ([]byte, error) {
	if maxCashPages < 1 || maxCashPages > 100 {
		return nil, ErrStructuredAccountBenefitsDataMissing
	}
	membership, err := s.readAccountPage(ctx, wowMembershipURL, waitForMembershipData)
	if err != nil {
		return nil, err
	}
	cash, err := s.readAccountPage(ctx, coupangCashURL, func(ctx context.Context, page *cdpClient) ([]byte, error) {
		return waitOnCashData(ctx, page, maxCashPages)
	})
	if err != nil {
		return nil, err
	}
	var cashResult struct {
		Summary json.RawMessage   `json:"summary"`
		Pages   []json.RawMessage `json:"pages"`
	}
	if json.Unmarshal(cash, &cashResult) != nil {
		return nil, ErrStructuredAccountBenefitsDataMissing
	}
	result, err := json.Marshal(struct {
		Membership           json.RawMessage   `json:"membership"`
		CashSummary          json.RawMessage   `json:"cash_summary,omitempty"`
		CashTransactionPages []json.RawMessage `json:"cash_transaction_pages"`
	}{Membership: membership, CashSummary: cashResult.Summary, CashTransactionPages: cashResult.Pages})
	if err != nil {
		return nil, ErrStructuredAccountBenefitsDataMissing
	}
	if err := s.persistBrowserSession(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *chromeSession) ReadReceiptStatusDocument(ctx context.Context) ([]byte, error) {
	return s.readReceiptPage(ctx, `(async () => {
		const read = async (path) => {
			const response = await fetch(path, { credentials: 'include' });
			if (!response.ok) throw new Error('receipt status unavailable');
			return await response.json();
		};
		const [cash, card] = await Promise.all([
			read('/ssr/api/payment-receipt/cash/request-status'),
			read('/ssr/api/payment-receipt/card/request-status'),
		]);
		return JSON.stringify({ cash, card });
	})()`)
}

func (s *chromeSession) ReadReceiptHistoryDocument(ctx context.Context, request core.ReceiptHistoryRequest) ([]byte, error) {
	if err := request.Validate(); err != nil {
		return nil, ErrStructuredReceiptDataMissing
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return nil, ErrStructuredReceiptDataMissing
	}
	expression := `(async () => {
		const request = ` + string(encoded) + `;
		const path = request.kind === 'cash'
			? '/ssr/api/payment-receipt/cash/download-request-histories'
			: '/ssr/api/payment-receipt/card/download-request-histories';
		const target = new URL(path, location.origin);
		target.searchParams.set('pageIndex', String(request.page_index ?? 0));
		target.searchParams.set('size', String(request.page_size));
		const response = await fetch(target, { credentials: 'include' });
		if (!response.ok) throw new Error('receipt history unavailable');
		return JSON.stringify(await response.json());
	})()`
	return s.readReceiptPage(ctx, expression)
}

func (s *chromeSession) ReadReceiptSummaryDocument(ctx context.Context, request core.ReceiptSummaryRequest) ([]byte, error) {
	if err := request.Validate(); err != nil {
		return nil, ErrStructuredReceiptDataMissing
	}
	request.From = strings.ReplaceAll(request.From, "-", ".")
	request.To = strings.ReplaceAll(request.To, "-", ".")
	encoded, err := json.Marshal(request)
	if err != nil {
		return nil, ErrStructuredReceiptDataMissing
	}
	expression := `(async () => {
		const request = ` + string(encoded) + `;
		const path = request.kind === 'cash'
			? '/ssr/api/payment-receipt/cash/receipt-summary'
			: '/ssr/api/payment-receipt/card/receipt-summary';
		const read = async (fields) => {
			const target = new URL(path, location.origin);
			for (const [name, value] of Object.entries(fields)) target.searchParams.set(name, String(value ?? ''));
			let last = null;
			for (let attempt = 0; attempt < 2; attempt += 1) {
				const response = await fetch(target, { credentials: 'include', cache: 'no-store' });
				if (response.ok) {
					last = await response.json();
					if (last?.success === true) return last;
				}
				if (attempt === 0) await new Promise((resolve) => setTimeout(resolve, 200));
			}
			if (last !== null) return last;
			throw new Error('receipt summary unavailable');
		};
		const base = { from: request.from, to: request.to };
		if (request.kind === 'cash') {
			return JSON.stringify({ kind: 'cash', summary: await read(base), per_card: [] });
		}
		const summary = await read({ ...base, cardId: '', cardNumber: '', displayCardName: '' });
		const cards = Array.isArray(summary?.data?.cardList) ? summary.data.cardList.slice(0, request.max_cards) : [];
		const perCard = [];
		for (const card of cards) {
			perCard.push(await read({
				...base,
				cardId: card?.cardId ?? '',
				cardNumber: card?.cardNumber ?? '',
				displayCardName: card?.displayCardName ?? '',
			}));
		}
		return JSON.stringify({ kind: 'card', summary, per_card: perCard });
	})()`
	return s.readReceiptPage(ctx, expression)
}

func (s *chromeSession) ReadReceiptDownloadDocument(ctx context.Context, request core.ReceiptDownloadRequest) ([]byte, error) {
	if err := request.Validate(); err != nil {
		return nil, ErrStructuredReceiptDataMissing
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		return nil, ErrStructuredReceiptDataMissing
	}
	expression := fmt.Sprintf(`(async () => {
		const request = %s;
		const path = request.kind === 'cash'
			? '/ssr/api/payment-receipt/cash/download-request-histories'
			: '/ssr/api/payment-receipt/card/download-request-histories';
		const historyTarget = new URL(path, location.origin);
		historyTarget.searchParams.set('pageIndex', String(request.page_index ?? 0));
		historyTarget.searchParams.set('size', String(request.page_size));
		const historyResponse = await fetch(historyTarget, { credentials: 'include' });
		if (!historyResponse.ok) throw new Error('receipt history unavailable');
		const history = await historyResponse.json();
		const item = history?.data?.list?.[request.history_index];
		const rawURL = item?.downloadUrlList?.[request.download_index]?.downloadUrl;
		if (typeof rawURL !== 'string' || rawURL.length > 4096) throw new Error('receipt download missing');
		const allowed = (target) => target.protocol === 'https:' && (
			target.hostname === 'mc.coupang.com' || target.hostname.endsWith('.coupang.com') ||
			target.hostname === 'coupangcdn.com' || target.hostname.endsWith('.coupangcdn.com'));
		const target = new URL(rawURL, location.origin);
		if (!allowed(target)) throw new Error('receipt download blocked');
		const response = await fetch(target, { credentials: 'include', redirect: 'follow' });
		const finalTarget = new URL(response.url || target.href);
		if (!response.ok || !allowed(finalTarget)) throw new Error('receipt download unavailable');
		const content = new Uint8Array(await response.arrayBuffer());
		if (content.length < 1 || content.length > %d) throw new Error('receipt download too large');
		let binary = '';
		for (let offset = 0; offset < content.length; offset += 32768) {
			binary += String.fromCharCode(...content.subarray(offset, Math.min(content.length, offset + 32768)));
		}
		const disposition = response.headers.get('content-disposition') ?? '';
		const filenameMatch = disposition.match(/filename\*?=(?:UTF-8''|["']?)([^"';]+)/i);
		return JSON.stringify({
			filename: filenameMatch ? decodeURIComponent(filenameMatch[1]) : 'coupang-receipt',
			content_type: response.headers.get('content-type') ?? 'application/octet-stream',
			bytes: content.length,
			base64: btoa(binary),
		});
	})()`, string(encoded), maxReceiptDownloadBytes)
	return s.readReceiptPage(ctx, expression)
}

func (s *chromeSession) readReceiptPage(ctx context.Context, expression string) ([]byte, error) {
	var created struct {
		TargetID string `json:"targetId"`
	}
	if err := s.browser.Call(ctx, "Target.createTarget", map[string]any{"url": "about:blank"}, &created); err != nil {
		return nil, fmt.Errorf("create receipt page: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.browser.Call(closeCtx, "Target.closeTarget", map[string]any{"targetId": created.TargetID}, nil)
	}()
	target, err := s.waitForTarget(ctx, created.TargetID)
	if err != nil {
		return nil, err
	}
	page, err := dialCDP(ctx, target.WebSocketDebuggerURL)
	if err != nil {
		return nil, fmt.Errorf("connect receipt page: %w", err)
	}
	defer page.Close()
	if err := page.Call(ctx, "Page.enable", struct{}{}, nil); err != nil {
		return nil, fmt.Errorf("enable receipt page: %w", err)
	}
	if err := page.Call(ctx, "Page.navigate", map[string]any{"url": paymentReceiptURL}, nil); err != nil {
		return nil, fmt.Errorf("navigate receipt page: %w", err)
	}
	if err := waitForReceiptPageReady(ctx, page); err != nil {
		return nil, err
	}
	encodedDocument, err := evaluateBrowserString(ctx, page, expression, true)
	if err != nil || len(encodedDocument) == 0 || len(encodedDocument) > maxCDPMessageSize || !json.Valid([]byte(encodedDocument)) {
		return nil, ErrStructuredReceiptDataMissing
	}
	if err := s.persistBrowserSession(ctx); err != nil {
		return nil, err
	}
	return []byte(encodedDocument), nil
}

func (s *chromeSession) readAccountPage(ctx context.Context, targetURL string, read func(context.Context, *cdpClient) ([]byte, error)) ([]byte, error) {
	if err := validateAccountBenefitsTarget(targetURL); err != nil {
		return nil, err
	}
	var created struct {
		TargetID string `json:"targetId"`
	}
	if err := s.browser.Call(ctx, "Target.createTarget", map[string]any{"url": "about:blank"}, &created); err != nil {
		return nil, fmt.Errorf("create account benefits page: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.browser.Call(closeCtx, "Target.closeTarget", map[string]any{"targetId": created.TargetID}, nil)
	}()
	target, err := s.waitForTarget(ctx, created.TargetID)
	if err != nil {
		return nil, err
	}
	page, err := dialCDP(ctx, target.WebSocketDebuggerURL)
	if err != nil {
		return nil, fmt.Errorf("connect account benefits page: %w", err)
	}
	defer page.Close()
	if err := page.Call(ctx, "Page.enable", struct{}{}, nil); err != nil {
		return nil, fmt.Errorf("enable account benefits page: %w", err)
	}
	if err := page.Call(ctx, "Page.navigate", map[string]any{"url": targetURL}, nil); err != nil {
		return nil, fmt.Errorf("navigate account benefits page: %w", err)
	}
	return read(ctx, page)
}

func (s *chromeSession) readProductPage(ctx context.Context, targetURL string, read func(context.Context, *cdpClient) ([]byte, error)) ([]byte, error) {
	var created struct {
		TargetID string `json:"targetId"`
	}
	if err := s.browser.Call(ctx, "Target.createTarget", map[string]any{"url": "about:blank"}, &created); err != nil {
		return nil, fmt.Errorf("create product page: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.browser.Call(closeCtx, "Target.closeTarget", map[string]any{"targetId": created.TargetID}, nil)
	}()
	target, err := s.waitForTarget(ctx, created.TargetID)
	if err != nil {
		return nil, err
	}
	page, err := dialCDP(ctx, target.WebSocketDebuggerURL)
	if err != nil {
		return nil, fmt.Errorf("connect product page: %w", err)
	}
	defer page.Close()
	if err := page.Call(ctx, "Page.enable", struct{}{}, nil); err != nil {
		return nil, fmt.Errorf("enable product page: %w", err)
	}
	if err := page.Call(ctx, "Page.navigate", map[string]any{"url": targetURL}, nil); err != nil {
		return nil, fmt.Errorf("navigate product page: %w", err)
	}
	document, err := read(ctx, page)
	if err != nil {
		return nil, err
	}
	if err := s.persistBrowserSession(ctx); err != nil {
		return nil, err
	}
	return document, nil
}

func waitForProductSearchDocument(ctx context.Context, page *cdpClient) ([]byte, error) {
	const expression = `(() => {
		const body = document.body?.innerText ?? '';
		const navigation = performance.getEntriesByType('navigation')[0];
		const cleanText = (value, limit = 300) => (value ?? '').replace(/\s+/g, ' ').trim().slice(0, limit);
		const numeric = (value) => {
			const match = cleanText(value, 80).replace(/,/g, '').match(/\d+(?:\.\d+)?/);
			return match ? Number(match[0]) : 0;
		};
		const imageURL = (raw) => {
			if (!raw) return '';
			try {
				const parsed = new URL(raw, location.origin);
				return parsed.protocol === 'https:' && /(^|\.)coupangcdn\.com$/i.test(parsed.hostname) ? parsed.href : '';
			} catch { return ''; }
		};
		const seen = new Set();
		const items = [];
		const sourceSort = new URL(location.href).searchParams.get('sorter') || 'scoreDesc';
		for (const anchor of document.querySelectorAll('a[href*="/vp/products/"]')) {
			if (items.length >= 60) break;
			let parsed;
			try { parsed = new URL(anchor.href, location.origin); } catch { continue; }
			const match = parsed.pathname.match(/^\/vp\/products\/(\d+)$/);
			if (!match || parsed.hostname !== 'www.coupang.com') continue;
			const productId = match[1];
			const itemId = parsed.searchParams.get('itemId') ?? '';
			const vendorItemId = parsed.searchParams.get('vendorItemId') ?? '';
			const key = productId + '/' + vendorItemId;
			if (seen.has(key)) continue;
			const card = anchor.closest('li, article, [class*="productUnit"]') ?? anchor.parentElement;
			if (!card) continue;
			const selectText = (...selectors) => {
				for (const selector of selectors) {
					const element = card.querySelector(selector);
					const text = cleanText(element?.textContent);
					if (text) return { text, found: true };
				}
				return { text: '', found: false };
			};
			const nameValue = selectText('[class*="productName"]', '[class*="ProductName"]', '.name');
			const image = card.querySelector('img');
			const name = nameValue.text || cleanText(image?.alt) || cleanText(anchor.getAttribute('title'));
			if (!name) continue;
			const priceArea = card.querySelector('[class*="PriceArea"], [class*="priceArea"]');
			const currentElement = priceArea ? Array.from(priceArea.querySelectorAll('*')).find((element) =>
				element.children.length === 0 && element.tagName !== 'DEL' && /\d[\d,]*\s*원/.test(cleanText(element.textContent, 80))) : null;
			const originalElement = priceArea?.querySelector('del') ?? null;
			const current = { text: cleanText(currentElement?.textContent, 80), found: currentElement !== null };
			const original = { text: cleanText(originalElement?.textContent, 80), found: originalElement !== null };
			const currentAmount = Math.round(numeric(current.text));
			const originalAmount = Math.round(numeric(original.text));
			const computedDiscount = originalAmount > currentAmount && currentAmount > 0
				? Math.round((originalAmount - currentAmount) * 100 / originalAmount) : 0;
			const discount = { text: String(computedDiscount), found: computedDiscount > 0 };
			const ratingArea = card.querySelector('[class*="ProductRating"], [class*="productRating"]');
			const ratingElement = ratingArea?.querySelector('[aria-label]') ?? null;
			const rating = { text: cleanText(ratingElement?.getAttribute('aria-label'), 20), found: ratingElement !== null };
			const reviewMatch = cleanText(ratingArea?.textContent, 100).match(/\(([\d,]+)\)/);
			const review = { text: reviewMatch?.[1] ?? '', found: reviewMatch !== null };
			const cardText = cleanText(card.textContent, 2000);
			const canonical = new URL('/vp/products/' + productId, location.origin);
			if (/^\d+$/.test(itemId)) canonical.searchParams.set('itemId', itemId);
			if (/^\d+$/.test(vendorItemId)) canonical.searchParams.set('vendorItemId', vendorItemId);
			const observed = ['name'];
			if (current.found) observed.push('price.current_amount');
			if (original.found) observed.push('price.original_amount');
			if (discount.found) observed.push('price.discount_rate');
			if (rating.found) observed.push('rating');
			if (review.found) observed.push('review_count');
			observed.push('search_position');
			seen.add(key);
			items.push({
				product_id: productId, item_id: /^\d+$/.test(itemId) ? itemId : '',
				vendor_item_id: /^\d+$/.test(vendorItemId) ? vendorItemId : '',
				name, url: canonical.href,
				image_url: imageURL(image?.currentSrc || image?.getAttribute('src') || image?.getAttribute('data-img-src')),
				current_amount: currentAmount, original_amount: originalAmount,
				discount_rate: Math.min(100, Math.round(numeric(discount.text))),
				rating: Math.min(5, numeric(rating.text)), review_count: Math.round(numeric(review.text)),
				rocket: /로켓|rocket/i.test(cardText), free_shipping: /무료\s*배송/.test(cardText),
				coupon: /쿠폰/.test(cardText), sponsored: /(^|\s)광고(\s|$)/.test(cardText), observed_fields: observed,
				search_position: items.length + 1, rank_source: 'coupang_search_order',
				review_scope: review.found ? 'product_page_observed' : '', source_sort: sourceSort,
			});
		}
		const noResults = /검색\s*결과.{0,12}(없|0개)|검색결과가 없습니다/i.test(body);
		return JSON.stringify({
			href: location.href, ready: document.readyState, status: navigation?.responseStatus ?? 0,
			login_form: document.querySelector('input[type="password"]') !== null,
			challenge: /captcha|보안문자|자동입력방지/i.test(body),
			access_denied: /access denied|접근.{0,8}(거부|제한)|비정상.{0,8}접근/i.test(body),
			items, no_results: noResults,
			coverage: { source: 'coupang_search_document', observed_fields: ['identity','name','image','price','rating','reviews','delivery_badges','search_position','native_sort_controls'], unavailable_fields: [] }
		});
	})()`
	return waitForProductEnvelope(ctx, page, expression, true)
}

func waitForProductDetailReady(ctx context.Context, page *cdpClient) error {
	const expression = `(() => JSON.stringify({
		href: location.href,
		ready: document.readyState,
		status: performance.getEntriesByType('navigation')[0]?.responseStatus ?? 0,
		login_form: document.querySelector('input[type="password"]') !== null,
		challenge: /captcha|보안문자|자동입력방지/i.test(document.body?.innerText ?? ''),
		access_denied: /access denied|접근.{0,8}(거부|제한)|비정상.{0,8}접근/i.test(document.body?.innerText ?? ''),
		items: document.querySelectorAll('script[type="application/ld+json"]').length > 0 ? [{}] : []
	}))()`
	_, err := waitForProductEnvelope(ctx, page, expression, true)
	return err
}

func extractProductInspection(ctx context.Context, page *cdpClient, request core.ProductInspectRequest) ([]byte, error) {
	encodedRequest, err := json.Marshal(request)
	if err != nil {
		return nil, ErrStructuredProductDataMissing
	}
	expression := `(async () => {
		const request = ` + string(encodedRequest) + `;
		const cleanText = (value, limit = 1500) => (typeof value === 'string' || typeof value === 'number' ? String(value) : '')
			.replace(/\s+/g, ' ').trim().slice(0, limit);
		const numeric = (value) => {
			if (typeof value === 'number' && Number.isFinite(value)) return value;
			const match = cleanText(value, 100).replace(/,/g, '').match(/\d+(?:\.\d+)?/);
			return match ? Number(match[0]) : 0;
		};
		const money = (value, depth = 0) => {
			if (depth > 3 || value == null) return 0;
			if (typeof value === 'number') return Math.max(0, Math.round(value));
			if (typeof value === 'string') return Math.max(0, Math.round(numeric(value)));
			if (typeof value !== 'object') return 0;
			for (const key of ['amount','value','finalPrice','salePrice','price','priceValue']) {
				if (key in value) { const found = money(value[key], depth + 1); if (found) return found; }
			}
			return 0;
		};
		const imageURL = (raw) => {
			if (!raw) return '';
			try {
				const parsed = new URL(raw, location.origin);
				return parsed.protocol === 'https:' && /(^|\.)coupangcdn\.com$/i.test(parsed.hostname) ? parsed.href : '';
			} catch { return ''; }
		};
		const unique = (values, limit) => [...new Set(values.filter(Boolean))].slice(0, limit);
		for (let attempt = 0; attempt < 25; attempt++) {
			if (document.querySelector('.option-picker-container, .option-picker-select .select-item.selected')) break;
			await new Promise((resolve) => setTimeout(resolve, 100));
		}
		const jsonLD = [];
		for (const script of document.querySelectorAll('script[type="application/ld+json"]')) {
			try { jsonLD.push(JSON.parse(script.textContent || 'null')); } catch {}
		}
		const objects = [];
		const walk = (value, depth = 0) => {
			if (depth > 5 || value == null || typeof value !== 'object') return;
			if (Array.isArray(value)) { for (const item of value) walk(item, depth + 1); return; }
			objects.push(value);
			for (const key of ['@graph','itemListElement','item']) if (key in value) walk(value[key], depth + 1);
		};
		for (const value of jsonLD) walk(value);
		const isType = (value, wanted) => Array.isArray(value) ? value.includes(wanted) : value === wanted;
		const structured = objects.find((value) => isType(value['@type'], 'Product')) ?? {};
		const offers = Array.isArray(structured.offers) ? structured.offers[0] ?? {} : structured.offers ?? {};
		const aggregate = structured.aggregateRating ?? {};
		const bodyText = cleanText(document.body?.innerText, 10000);
		const name = cleanText(structured.name, 300) || cleanText(document.querySelector('h1')?.textContent, 300);
		const description = cleanText(structured.description, 5000) || cleanText(document.querySelector('[class*="description"]')?.textContent, 5000);
		const gallery = unique([
			...(Array.isArray(structured.image) ? structured.image : [structured.image]),
			...Array.from(document.querySelectorAll('[class*="gallery"] img, [class*="thumbnail"] img, .prod-image img'))
				.map((node) => node.currentSrc || node.getAttribute('src') || node.getAttribute('data-img-src')),
		].map(imageURL), 50);
		const detailImages = unique(Array.from(document.querySelectorAll(
			'#productDetail img, .product-detail img, [class*="productDetail"] img, [id*="productDetail"] img, [class*="detail-content"] img'
		)).map((node) => imageURL(node.currentSrc || node.getAttribute('src') || node.getAttribute('data-src') || node.getAttribute('data-img-src'))), 50);
		const specifications = unique(Array.from(document.querySelectorAll(
			'#itemBrief li, #itemBrief tr, [class*="attribute"] li, [class*="spec"] tr, .prod-description-attribute'
		)).map((node) => cleanText(node.textContent, 300)).filter((value) => value.length >= 2), 50);
		const selectedOptions = unique(Array.from(document.querySelectorAll(
			'.option-picker-select .select-item.selected'
		)).map((node) => cleanText(node.textContent, 300)).filter((value) => value.length >= 1 && value.length <= 300), 20);

		const safeFetch = async (target) => {
			try {
				const response = await fetch(target, { credentials: 'include', headers: { accept: 'application/json' } });
				if (!response.ok) return null;
				return await response.json();
			} catch { return null; }
		};
		const productId = request.product_id;
		const vendorItemId = request.vendor_item_id || new URL(location.href).searchParams.get('vendorItemId') || '';
		const quantityTarget = /^\d+$/.test(vendorItemId)
			? '/next-api/products/quantity-info?productId=' + encodeURIComponent(productId) + '&vendorItemId=' + encodeURIComponent(vendorItemId) : '';
		const reviewTarget = '/next-api/review?productId=' + encodeURIComponent(productId) +
			'&page=1&size=' + Math.max(1, Math.min(20, request.review_limit || 5)) + '&sortBy=ORDER_SCORE_ASC&ratingSummary=true&ratings=&market=';
		const [quantityPayload, reviewPayload] = await Promise.all([
			quantityTarget ? safeFetch(quantityTarget) : Promise.resolve(null), safeFetch(reviewTarget),
		]);
		const quantity = Array.isArray(quantityPayload) ? quantityPayload[0] :
			(Array.isArray(quantityPayload?.data) ? quantityPayload.data[0] : quantityPayload?.data ?? quantityPayload ?? {});
		const reviewRoot = reviewPayload?.rData ?? reviewPayload?.data ?? reviewPayload ?? {};
		const paging = reviewRoot?.paging ?? {};
		const reviewRows = Array.isArray(paging?.contents) ? paging.contents :
			(Array.isArray(reviewRoot?.contents) ? reviewRoot.contents : Array.isArray(reviewRoot?.reviews) ? reviewRoot.reviews : []);
		const reviews = reviewRows.slice(0, Math.max(1, Math.min(20, request.review_limit || 5))).map((row) => {
			const images = Array.isArray(row?.images) ? row.images : Array.isArray(row?.attachments) ? row.attachments : [];
			return {
				rating: Math.min(5, numeric(row?.rating ?? row?.ratingScore ?? row?.reviewRating)),
				content: cleanText(row?.content ?? row?.reviewContent ?? row?.title, 1500),
				created_date: cleanText(row?.createdAt ?? row?.createdDate ?? row?.date, 40),
				helpful_count: Math.round(numeric(row?.helpfulCount ?? row?.helpCount)),
				image_urls: unique(images.map((image) => imageURL(typeof image === 'string' ? image : image?.imageUrl ?? image?.url)), 10),
			};
		}).filter((review) => review.content || review.rating || review.image_urls.length);
		const ratingRoot = reviewRoot?.ratingSummaryTotal ?? aggregate ?? {};
		const distribution = {};
		const ratingRows = Array.isArray(ratingRoot?.ratingSummaries) ? ratingRoot.ratingSummaries : [];
		for (const row of ratingRows) {
			const star = Math.round(numeric(row?.rating ?? row?.score ?? row?.star));
			const count = Math.round(numeric(row?.count ?? row?.ratingCount));
			if (star >= 1 && star <= 5 && count >= 0) distribution[String(star)] = count;
		}
		const benefitNodes = Array.from(document.querySelectorAll('[class*="coupon"], [class*="cardBenefit"], [class*="cashback"], [class*="promotion"]'));
		const benefitTexts = unique(benefitNodes.map((node) => cleanText(node.textContent, 500))
			.filter((value) => value.length >= 2 && value.length <= 500), 20);
		const benefits = benefitTexts.map((text) => ({
			kind: /카드/.test(text) ? 'card' : /쿠폰/.test(text) ? 'coupon' : /캐시|적립/.test(text) ? 'cashback' : 'promotion',
			title: text.slice(0, 200), description: text.length > 200 ? text : '', source: 'product_page',
		}));
		const couponText = cleanText(quantity?.appliedCoupon?.title ?? quantity?.appliedCoupon?.description ?? quantity?.appliedCoupon, 300);
		if (couponText) benefits.push({ kind: 'coupon', title: couponText.slice(0, 200), description: couponText, source: 'quantity_info' });
		const cashBackText = cleanText(quantity?.cashBackSummary?.title ?? quantity?.cashBackSummary?.description ?? quantity?.cashBackSummary, 300);
		if (cashBackText) benefits.push({ kind: 'cashback', title: cashBackText.slice(0, 200), description: cashBackText, source: 'quantity_info' });
		const deliverySummary = cleanText(quantity?.delivery?.text ?? quantity?.delivery?.description ?? quantity?.delivery, 500) ||
			cleanText(document.querySelector('[class*="delivery"]')?.textContent, 500);
		const currentAmount = money(offers?.price) || money(quantity?.price) || money(quantity?.priceUnit) ||
			money(document.querySelector('[class*="price"] strong')?.textContent);
		const originalAmount = money(quantity?.originalPrice) || money(quantity?.priceList) || money(document.querySelector('del')?.textContent);
		const discountRate = Math.min(100, Math.round(numeric(quantity?.discountRate ?? document.querySelector('[class*="discountRate"]')?.textContent)));
		const itemId = request.item_id || new URL(location.href).searchParams.get('itemId') || '';
		const canonical = new URL('/vp/products/' + productId, location.origin);
		if (/^\d+$/.test(itemId)) canonical.searchParams.set('itemId', itemId);
		if (/^\d+$/.test(vendorItemId)) canonical.searchParams.set('vendorItemId', vendorItemId);
		const observed = ['name'];
		if (currentAmount || offers?.price === 0) observed.push('price.current_amount');
		if (originalAmount) observed.push('price.original_amount');
		if (discountRate) observed.push('price.discount_rate');
		if (numeric(ratingRoot?.ratingAverage ?? aggregate?.ratingValue)) observed.push('rating');
		if (numeric(reviewRoot?.reviewTotalCount ?? ratingRoot?.ratingCount ?? aggregate?.reviewCount)) observed.push('review_count');
		const rocket = /로켓|rocket/i.test(deliverySummary || bodyText);
		const freeShipping = /무료\s*배송/.test(deliverySummary || bodyText);
		const warnings = [];
		if (!quantityPayload) warnings.push('current quantity and promotion endpoint was unavailable');
		if (!reviewPayload) warnings.push('review endpoint was unavailable');
		if (!benefits.some((benefit) => benefit.kind === 'card')) warnings.push('no structured card benefit was observed for this item');
		return JSON.stringify({
			product: {
				product_id: productId, item_id: /^\d+$/.test(itemId) ? itemId : '', vendor_item_id: /^\d+$/.test(vendorItemId) ? vendorItemId : '',
				name, url: canonical.href, image_url: gallery[0] || '', current_amount: currentAmount,
				original_amount: originalAmount, discount_rate: discountRate,
				rating: Math.min(5, numeric(ratingRoot?.ratingAverage ?? aggregate?.ratingValue)),
				review_count: Math.round(numeric(reviewRoot?.reviewTotalCount ?? ratingRoot?.ratingCount ?? aggregate?.reviewCount)),
				rocket, free_shipping: freeShipping, coupon: benefits.some((benefit) => benefit.kind === 'coupon'),
				sponsored: false, observed_fields: observed,
			},
			selected_options: selectedOptions, description, specifications, gallery_images: gallery, detail_images: detailImages,
			delivery: { summary: deliverySummary, free_shipping: freeShipping, rocket }, benefits,
			rating: {
				average: Math.min(5, numeric(ratingRoot?.ratingAverage ?? aggregate?.ratingValue)),
				count: Math.round(numeric(reviewRoot?.reviewTotalCount ?? ratingRoot?.ratingCount ?? aggregate?.reviewCount)), distribution,
			},
			reviews,
			coverage: {
				source: 'coupang_product_document_and_read_endpoints',
				observed_fields: unique(['identity','name', selectedOptions.length && 'selected_options', description && 'description', gallery.length && 'gallery_images', detailImages.length && 'detail_images',
					currentAmount && 'price', deliverySummary && 'delivery', benefits.length && 'benefits', reviews.length && 'reviews'].filter(Boolean), 30),
				unavailable_fields: unique([!quantityPayload && 'quantity_info', !reviewPayload && 'reviews', !benefits.some((benefit) => benefit.kind === 'card') && 'card_benefit'].filter(Boolean), 20),
			}, warnings,
		});
	})()`
	var evaluated struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails,omitempty"`
	}
	evalCtx, cancel := context.WithTimeout(ctx, pageLoadTimeout)
	defer cancel()
	if err := page.Call(evalCtx, "Runtime.evaluate", map[string]any{
		"expression": expression, "awaitPromise": true, "returnByValue": true,
	}, &evaluated); err != nil || len(evaluated.ExceptionDetails) != 0 {
		return nil, ErrStructuredProductDataMissing
	}
	var encoded string
	if json.Unmarshal(evaluated.Result.Value, &encoded) != nil || len(encoded) == 0 || len(encoded) > maxProductDocumentBytes || !json.Valid([]byte(encoded)) {
		return nil, ErrStructuredProductDataMissing
	}
	return []byte(encoded), nil
}

func pressProductCartControl(ctx context.Context, page *cdpClient, request core.CartAddRequest) ([]byte, error) {
	encodedRequest, err := json.Marshal(request)
	if err != nil {
		return nil, ErrStructuredProductDataMissing
	}
	expression := `(async () => {
		const request = ` + string(encodedRequest) + `;
		const clean = (value) => (value ?? '').replace(/\s+/g, ' ').trim();
		const visible = (element) => {
			const style = getComputedStyle(element);
			const box = element.getBoundingClientRect();
			return style.visibility !== 'hidden' && style.display !== 'none' && box.width > 0 && box.height > 0;
		};
		const current = new URL(location.href);
		const selectedVendor = current.searchParams.get('vendorItemId') ||
			document.querySelector('input[name="vendorItemId"]')?.value ||
			document.querySelector('[data-vendor-item-id]')?.getAttribute('data-vendor-item-id') || '';
		if (selectedVendor && selectedVendor !== request.vendor_item_id) {
			return JSON.stringify({ attempted: false, reason: 'selected_item_mismatch' });
		}
		const quantityInput = document.querySelector('input[name="quantity"], input[class*="quantity"], input[type="number"]');
		if (quantityInput) {
			const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value')?.set;
			setter?.call(quantityInput, String(request.quantity));
			quantityInput.dispatchEvent(new Event('input', { bubbles: true }));
			quantityInput.dispatchEvent(new Event('change', { bubbles: true }));
		}
		const controls = Array.from(document.querySelectorAll('button, a')).filter(visible);
		const control = controls.find((element) => {
			const text = clean(element.textContent);
			return (text === '장바구니' || /장바구니\s*(담기|넣기|추가)/.test(text)) && !/구매|결제/.test(text) && !element.disabled;
		});
		if (!control) return JSON.stringify({ attempted: false, reason: 'cart_control_missing' });
		const badge = () => clean(document.querySelector('[class*="cart"] [class*="count"], [class*="cartCount"]')?.textContent);
		const beforeBadge = badge();
		control.click();
		let added = false;
		for (let attempt = 0; attempt < 30; attempt++) {
			await new Promise((resolve) => setTimeout(resolve, 200));
			const dialogs = Array.from(document.querySelectorAll('[role="dialog"], [class*="toast"], [class*="modal"], [class*="layer"]'))
				.filter(visible).map((element) => clean(element.textContent)).join(' ');
			const confirmation = /장바구니.{0,30}(담았|담겼|추가되|이동)|상품.{0,20}장바구니/.test(dialogs);
			const badgeChanged = beforeBadge && badge() && beforeBadge !== badge();
			if (confirmation || badgeChanged || location.hostname === 'cart.coupang.com') { added = true; break; }
		}
		return JSON.stringify({
			attempted: true, added, quantity: request.quantity,
			cart_url: 'https://cart.coupang.com/cartView.pang', source: 'coupang_product_page',
		});
	})()`
	var evaluated struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails,omitempty"`
	}
	evalCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	if err := page.Call(evalCtx, "Runtime.evaluate", map[string]any{
		"expression": expression, "awaitPromise": true, "returnByValue": true,
	}, &evaluated); err != nil || len(evaluated.ExceptionDetails) != 0 {
		return nil, ErrStructuredProductDataMissing
	}
	var encoded string
	if json.Unmarshal(evaluated.Result.Value, &encoded) != nil || !json.Valid([]byte(encoded)) {
		return nil, ErrStructuredProductDataMissing
	}
	var state struct {
		Attempted bool `json:"attempted"`
	}
	if json.Unmarshal([]byte(encoded), &state) != nil || !state.Attempted {
		return nil, ErrStructuredProductDataMissing
	}
	return []byte(encoded), nil
}

func waitForProductEnvelope(ctx context.Context, page *cdpClient, expression string, requireItems bool) ([]byte, error) {
	waitCtx, cancel := context.WithTimeout(ctx, pageLoadTimeout)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		var evaluated struct {
			Result struct {
				Value json.RawMessage `json:"value"`
			} `json:"result"`
			ExceptionDetails json.RawMessage `json:"exceptionDetails,omitempty"`
		}
		if err := page.Call(waitCtx, "Runtime.evaluate", map[string]any{"expression": expression, "returnByValue": true}, &evaluated); err == nil && len(evaluated.ExceptionDetails) == 0 {
			var encoded string
			if json.Unmarshal(evaluated.Result.Value, &encoded) == nil && len(encoded) <= maxProductDocumentBytes {
				var state struct {
					Href         string            `json:"href"`
					Ready        string            `json:"ready"`
					Status       int               `json:"status"`
					LoginForm    bool              `json:"login_form"`
					Challenge    bool              `json:"challenge"`
					AccessDenied bool              `json:"access_denied"`
					NoResults    bool              `json:"no_results"`
					Items        []json.RawMessage `json:"items"`
				}
				if json.Unmarshal([]byte(encoded), &state) == nil {
					if state.Status == http.StatusForbidden || state.AccessDenied {
						return nil, ErrBrowserAccessDenied
					}
					if isLoginURL(state.Href) || state.LoginForm {
						return nil, ErrAuthenticationRequired
					}
					if (!requireItems || len(state.Items) > 0 || state.NoResults) && json.Valid([]byte(encoded)) {
						return []byte(encoded), nil
					}
					if state.Ready == "complete" && (state.Status >= 400 || state.Challenge) {
						return nil, ErrStructuredProductDataMissing
					}
				}
			}
		}
		select {
		case <-waitCtx.Done():
			return nil, ErrStructuredProductDataMissing
		case <-ticker.C:
		}
	}
}

func waitForProductCategoryDocument(ctx context.Context, page *cdpClient) ([]byte, error) {
	waitCtx, cancel := context.WithTimeout(ctx, pageLoadTimeout)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	const expression = `(() => {
		const body = document.body?.innerText ?? '';
		const navigation = performance.getEntriesByType('navigation')[0];
		return JSON.stringify({
			href: location.href,
			ready: document.readyState,
			status: navigation?.responseStatus ?? 0,
			jsonLD: [...document.querySelectorAll('script[type="application/ld+json"]')]
				.map((script) => script.textContent ?? '').filter(Boolean),
			loginForm: document.querySelector('input[type="password"]') !== null,
			challenge: /captcha|보안문자|자동입력방지/i.test(body),
			accessDenied: /access denied|접근.{0,8}(거부|제한)|비정상.{0,8}접근/i.test(body)
		});
	})()`
	for {
		var evaluated struct {
			Result struct {
				Value json.RawMessage `json:"value"`
			} `json:"result"`
			ExceptionDetails json.RawMessage `json:"exceptionDetails,omitempty"`
		}
		if err := page.Call(waitCtx, "Runtime.evaluate", map[string]any{"expression": expression, "returnByValue": true}, &evaluated); err == nil && len(evaluated.ExceptionDetails) == 0 {
			var encoded string
			if json.Unmarshal(evaluated.Result.Value, &encoded) == nil {
				var state struct {
					Href         string   `json:"href"`
					Ready        string   `json:"ready"`
					Status       int      `json:"status"`
					JSONLD       []string `json:"jsonLD"`
					LoginForm    bool     `json:"loginForm"`
					Challenge    bool     `json:"challenge"`
					AccessDenied bool     `json:"accessDenied"`
				}
				if json.Unmarshal([]byte(encoded), &state) == nil {
					if state.Status == http.StatusForbidden || state.AccessDenied {
						return nil, ErrBrowserAccessDenied
					}
					if isLoginURL(state.Href) || state.LoginForm {
						return nil, ErrAuthenticationRequired
					}
					if validateProductCategoryTarget(state.Href) == nil && len(state.JSONLD) > 0 {
						documents := make([]json.RawMessage, 0, len(state.JSONLD))
						for _, raw := range state.JSONLD {
							if len(raw) == 0 || !json.Valid([]byte(raw)) {
								continue
							}
							documents = append(documents, json.RawMessage(raw))
						}
						if len(documents) > 0 {
							result, marshalErr := json.Marshal(struct {
								Documents []json.RawMessage `json:"json_ld"`
							}{Documents: documents})
							if marshalErr == nil && len(result) <= 2<<20 {
								return result, nil
							}
						}
					}
					if state.Ready == "complete" && (state.Status >= 400 || state.Challenge) {
						return nil, ErrStructuredCategoryDataMissing
					}
				}
			}
		}
		select {
		case <-waitCtx.Done():
			return nil, ErrStructuredCategoryDataMissing
		case <-ticker.C:
		}
	}
}

func fetchOrderModelDocument(ctx context.Context, page *cdpClient, targetURL string) ([]byte, error) {
	encodedTarget, err := json.Marshal(targetURL)
	if err != nil {
		return nil, ErrStructuredOrderDataMissing
	}
	expression := `(async () => {
		const response = await fetch(` + string(encodedTarget) + `, {
			method: 'GET',
			credentials: 'include',
			headers: {accept: 'application/json'}
		});
		return JSON.stringify({
			status: response.status,
			url: response.url,
			body: await response.text()
		});
	})()`
	var evaluated struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails,omitempty"`
	}
	if err := page.Call(ctx, "Runtime.evaluate", map[string]any{
		"expression": expression, "awaitPromise": true, "returnByValue": true,
	}, &evaluated); err != nil || len(evaluated.ExceptionDetails) != 0 {
		return nil, ErrStructuredOrderDataMissing
	}
	var encoded string
	if json.Unmarshal(evaluated.Result.Value, &encoded) != nil {
		return nil, ErrStructuredOrderDataMissing
	}
	var result struct {
		Status int    `json:"status"`
		URL    string `json:"url"`
		Body   string `json:"body"`
	}
	if json.Unmarshal([]byte(encoded), &result) != nil {
		return nil, ErrStructuredOrderDataMissing
	}
	if result.Status == http.StatusForbidden {
		return nil, ErrBrowserAccessDenied
	}
	if isLoginURL(result.URL) || result.Status == http.StatusUnauthorized {
		return nil, ErrAuthenticationRequired
	}
	parsed, parseErr := url.Parse(result.URL)
	if parseErr != nil || parsed.Scheme != "https" || parsed.Host != "mc.coupang.com" || parsed.Path != "/ssr/api/myorders/model" || result.Status < 200 || result.Status >= 300 {
		return nil, ErrStructuredOrderDataMissing
	}
	if len(result.Body) == 0 || len(result.Body) > maxCDPMessageSize/2 || !json.Valid([]byte(result.Body)) {
		return nil, ErrStructuredOrderDataMissing
	}
	return []byte(result.Body), nil
}

func (s *chromeSession) persistBrowserSession(ctx context.Context) error {
	if s.sessions == nil || s.browser == nil {
		return errors.New("browser session persistence unavailable")
	}
	return s.sessions.Capture(ctx, s.browser)
}

func (s *chromeSession) Close() error {
	var closeErr error
	s.closeOnce.Do(func() {
		if s.browser != nil {
			closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = s.browser.Call(closeCtx, "Browser.close", struct{}{}, nil)
			cancel()
			_ = s.browser.Close()
		}
		if s.command != nil && s.command.Process != nil {
			done := make(chan error, 1)
			go func() { done <- s.command.Wait() }()
			select {
			case err := <-done:
				if err != nil {
					var exitErr *exec.ExitError
					if !errors.As(err, &exitErr) {
						closeErr = fmt.Errorf("wait for browser shutdown: %w", err)
					}
				}
			case <-time.After(3 * time.Second):
				_ = s.command.Process.Kill()
				<-done
			}
		}
		s.httpClient.CloseIdleConnections()
	})
	return closeErr
}

func (s *chromeSession) waitForVersion(ctx context.Context) (versionMetadata, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/json/version", nil)
		response, err := s.httpClient.Do(request)
		if err == nil {
			var metadata versionMetadata
			decodeErr := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&metadata)
			response.Body.Close()
			if decodeErr == nil && metadata.WebSocketDebuggerURL != "" {
				return metadata, nil
			}
		}
		select {
		case <-ctx.Done():
			return versionMetadata{}, errors.New("local browser control channel did not start")
		case <-ticker.C:
		}
	}
}

func (s *chromeSession) waitForTarget(ctx context.Context, targetID string) (targetMetadata, error) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.baseURL+"/json/list", nil)
		response, err := s.httpClient.Do(request)
		if err == nil {
			var targets []targetMetadata
			decodeErr := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&targets)
			response.Body.Close()
			if decodeErr == nil {
				for _, target := range targets {
					if target.ID == targetID && target.WebSocketDebuggerURL != "" {
						return target, nil
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return targetMetadata{}, errors.New("read-only browser page did not start")
		case <-ticker.C:
		}
	}
}

func waitForOrderDocument(ctx context.Context, page *cdpClient) ([]byte, error) {
	waitCtx, cancel := context.WithTimeout(ctx, pageLoadTimeout)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	const expression = `(() => {
		const body = document.body?.innerText ?? '';
		const navigation = performance.getEntriesByType('navigation')[0];
		const structuredModel = location.hostname === 'mc.coupang.com' &&
			location.pathname === '/ssr/api/myorders/model' && body.trimStart().startsWith('{')
			? body : null;
		return JSON.stringify({
			href: location.href,
			ready: document.readyState,
			nextData: document.getElementById('__NEXT_DATA__')?.textContent ?? null,
			structuredModel,
			status: navigation?.responseStatus ?? 0,
			scriptPresent: document.getElementById('__NEXT_DATA__') !== null,
			loginForm: document.querySelector('input[type="password"]') !== null,
			challenge: /captcha|보안문자|자동입력방지/i.test(body),
			accessDenied: /access denied|접근.{0,8}(거부|제한)|비정상.{0,8}접근/i.test(body)
		});
	})()`
	lastState := "runtime_unavailable"
	for {
		var evaluated struct {
			Result struct {
				Value json.RawMessage `json:"value"`
			} `json:"result"`
			ExceptionDetails json.RawMessage `json:"exceptionDetails,omitempty"`
		}
		err := page.Call(waitCtx, "Runtime.evaluate", map[string]any{
			"expression": expression, "returnByValue": true,
		}, &evaluated)
		if err == nil && len(evaluated.ExceptionDetails) == 0 {
			var encoded string
			if json.Unmarshal(evaluated.Result.Value, &encoded) == nil {
				var state struct {
					Href            string  `json:"href"`
					Ready           string  `json:"ready"`
					NextData        *string `json:"nextData"`
					StructuredModel *string `json:"structuredModel"`
					Status          int     `json:"status"`
					ScriptPresent   bool    `json:"scriptPresent"`
					LoginForm       bool    `json:"loginForm"`
					Challenge       bool    `json:"challenge"`
					AccessDenied    bool    `json:"accessDenied"`
				}
				if json.Unmarshal([]byte(encoded), &state) == nil {
					lastState = classifyDocumentState(state.Href, state.Status, state.ScriptPresent, state.LoginForm, state.Challenge, state.AccessDenied)
					if state.Status == http.StatusForbidden {
						return nil, ErrBrowserAccessDenied
					}
					if isLoginURL(state.Href) {
						return nil, ErrAuthenticationRequired
					}
					if state.NextData != nil && *state.NextData != "" {
						return []byte(*state.NextData), nil
					}
					if state.StructuredModel != nil && json.Valid([]byte(*state.StructuredModel)) {
						return []byte(*state.StructuredModel), nil
					}
					if state.Ready == "complete" {
						// Hydration can append __NEXT_DATA__ shortly after the load event.
					}
				}
			}
		}
		select {
		case <-waitCtx.Done():
			return nil, fmt.Errorf("%w: %s", ErrStructuredOrderDataMissing, lastState)
		case <-ticker.C:
		}
	}
}

func waitForMembershipData(ctx context.Context, page *cdpClient) ([]byte, error) {
	waitCtx, cancel := context.WithTimeout(ctx, pageLoadTimeout)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	const expression = `(() => {
		const body = document.body?.innerText ?? '';
		const navigation = performance.getEntriesByType('navigation')[0];
		let data = null;
		try {
			const parsed = JSON.parse(document.getElementById('__NEXT_DATA__')?.textContent ?? 'null');
			data = parsed?.props?.pageProps?.data ?? parsed?.query?.data ?? null;
		} catch {}
		return JSON.stringify({
			href: location.href,
			ready: document.readyState,
			status: navigation?.responseStatus ?? 0,
			data,
			loginForm: document.querySelector('input[type="password"]') !== null,
			challenge: /captcha|보안문자|자동입력방지/i.test(body),
			accessDenied: /access denied|접근.{0,8}(거부|제한)|비정상.{0,8}접근/i.test(body)
		});
	})()`
	for {
		encoded, err := evaluateBrowserString(waitCtx, page, expression, false)
		if err == nil {
			var state struct {
				Href         string          `json:"href"`
				Ready        string          `json:"ready"`
				Status       int             `json:"status"`
				Data         json.RawMessage `json:"data"`
				LoginForm    bool            `json:"loginForm"`
				Challenge    bool            `json:"challenge"`
				AccessDenied bool            `json:"accessDenied"`
			}
			if json.Unmarshal([]byte(encoded), &state) == nil {
				if state.Status == http.StatusForbidden || state.AccessDenied || state.Challenge {
					return nil, ErrBrowserAccessDenied
				}
				if isLoginURL(state.Href) || state.LoginForm || state.Status == http.StatusUnauthorized {
					return nil, ErrAuthenticationRequired
				}
				if len(state.Data) > 0 && string(state.Data) != "null" {
					var member struct {
						LoyaltyMemberInfo struct {
							MembershipStatus string `json:"membershipStatus"`
						} `json:"loyaltyMemberInfo"`
					}
					if json.Unmarshal(state.Data, &member) == nil && member.LoyaltyMemberInfo.MembershipStatus != "" {
						return json.Marshal(struct {
							Data json.RawMessage `json:"data"`
						}{Data: state.Data})
					}
				}
			}
		}
		select {
		case <-waitCtx.Done():
			return nil, ErrStructuredAccountBenefitsDataMissing
		case <-ticker.C:
		}
	}
}

func waitForReceiptPageReady(ctx context.Context, page *cdpClient) error {
	waitCtx, cancel := context.WithTimeout(ctx, pageLoadTimeout)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	const expression = `(() => {
		const body = document.body?.innerText ?? '';
		const navigation = performance.getEntriesByType('navigation')[0];
		let paymentReceipt = null;
		try {
			paymentReceipt = JSON.parse(document.getElementById('__NEXT_DATA__')?.textContent ?? 'null')
				?.props?.pageProps?.domains?.paymentReceipt ?? null;
		} catch {}
		return JSON.stringify({
			href: location.href,
			ready: document.readyState,
			status: navigation?.responseStatus ?? 0,
			paymentReceipt,
			loginForm: document.querySelector('input[type="password"]') !== null,
			challenge: /captcha|보안문자|자동입력방지/i.test(body),
			accessDenied: /access denied|접근.{0,8}(거부|제한)|비정상.{0,8}접근/i.test(body)
		});
	})()`
	for {
		encoded, err := evaluateBrowserString(waitCtx, page, expression, false)
		if err == nil {
			var state struct {
				Href           string          `json:"href"`
				Ready          string          `json:"ready"`
				Status         int             `json:"status"`
				PaymentReceipt json.RawMessage `json:"paymentReceipt"`
				LoginForm      bool            `json:"loginForm"`
				Challenge      bool            `json:"challenge"`
				AccessDenied   bool            `json:"accessDenied"`
			}
			if json.Unmarshal([]byte(encoded), &state) == nil {
				if state.Status == http.StatusForbidden || state.AccessDenied || state.Challenge {
					return ErrBrowserAccessDenied
				}
				if isLoginURL(state.Href) || state.LoginForm || state.Status == http.StatusUnauthorized {
					return ErrAuthenticationRequired
				}
				parsed, parseErr := url.Parse(state.Href)
				if parseErr == nil && parsed.Scheme == "https" && parsed.Host == "mc.coupang.com" && parsed.Path == "/ssr/desktop/payment-receipt" &&
					len(state.PaymentReceipt) > 0 && string(state.PaymentReceipt) != "null" {
					return nil
				}
				if state.Ready == "complete" && state.Status >= 400 {
					return ErrStructuredReceiptDataMissing
				}
			}
		}
		select {
		case <-waitCtx.Done():
			return ErrStructuredReceiptDataMissing
		case <-ticker.C:
		}
	}
}

func waitOnCashData(ctx context.Context, page *cdpClient, maxPages int) ([]byte, error) {
	waitCtx, cancel := context.WithTimeout(ctx, pageLoadTimeout)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	expression := fmt.Sprintf(`(async () => {
		const body = document.body?.innerText ?? '';
		const navigation = performance.getEntriesByType('navigation')[0];
		const resourceURLs = [...new Set(performance.getEntriesByType('resource').map((entry) => entry.name))]
			.filter((value) => {
				try {
					const parsed = new URL(value);
					return parsed.origin === 'https://cash.coupang.com' && parsed.pathname.startsWith('/api/cash/');
				} catch { return false; }
			});
		let summary = null;
		let transactionURL = '';
		for (const resourceURL of resourceURLs) {
			try {
				const response = await fetch(resourceURL, { credentials: 'include' });
				if (!response.ok) continue;
				const parsed = await response.json();
				if (parsed?.content?.expectedWowCardAccumulationAmount !== undefined) summary = parsed;
				if (Array.isArray(parsed?.content?.list) && parsed?.content?.currentPageNumber !== undefined) transactionURL = resourceURL;
			} catch {}
		}
		const pages = [];
		if (transactionURL) {
			for (let pageNumber = 1; pageNumber <= %d; pageNumber++) {
				try {
					const url = new URL(transactionURL);
					url.searchParams.set('page', String(pageNumber));
					const response = await fetch(url.href, { credentials: 'include' });
					if (!response.ok) break;
					const parsed = await response.json();
					if (!Array.isArray(parsed?.content?.list)) break;
					pages.push(parsed);
					if (!parsed.content.nextPageExist) break;
				} catch { break; }
			}
		}
		return JSON.stringify({
			href: location.href,
			ready: document.readyState,
			status: navigation?.responseStatus ?? 0,
			summary,
			pages,
			loginForm: document.querySelector('input[type="password"]') !== null,
			challenge: /captcha|보안문자|자동입력방지/i.test(body),
			accessDenied: /access denied|접근.{0,8}(거부|제한)|비정상.{0,8}접근/i.test(body)
		});
	})()`, maxPages)
	for {
		encoded, err := evaluateBrowserString(waitCtx, page, expression, true)
		if err == nil {
			var state struct {
				Href         string            `json:"href"`
				Status       int               `json:"status"`
				Summary      json.RawMessage   `json:"summary"`
				Pages        []json.RawMessage `json:"pages"`
				LoginForm    bool              `json:"loginForm"`
				Challenge    bool              `json:"challenge"`
				AccessDenied bool              `json:"accessDenied"`
			}
			if json.Unmarshal([]byte(encoded), &state) == nil {
				if state.Status == http.StatusForbidden || state.AccessDenied || state.Challenge {
					return nil, ErrBrowserAccessDenied
				}
				if isLoginURL(state.Href) || state.LoginForm || state.Status == http.StatusUnauthorized {
					return nil, ErrAuthenticationRequired
				}
				if len(state.Pages) > 0 {
					return json.Marshal(struct {
						Summary json.RawMessage   `json:"summary,omitempty"`
						Pages   []json.RawMessage `json:"pages"`
					}{Summary: state.Summary, Pages: state.Pages})
				}
			}
		}
		select {
		case <-waitCtx.Done():
			return nil, ErrStructuredAccountBenefitsDataMissing
		case <-ticker.C:
		}
	}
}

func evaluateBrowserString(ctx context.Context, page *cdpClient, expression string, awaitPromise bool) (string, error) {
	var evaluated struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails,omitempty"`
	}
	if err := page.Call(ctx, "Runtime.evaluate", map[string]any{
		"expression": expression, "returnByValue": true, "awaitPromise": awaitPromise,
	}, &evaluated); err != nil || len(evaluated.ExceptionDetails) > 0 {
		return "", ErrStructuredAccountBenefitsDataMissing
	}
	var encoded string
	if json.Unmarshal(evaluated.Result.Value, &encoded) != nil {
		return "", ErrStructuredAccountBenefitsDataMissing
	}
	return encoded, nil
}

func classifyNavigation(target string) string {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Host == "" {
		return "runtime_unavailable"
	}
	switch {
	case parsed.Host == "login.coupang.com":
		return "login"
	case parsed.Host == "mc.coupang.com" && parsed.Path == "/ssr/desktop/order/list":
		return "expected_order"
	case parsed.Host == "mc.coupang.com" && parsed.Path == "/ssr/api/myorders/model":
		return "expected_order_model"
	case parsed.Host == "mc.coupang.com" || parsed.Host == "www.coupang.com":
		return "other_coupang"
	default:
		return "external"
	}
}

func classifyDocumentState(target string, status int, scriptPresent, loginForm, challenge, accessDenied bool) string {
	location := classifyNavigation(target)
	if location != "expected_order" && location != "expected_order_model" {
		return location
	}
	switch {
	case status >= 400:
		return fmt.Sprintf("expected_order_http_%d", status)
	case challenge:
		return "expected_order_challenge"
	case accessDenied:
		return "expected_order_access_denied"
	case loginForm:
		return "expected_order_login_form"
	case scriptPresent:
		return "expected_order_empty_next_data"
	default:
		return fmt.Sprintf("expected_order_http_%d_no_next_data", status)
	}
}

func (c *cdpClient) Call(ctx context.Context, method string, params any, result any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextID++
	id := c.nextID
	if err := wsjson.Write(ctx, c.connection, cdpRequest{ID: id, Method: method, Params: params}); err != nil {
		return err
	}
	for {
		var response cdpResponse
		if err := wsjson.Read(ctx, c.connection, &response); err != nil {
			return err
		}
		if response.ID != id {
			continue
		}
		if response.Error != nil {
			return fmt.Errorf("browser protocol command failed: code %d", response.Error.Code)
		}
		if result != nil && len(response.Result) > 0 {
			if err := json.Unmarshal(response.Result, result); err != nil {
				return fmt.Errorf("decode browser protocol response: %w", err)
			}
		}
		return nil
	}
}

func (c *cdpClient) Close() error {
	return c.connection.Close(websocket.StatusNormalClosure, "")
}

func dialCDP(ctx context.Context, endpoint string) (*cdpClient, error) {
	connection, _, err := websocket.Dial(ctx, endpoint, &websocket.DialOptions{HTTPClient: &http.Client{Transport: &http.Transport{Proxy: nil}}})
	if err != nil {
		return nil, err
	}
	connection.SetReadLimit(maxCDPMessageSize)
	return &cdpClient{connection: connection}, nil
}

func availableLoopbackPort() (int, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func validateOrderTarget(target string) error {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "mc.coupang.com" {
		return errors.New("blocked browser navigation target")
	}
	query := parsed.Query()
	switch parsed.Path {
	case "/ssr/desktop/order/list":
		for key := range query {
			if key != "pageIndex" && key != "periodYear" {
				return errors.New("blocked browser navigation query")
			}
		}
	case "/ssr/api/myorders/model":
		if len(query) != 3 || query.Get("size") != "5" {
			return errors.New("blocked browser navigation query")
		}
		page, pageErr := strconv.Atoi(query.Get("pageIndex"))
		year, yearErr := strconv.Atoi(query.Get("requestYear"))
		if pageErr != nil || yearErr != nil || page < 0 || page > 1000 || year < 2000 || year > 2100 {
			return errors.New("blocked browser navigation query")
		}
	default:
		return errors.New("blocked browser navigation target")
	}
	return nil
}

func validateProductCategoryTarget(target string) error {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "www.coupang.com" || !strings.HasPrefix(parsed.Path, "/vp/products/") {
		return errors.New("blocked product category target")
	}
	productID := strings.TrimPrefix(parsed.Path, "/vp/products/")
	if strings.Contains(productID, "/") || !numericURLValue(productID) {
		return errors.New("blocked product category target")
	}
	query := parsed.Query()
	if len(query) != 1 || !numericURLValue(query.Get("vendorItemId")) {
		return errors.New("blocked product category query")
	}
	return nil
}

func validateAccountBenefitsTarget(target string) error {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "https" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("blocked account benefits target")
	}
	if (parsed.Host == "loyalty.coupang.com" && parsed.Path == "/loyalty/management/home") ||
		(parsed.Host == "cash.coupang.com" && parsed.Path == "/coupang-cash/home") {
		return nil
	}
	return errors.New("blocked account benefits target")
}

func validateProductSearchTarget(target string) error {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "www.coupang.com" {
		return errors.New("blocked product search target")
	}
	query := parsed.Query()
	searchPath := parsed.Path == "/np/search"
	categoryID := strings.TrimPrefix(parsed.Path, "/np/categories/")
	categoryPath := categoryID != parsed.Path && numericURLValue(categoryID)
	if !searchPath && !categoryPath {
		return errors.New("blocked product search target")
	}
	for key := range query {
		if key != "q" && key != "sorter" {
			return errors.New("blocked product search query")
		}
	}
	search := strings.TrimSpace(query.Get("q"))
	if (searchPath && (search == "" || len([]rune(search)) > 200)) || (categoryPath && search != "") {
		return errors.New("blocked product search query")
	}
	if sorter := query.Get("sorter"); sorter != "" {
		switch sorter {
		case "scoreDesc", "saleCountDesc", "latestAsc", "salePriceAsc", "salePriceDesc":
		default:
			return errors.New("blocked product search query")
		}
	}
	return nil
}

func validateProductInspectionTarget(target string, request core.ProductInspectRequest) error {
	if err := request.Validate(); err != nil {
		return errors.New("blocked product inspection request")
	}
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "www.coupang.com" || parsed.Path != "/vp/products/"+request.ProductID {
		return errors.New("blocked product inspection target")
	}
	query := parsed.Query()
	for key := range query {
		if key != "itemId" && key != "vendorItemId" {
			return errors.New("blocked product inspection query")
		}
	}
	if query.Get("itemId") != request.ItemID || query.Get("vendorItemId") != request.VendorItemID {
		return errors.New("blocked product inspection query")
	}
	return nil
}

func validateProductCartTarget(target string, request core.CartAddRequest) error {
	if err := request.Validate(); err != nil {
		return errors.New("blocked product cart request")
	}
	inspection := core.ProductInspectRequest{ProductID: request.ProductID, ItemID: request.ItemID, VendorItemID: request.VendorItemID}
	return validateProductInspectionTarget(target, inspection)
}

func numericURLValue(value string) bool {
	if value == "" || len(value) > 24 {
		return false
	}
	_, err := strconv.ParseUint(value, 10, 64)
	return err == nil
}

func isOrderModelURL(target string) bool {
	parsed, err := url.Parse(target)
	return err == nil && parsed.Host == "mc.coupang.com" && parsed.Path == "/ssr/api/myorders/model"
}

func isLoginURL(target string) bool {
	parsed, err := url.Parse(target)
	return err == nil && parsed.Host == "login.coupang.com"
}
