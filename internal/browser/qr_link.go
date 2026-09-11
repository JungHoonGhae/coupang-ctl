package browser

import (
	"errors"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"net/url"
)

var ErrQRExpired = core.WithErrorCode("qr_expired", errors.New("QR login expired"))

var ErrQRLoginTimedOut = core.WithErrorCode("qr_login_timeout", errors.New("QR login timed out"))

var ErrQRUnexpectedDestination = core.WithErrorCode("qr_return_context_missing", errors.New("QR login reached an unexpected destination"))

var ErrQRLinkUnavailable = core.WithErrorCode("qr_link_unavailable", errors.New("QR login link unavailable"))

const qrLoginLinkExpression = `(() => (async () => {
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
