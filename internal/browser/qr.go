package browser

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

const qrApprovalTimeout = 3 * time.Minute
const qrLinkDiscoveryTimeout = 10 * time.Second

var ErrQRExpired = errors.New("QR login expired")
var ErrQRLoginTimedOut = errors.New("QR login timed out")
var ErrQRUnexpectedDestination = errors.New("QR login reached an unexpected destination")
var ErrQRLinkUnavailable = errors.New("QR login link unavailable")

type qrPagePhase string

const (
	qrPhaseWaiting    qrPagePhase = "waiting"
	qrPhaseReady      qrPagePhase = "ready"
	qrPhaseExpired    qrPagePhase = "expired"
	qrPhaseApproved   qrPagePhase = "approved"
	qrPhaseUnexpected qrPagePhase = "unexpected"
	qrPhaseDenied     qrPagePhase = "denied"
)

type qrPageState struct {
	Href                string `json:"href"`
	Status              int    `json:"status"`
	DocumentReady       bool   `json:"documentReady"`
	StructuredOrderData bool   `json:"structuredOrderData"`
	QRReady             bool   `json:"qrReady"`
	QRExpired           bool   `json:"qrExpired"`
	AccessDenied        bool   `json:"accessDenied"`
}

func classifyQRPage(state qrPageState) qrPagePhase {
	if state.Status == http.StatusForbidden || state.AccessDenied {
		return qrPhaseDenied
	}
	parsed, err := url.Parse(state.Href)
	if err != nil || parsed.Host == "" {
		return qrPhaseWaiting
	}
	if parsed.Host == "mc.coupang.com" && parsed.Path == "/ssr/desktop/order/list" {
		if state.DocumentReady && state.StructuredOrderData {
			return qrPhaseApproved
		}
		return qrPhaseWaiting
	}
	if parsed.Host != "login.coupang.com" {
		return qrPhaseUnexpected
	}
	if state.QRExpired {
		return qrPhaseExpired
	}
	if state.QRReady {
		return qrPhaseReady
	}
	return qrPhaseWaiting
}

func presentQRLogin(ctx context.Context, executable, profileDir, targetURL, outputPath string, presentLink core.QRLinkPresenter) error {
	if err := validateOrderTarget(targetURL); err != nil {
		return err
	}
	source, err := startChromeSession(ctx, executable, profileDir, false)
	if err != nil {
		return err
	}
	session, ok := source.(*chromeSession)
	if !ok {
		_ = source.Close()
		return errors.New("QR browser session unavailable")
	}
	defer session.Close()
	if err := session.presentQR(ctx, targetURL, outputPath, presentLink); err != nil {
		return err
	}
	return session.persistBrowserSession(ctx)
}

func (s *chromeSession) presentQR(ctx context.Context, targetURL, outputPath string, presentLink core.QRLinkPresenter) error {
	var created struct {
		TargetID string `json:"targetId"`
	}
	if err := s.browser.Call(ctx, "Target.createTarget", map[string]any{"url": "about:blank"}, &created); err != nil {
		return fmt.Errorf("create QR browser page: %w", err)
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.browser.Call(closeCtx, "Target.closeTarget", map[string]any{"targetId": created.TargetID}, nil)
	}()

	target, err := s.waitForTarget(ctx, created.TargetID)
	if err != nil {
		return err
	}
	page, err := dialCDP(ctx, target.WebSocketDebuggerURL)
	if err != nil {
		return fmt.Errorf("connect QR browser page: %w", err)
	}
	defer page.Close()
	if err := page.Call(ctx, "Page.enable", struct{}{}, nil); err != nil {
		return fmt.Errorf("enable QR browser page: %w", err)
	}
	if err := page.Call(ctx, "Page.navigate", map[string]any{"url": targetURL}, nil); err != nil {
		return fmt.Errorf("navigate QR browser page: %w", err)
	}

	readyCtx, cancelReady := context.WithTimeout(ctx, pageLoadTimeout)
	defer cancelReady()
	readyTicker := time.NewTicker(200 * time.Millisecond)
	defer readyTicker.Stop()
	for {
		state, err := readQRPageState(readyCtx, page, true)
		if err == nil {
			switch classifyQRPage(state) {
			case qrPhaseApproved:
				return nil
			case qrPhaseReady:
				if presentLink != nil {
					if err := discoverAndPresentQRLink(ctx, page, presentLink); err != nil {
						return err
					}
				}
				if outputPath != "" {
					if err := captureQRPage(readyCtx, page, outputPath); err != nil {
						return err
					}
					defer os.Remove(outputPath)
				}
				return waitForQRApproval(ctx, page)
			case qrPhaseDenied:
				return ErrBrowserAccessDenied
			case qrPhaseUnexpected:
				return ErrQRUnexpectedDestination
			}
		}
		select {
		case <-readyCtx.Done():
			return ErrQRLoginTimedOut
		case <-readyTicker.C:
		}
	}
}

func discoverAndPresentQRLink(ctx context.Context, page qrCaptureCaller, presenter core.QRLinkPresenter) error {
	discoveryCtx, cancel := context.WithTimeout(ctx, qrLinkDiscoveryTimeout)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		link, err := readQRLoginLink(discoveryCtx, page)
		if err == nil {
			if err := presenter(discoveryCtx, link); err != nil {
				return ErrQRLinkUnavailable
			}
			return nil
		}
		select {
		case <-discoveryCtx.Done():
			return ErrQRLinkUnavailable
		case <-ticker.C:
		}
	}
}

func readQRLoginLink(ctx context.Context, page qrCaptureCaller) (core.QRLoginLink, error) {
	const expression = `(() => (async () => {
		if (typeof BarcodeDetector !== 'function') return JSON.stringify({});
		const visible = (element) => {
			const style = getComputedStyle(element);
			const rect = element.getBoundingClientRect();
			return style.display !== 'none' && style.visibility !== 'hidden' && Number(style.opacity) > 0 && rect.width > 0 && rect.height > 0;
		};
		const detector = new BarcodeDetector({formats: ['qr_code']});
		const candidates = [...document.querySelectorAll('img,canvas,video')].filter((element) => {
			const rect = element.getBoundingClientRect();
			return visible(element) && rect.width >= 120 && rect.height >= 120;
		});
		let rawValue = '';
		for (const candidate of candidates) {
			try {
				const codes = await detector.detect(candidate);
				const value = codes.find((code) => typeof code.rawValue === 'string' && code.rawValue.length > 0)?.rawValue;
				if (value) { rawValue = value; break; }
			} catch {}
		}
		const approvalCode = [...document.querySelectorAll('body *')]
			.filter((element) => element.children.length === 0 && visible(element))
			.map((element) => element.textContent?.trim() ?? '')
			.find((value) => /^\d{2}$/.test(value)) ?? '';
		return JSON.stringify({url: rawValue, approvalCode});
	})())()`
	var evaluated struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails,omitempty"`
	}
	if err := page.Call(ctx, "Runtime.evaluate", map[string]any{
		"expression": expression, "awaitPromise": true, "returnByValue": true,
	}, &evaluated); err != nil || len(evaluated.ExceptionDetails) != 0 {
		return core.QRLoginLink{}, ErrQRLinkUnavailable
	}
	var encoded string
	if err := json.Unmarshal(evaluated.Result.Value, &encoded); err != nil {
		return core.QRLoginLink{}, ErrQRLinkUnavailable
	}
	var decoded struct {
		URL          string `json:"url"`
		ApprovalCode string `json:"approvalCode"`
	}
	if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
		return core.QRLoginLink{}, ErrQRLinkUnavailable
	}
	link := core.QRLoginLink{URL: decoded.URL, ApprovalCode: decoded.ApprovalCode}
	if !validQRLoginLink(link) {
		return core.QRLoginLink{}, ErrQRLinkUnavailable
	}
	return link, nil
}

func validQRLoginLink(link core.QRLoginLink) bool {
	if len(link.ApprovalCode) != 2 || link.ApprovalCode[0] < '0' || link.ApprovalCode[0] > '9' || link.ApprovalCode[1] < '0' || link.ApprovalCode[1] > '9' {
		return false
	}
	parsed, err := url.Parse(link.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Port() != "" || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	if directQRBindURL(parsed) {
		return true
	}
	if parsed.Hostname() != "applink.coupang.com" || parsed.Path != "/open" {
		return false
	}
	for _, values := range parsed.Query() {
		for _, value := range values {
			nested, err := url.Parse(value)
			if err == nil && directQRBindURL(nested) {
				return true
			}
		}
	}
	return false
}

func directQRBindURL(parsed *url.URL) bool {
	return parsed != nil && parsed.Scheme == "https" && parsed.Hostname() == "login.coupang.com" && parsed.Port() == "" && parsed.User == nil && parsed.Fragment == "" && parsed.Path == "/login/m/qrcode/bind.pang" && parsed.Query().Get("qrCode") != ""
}

func waitForQRApproval(ctx context.Context, page *cdpClient) error {
	waitCtx, cancel := context.WithTimeout(ctx, qrApprovalTimeout)
	defer cancel()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		state, err := readQRPageState(waitCtx, page, false)
		if err == nil {
			switch classifyQRPage(state) {
			case qrPhaseApproved:
				return nil
			case qrPhaseExpired:
				return ErrQRExpired
			case qrPhaseDenied:
				return ErrBrowserAccessDenied
			case qrPhaseUnexpected:
				return ErrQRUnexpectedDestination
			}
		}
		select {
		case <-waitCtx.Done():
			return ErrQRLoginTimedOut
		case <-ticker.C:
		}
	}
}

func readQRPageState(ctx context.Context, page *cdpClient, activate bool) (qrPageState, error) {
	expression := fmt.Sprintf(`(() => {
		const body = document.body?.innerText ?? '';
		if (%t) {
			const candidates = [...document.querySelectorAll('a,button,[role="tab"],li')];
			const tab = candidates.find((element) => element.textContent?.trim() === 'QR코드 로그인');
			if (tab instanceof HTMLElement) tab.click();
		}
		const navigation = performance.getEntriesByType('navigation')[0];
		return JSON.stringify({
			href: location.href,
			status: navigation?.responseStatus ?? 0,
			documentReady: document.readyState === 'complete',
			structuredOrderData: (document.getElementById('__NEXT_DATA__')?.textContent?.length ?? 0) > 0,
			qrReady: /휴대폰 카메라로 QR코드를 스캔|남은시간/.test(body),
			qrExpired: /시간.{0,8}만료|QR.{0,8}만료|새로고침/.test(body),
			accessDenied: /access denied|접근.{0,8}(거부|제한)|비정상.{0,8}접근/i.test(body)
		});
	})()`, activate)
	var evaluated struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails,omitempty"`
	}
	if err := page.Call(ctx, "Runtime.evaluate", map[string]any{"expression": expression, "returnByValue": true}, &evaluated); err != nil {
		return qrPageState{}, err
	}
	if len(evaluated.ExceptionDetails) != 0 {
		return qrPageState{}, errors.New("inspect QR page")
	}
	var encoded string
	if err := json.Unmarshal(evaluated.Result.Value, &encoded); err != nil {
		return qrPageState{}, errors.New("decode QR page state")
	}
	var state qrPageState
	if err := json.Unmarshal([]byte(encoded), &state); err != nil {
		return qrPageState{}, errors.New("decode QR page state")
	}
	return state, nil
}

type qrCaptureCaller interface {
	Call(context.Context, string, any, any) error
}

type qrScreenshotClip struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	Scale  float64 `json:"scale"`
}

func captureQRPage(ctx context.Context, page qrCaptureCaller, outputPath string) error {
	clip, err := locateQRCodeClip(ctx, page)
	if err != nil {
		return err
	}
	var screenshot struct {
		Data string `json:"data"`
	}
	if err := page.Call(ctx, "Page.captureScreenshot", map[string]any{
		"format": "png", "fromSurface": true, "captureBeyondViewport": true,
		"clip": map[string]any{
			"x": clip.X, "y": clip.Y, "width": clip.Width, "height": clip.Height, "scale": clip.Scale,
		},
	}, &screenshot); err != nil {
		return fmt.Errorf("capture QR page: %w", err)
	}
	data, err := base64.StdEncoding.DecodeString(screenshot.Data)
	if err != nil || len(data) == 0 || len(data) > maxCDPMessageSize {
		return errors.New("decode QR page image")
	}
	if err := writeSensitiveQRFile(outputPath, data); err != nil {
		return fmt.Errorf("write private QR image: %w", err)
	}
	return nil
}

func locateQRCodeClip(ctx context.Context, page qrCaptureCaller) (qrScreenshotClip, error) {
	const expression = `(() => {
		const visible = (element) => {
			const style = getComputedStyle(element);
			const rect = element.getBoundingClientRect();
			return style.display !== 'none' && style.visibility !== 'hidden' && Number(style.opacity) > 0 && rect.width > 0 && rect.height > 0;
		};
		const candidates = [...document.querySelectorAll('img,canvas,svg,[class*="qr" i]')]
			.filter(visible)
			.map((element) => ({element, rect: element.getBoundingClientRect()}))
			.filter(({rect}) => rect.width >= 120 && rect.height >= 120 && Math.abs(rect.width - rect.height) <= Math.max(rect.width, rect.height) * 0.15)
			.sort((left, right) => (left.rect.width * left.rect.height) - (right.rect.width * right.rect.height));
		if (!candidates.length) return JSON.stringify({});
		const rect = candidates[0].rect;
		const padding = 10;
		const x = Math.max(0, rect.left - padding);
		const y = Math.max(0, rect.top - padding);
		return JSON.stringify({
			x,
			y,
			width: rect.width + (rect.left >= padding ? padding * 2 : padding),
			height: rect.height + (rect.top >= padding ? padding * 2 : padding),
			scale: 2
		});
	})()`
	var evaluated struct {
		Result struct {
			Value json.RawMessage `json:"value"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails,omitempty"`
	}
	if err := page.Call(ctx, "Runtime.evaluate", map[string]any{"expression": expression, "returnByValue": true}, &evaluated); err != nil {
		return qrScreenshotClip{}, fmt.Errorf("locate QR code: %w", err)
	}
	if len(evaluated.ExceptionDetails) != 0 {
		return qrScreenshotClip{}, errors.New("locate QR code")
	}
	var encoded string
	if err := json.Unmarshal(evaluated.Result.Value, &encoded); err != nil {
		return qrScreenshotClip{}, errors.New("decode QR code location")
	}
	var clip qrScreenshotClip
	if err := json.Unmarshal([]byte(encoded), &clip); err != nil || clip.Width < 120 || clip.Height < 120 || clip.Scale < 1 {
		return qrScreenshotClip{}, errors.New("QR code location unavailable")
	}
	return clip, nil
}

func writeSensitiveQRFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	ok = true
	return nil
}
