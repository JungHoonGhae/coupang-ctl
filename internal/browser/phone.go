package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

const phoneLoginTimeout = 10 * time.Minute

var ErrPhoneRequestUnverified = errors.New("phone OTP request was not verified")
var ErrPhoneSystemError = errors.New("phone login reported a system error")
var ErrPhoneVerificationFailed = errors.New("phone OTP verification failed")
var ErrPhoneLoginTimedOut = errors.New("phone login timed out")
var ErrPhoneUnexpectedDestination = errors.New("phone login reached an unexpected destination")

type phonePagePhase string
type phoneOTPAction string

const (
	phonePhaseWaiting           phonePagePhase = "waiting"
	phonePhaseReady             phonePagePhase = "ready"
	phonePhaseOTPReady          phonePagePhase = "otp_ready"
	phonePhaseChallenge         phonePagePhase = "challenge"
	phonePhaseSystemError       phonePagePhase = "system_error"
	phonePhaseVerificationError phonePagePhase = "verification_error"
	phonePhaseApproved          phonePagePhase = "approved"
	phonePhaseUnexpected        phonePagePhase = "unexpected"
	phonePhaseDenied            phonePagePhase = "denied"
)

const (
	phoneOTPActionNone   phoneOTPAction = "none"
	phoneOTPActionResend phoneOTPAction = "resend"
	phoneOTPActionRead   phoneOTPAction = "read"
)

type phonePageState struct {
	Href                string `json:"href"`
	Status              int    `json:"status"`
	DocumentReady       bool   `json:"documentReady"`
	StructuredOrderData bool   `json:"structuredOrderData"`
	PhoneReady          bool   `json:"phoneReady"`
	OTPReady            bool   `json:"otpReady"`
	HumanChallenge      bool   `json:"humanChallenge"`
	SystemError         bool   `json:"systemError"`
	VerificationError   bool   `json:"verificationError"`
	AccessDenied        bool   `json:"accessDenied"`
}

type phoneCDPCaller interface {
	Call(context.Context, string, any, any) error
}

func classifyPhonePage(state phonePageState) phonePagePhase {
	if state.Status == http.StatusForbidden || state.AccessDenied {
		return phonePhaseDenied
	}
	parsed, err := url.Parse(state.Href)
	if err != nil || parsed.Host == "" {
		return phonePhaseWaiting
	}
	if parsed.Host == "mc.coupang.com" && parsed.Path == "/ssr/desktop/order/list" {
		if state.DocumentReady && state.StructuredOrderData {
			return phonePhaseApproved
		}
		return phonePhaseWaiting
	}
	if parsed.Host != "login.coupang.com" {
		return phonePhaseUnexpected
	}
	if state.VerificationError {
		return phonePhaseVerificationError
	}
	if state.SystemError {
		return phonePhaseSystemError
	}
	if state.OTPReady {
		return phonePhaseOTPReady
	}
	if state.PhoneReady {
		return phonePhaseReady
	}
	if state.HumanChallenge {
		return phonePhaseChallenge
	}
	return phonePhaseWaiting
}

func presentPhoneLogin(ctx context.Context, executable, profileDir, targetURL, phone string, readOTP core.OTPProvider) error {
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
		return errors.New("phone browser session unavailable")
	}
	defer session.Close()
	if err := session.presentPhone(ctx, targetURL, phone, readOTP); err != nil {
		return err
	}
	return nil
}

func (s *chromeSession) presentPhone(ctx context.Context, targetURL, phone string, readOTP core.OTPProvider) error {
	var created struct {
		TargetID string `json:"targetId"`
	}
	if err := s.browser.Call(ctx, "Target.createTarget", map[string]any{"url": "about:blank"}, &created); err != nil {
		return fmt.Errorf("create phone login page: %w", err)
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
		return fmt.Errorf("connect phone login page: %w", err)
	}
	defer page.Close()
	if err := page.Call(ctx, "Page.enable", struct{}{}, nil); err != nil {
		return fmt.Errorf("enable phone login page: %w", err)
	}
	if err := page.Call(ctx, "Page.navigate", map[string]any{"url": targetURL}, nil); err != nil {
		return fmt.Errorf("navigate phone login page: %w", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, phoneLoginTimeout)
	defer cancel()
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	requestClicked := false
	otpSubmitted := false
	requestDeadline := time.Time{}

	for {
		state, stateErr := readPhonePageState(waitCtx, page)
		if stateErr == nil {
			switch classifyPhonePage(state) {
			case phonePhaseApproved:
				return nil
			case phonePhaseDenied:
				return ErrBrowserAccessDenied
			case phonePhaseUnexpected:
				return ErrPhoneUnexpectedDestination
			case phonePhaseSystemError:
				return ErrPhoneSystemError
			case phonePhaseVerificationError:
				return ErrPhoneVerificationFailed
			case phonePhaseOTPReady:
				if !state.DocumentReady {
					break
				}
				switch nextPhoneOTPAction(requestClicked, otpSubmitted) {
				case phoneOTPActionResend:
					clicked, err := resendPhoneOTP(waitCtx, page)
					if err != nil {
						return err
					}
					if !clicked {
						return ErrPhoneRequestUnverified
					}
					requestClicked = true
					requestDeadline = time.Now().Add(30 * time.Second)
				case phoneOTPActionRead:
					otp, err := readOTP(waitCtx)
					if err != nil {
						return err
					}
					if !validOTP(otp) {
						return core.ErrInvalidLoginRequest
					}
					submitted, err := submitPhoneOTP(waitCtx, page, otp)
					if err != nil {
						return err
					}
					if !submitted {
						return ErrPhoneVerificationFailed
					}
					otpSubmitted = true
				}
			case phonePhaseChallenge:
				// A human may solve the visible challenge; never inspect or bypass it.
			default:
				if !state.DocumentReady {
					break
				}
				if !requestClicked {
					clicked, err := prepareAndRequestPhoneOTP(waitCtx, page, phone)
					if err != nil {
						return err
					}
					if clicked {
						requestClicked = true
						requestDeadline = time.Now().Add(30 * time.Second)
					}
				}
			}
		}
		if requestClicked && !otpSubmitted && !requestDeadline.IsZero() && time.Now().After(requestDeadline) {
			return ErrPhoneRequestUnverified
		}
		select {
		case <-waitCtx.Done():
			return ErrPhoneLoginTimedOut
		case <-ticker.C:
		}
	}
}

func nextPhoneOTPAction(requestClicked, otpSubmitted bool) phoneOTPAction {
	if otpSubmitted {
		return phoneOTPActionNone
	}
	if !requestClicked {
		return phoneOTPActionResend
	}
	return phoneOTPActionRead
}

func readPhonePageState(ctx context.Context, page phoneCDPCaller) (phonePageState, error) {
	const expression = `(() => {
		const documents = [document];
		for (const frame of document.querySelectorAll('iframe')) {
			try { if (frame.contentDocument) documents.push(frame.contentDocument); } catch (_) {}
		}
		const text = documents.map((doc) => doc.body?.innerText ?? '').join('\n');
		const inputs = documents.flatMap((doc) => [...doc.querySelectorAll('input')]);
		const navigation = performance.getEntriesByType('navigation')[0];
		return JSON.stringify({
			href: location.href,
			status: navigation?.responseStatus ?? 0,
			documentReady: document.readyState === 'complete',
			structuredOrderData: (document.getElementById('__NEXT_DATA__')?.textContent?.length ?? 0) > 0,
			phoneReady: inputs.some((input) => /휴대폰|전화번호/.test(input.placeholder ?? '') || input.type === 'tel'),
			otpReady: inputs.some((input) => /인증번호/.test(input.placeholder ?? '')) || /재발송/.test(text),
			humanChallenge: /자동입력 방지문자|보안문자|captcha/i.test(text),
			systemError: /시스템 오류/.test(text),
			verificationError: /인증번호.{0,16}(올바르지|일치하지|만료|오류|실패)|(?:올바르지|일치하지|만료).{0,16}인증번호/.test(text),
			accessDenied: /access denied|접근.{0,8}(거부|제한)|비정상.{0,8}접근/i.test(text)
		});
	})()`
	return evaluatePhoneState(ctx, page, expression)
}

func prepareAndRequestPhoneOTP(ctx context.Context, page phoneCDPCaller, phone string) (bool, error) {
	const functionDeclaration = `function(phone) {
		const documents = [document];
		for (const frame of document.querySelectorAll('iframe')) {
			try { if (frame.contentDocument) documents.push(frame.contentDocument); } catch (_) {}
		}
		const elements = documents.flatMap((doc) => [...doc.querySelectorAll('a,button,[role="tab"],li')]);
		const tab = elements.find((element) => /^(휴대폰번호|휴대폰번호로) 로그인$/.test(element.textContent?.trim() ?? ''));
		if (tab && typeof tab.click === 'function') tab.click();
		const inputs = documents.flatMap((doc) => [...doc.querySelectorAll('input')]);
		const input = inputs.find((element) => /휴대폰|전화번호/.test(element.placeholder ?? '') || element.type === 'tel');
		if (!input || input.tagName !== 'INPUT') return JSON.stringify({ready:false});
		const inputType = input.ownerDocument.defaultView?.HTMLInputElement;
		const setter = inputType ? Object.getOwnPropertyDescriptor(inputType.prototype, 'value')?.set : null;
		if (!setter) return JSON.stringify({ready:false});
		setter.call(input, phone);
		const EventType = input.ownerDocument.defaultView?.Event;
		if (!EventType) return JSON.stringify({ready:false});
		input.dispatchEvent(new EventType('input', {bubbles:true}));
		input.dispatchEvent(new EventType('change', {bubbles:true}));
		const scope = input.closest('form') ?? input.ownerDocument;
		const refreshed = [...scope.querySelectorAll('button')];
		const send = refreshed.find((element) => /^(인증번호 발송|인증번호 받기|전송)$/.test(element.textContent?.trim() ?? ''));
		if (!send || send.tagName !== 'BUTTON' || send.disabled) return JSON.stringify({ready:false});
		return JSON.stringify(buttonCenter(send));

		function buttonCenter(button) {
			button.scrollIntoView({block:'center', inline:'center'});
			const rect = button.getBoundingClientRect();
			if (rect.width <= 0 || rect.height <= 0) return {ready:false};
			let x = rect.left + rect.width / 2;
			let y = rect.top + rect.height / 2;
			let view = button.ownerDocument.defaultView;
			while (view && view !== window) {
				const frame = view.frameElement;
				if (!frame) return {ready:false};
				const frameRect = frame.getBoundingClientRect();
				x += frameRect.left;
				y += frameRect.top;
				view = frame.ownerDocument.defaultView;
			}
			return {ready:true, x, y};
		}
	}`
	return evaluateAndClickPhoneButtonWithStringArgument(ctx, page, functionDeclaration, phone)
}

func submitPhoneOTP(ctx context.Context, page phoneCDPCaller, otp string) (bool, error) {
	const functionDeclaration = `function(otp) {
		const documents = [document];
		for (const frame of document.querySelectorAll('iframe')) {
			try { if (frame.contentDocument) documents.push(frame.contentDocument); } catch (_) {}
		}
		const inputs = documents.flatMap((doc) => [...doc.querySelectorAll('input')]);
		const input = inputs.find((element) => /인증번호/.test(element.placeholder ?? ''));
		if (!input || input.tagName !== 'INPUT') return JSON.stringify({ready:false});
		const inputType = input.ownerDocument.defaultView?.HTMLInputElement;
		const setter = inputType ? Object.getOwnPropertyDescriptor(inputType.prototype, 'value')?.set : null;
		if (!setter) return JSON.stringify({ready:false});
		setter.call(input, otp);
		const EventType = input.ownerDocument.defaultView?.Event;
		if (!EventType) return JSON.stringify({ready:false});
		input.dispatchEvent(new EventType('input', {bubbles:true}));
		input.dispatchEvent(new EventType('change', {bubbles:true}));
		const scope = input.closest('form') ?? input.ownerDocument;
		const buttons = [...scope.querySelectorAll('button')];
		const submit = buttons.find((element) => element.textContent?.trim() === '로그인');
		if (!submit || submit.tagName !== 'BUTTON' || submit.disabled) return JSON.stringify({ready:false});
		return JSON.stringify(buttonCenter(submit));

		function buttonCenter(button) {
			button.scrollIntoView({block:'center', inline:'center'});
			const rect = button.getBoundingClientRect();
			if (rect.width <= 0 || rect.height <= 0) return {ready:false};
			let x = rect.left + rect.width / 2;
			let y = rect.top + rect.height / 2;
			let view = button.ownerDocument.defaultView;
			while (view && view !== window) {
				const frame = view.frameElement;
				if (!frame) return {ready:false};
				const frameRect = frame.getBoundingClientRect();
				x += frameRect.left;
				y += frameRect.top;
				view = frame.ownerDocument.defaultView;
			}
			return {ready:true, x, y};
		}
	}`
	return evaluateAndClickPhoneButtonWithStringArgument(ctx, page, functionDeclaration, otp)
}

func resendPhoneOTP(ctx context.Context, page phoneCDPCaller) (bool, error) {
	const expression = `(() => {
		const documents = [document];
		for (const frame of document.querySelectorAll('iframe')) {
			try { if (frame.contentDocument) documents.push(frame.contentDocument); } catch (_) {}
		}
		const inputs = documents.flatMap((doc) => [...doc.querySelectorAll('input')]);
		const input = inputs.find((element) => /인증번호/.test(element.placeholder ?? ''));
		if (!input || input.tagName !== 'INPUT') return JSON.stringify({ready:false});
		const scope = input.closest('form') ?? input.ownerDocument;
		const buttons = [...scope.querySelectorAll('button')];
		const resend = buttons.find((element) => element.textContent?.trim() === '재발송');
		if (!resend || resend.tagName !== 'BUTTON' || resend.disabled) return JSON.stringify({ready:false});
		return JSON.stringify(buttonCenter(resend));

		function buttonCenter(button) {
			button.scrollIntoView({block:'center', inline:'center'});
			const rect = button.getBoundingClientRect();
			if (rect.width <= 0 || rect.height <= 0) return {ready:false};
			let x = rect.left + rect.width / 2;
			let y = rect.top + rect.height / 2;
			let view = button.ownerDocument.defaultView;
			while (view && view !== window) {
				const frame = view.frameElement;
				if (!frame) return {ready:false};
				const frameRect = frame.getBoundingClientRect();
				x += frameRect.left;
				y += frameRect.top;
				view = frame.ownerDocument.defaultView;
			}
			return {ready:true, x, y};
		}
	})()`
	return evaluateAndClickPhoneButton(ctx, page, expression)
}

type phoneButtonTarget struct {
	Ready bool    `json:"ready"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
}

func evaluateAndClickPhoneButton(ctx context.Context, page phoneCDPCaller, expression string) (bool, error) {
	var target phoneButtonTarget
	if err := evaluateJSON(ctx, page, expression, &target); err != nil {
		return false, err
	}
	return clickPhoneButtonTarget(ctx, page, target)
}

func evaluateAndClickPhoneButtonWithStringArgument(ctx context.Context, page phoneCDPCaller, functionDeclaration, value string) (bool, error) {
	var target phoneButtonTarget
	if err := evaluateJSONWithStringArgument(ctx, page, functionDeclaration, value, &target); err != nil {
		return false, err
	}
	return clickPhoneButtonTarget(ctx, page, target)
}

func clickPhoneButtonTarget(ctx context.Context, page phoneCDPCaller, target phoneButtonTarget) (bool, error) {
	if !target.Ready {
		return false, nil
	}
	for _, event := range []map[string]any{
		{"type": "mouseMoved", "x": target.X, "y": target.Y},
		{"type": "mousePressed", "x": target.X, "y": target.Y, "button": "left", "clickCount": 1},
		{"type": "mouseReleased", "x": target.X, "y": target.Y, "button": "left", "clickCount": 1},
	} {
		if err := page.Call(ctx, "Input.dispatchMouseEvent", event, nil); err != nil {
			return false, fmt.Errorf("click phone login button: %w", err)
		}
	}
	return true, nil
}

func evaluatePhoneState(ctx context.Context, page phoneCDPCaller, expression string) (phonePageState, error) {
	var state phonePageState
	if err := evaluateJSON(ctx, page, expression, &state); err != nil {
		return phonePageState{}, err
	}
	return state, nil
}

type runtimeJSONEvaluation struct {
	Result struct {
		Value json.RawMessage `json:"value"`
	} `json:"result"`
	ExceptionDetails json.RawMessage `json:"exceptionDetails,omitempty"`
}

func evaluateJSON(ctx context.Context, page phoneCDPCaller, expression string, target any) error {
	var evaluated runtimeJSONEvaluation
	if err := page.Call(ctx, "Runtime.evaluate", map[string]any{"expression": expression, "returnByValue": true}, &evaluated); err != nil {
		return err
	}
	return decodeRuntimeJSON(evaluated, target)
}

func evaluateJSONWithStringArgument(ctx context.Context, page phoneCDPCaller, functionDeclaration, value string, target any) error {
	var global struct {
		Result struct {
			ObjectID string `json:"objectId"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails,omitempty"`
	}
	if err := page.Call(ctx, "Runtime.evaluate", map[string]any{"expression": "globalThis"}, &global); err != nil {
		return err
	}
	if len(global.ExceptionDetails) != 0 || global.Result.ObjectID == "" {
		return errors.New("evaluate phone login page")
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = page.Call(releaseCtx, "Runtime.releaseObject", map[string]any{"objectId": global.Result.ObjectID}, nil)
	}()

	var evaluated runtimeJSONEvaluation
	if err := page.Call(ctx, "Runtime.callFunctionOn", map[string]any{
		"objectId":            global.Result.ObjectID,
		"functionDeclaration": functionDeclaration,
		"arguments":           []map[string]any{{"value": value}},
		"returnByValue":       true,
		"userGesture":         true,
	}, &evaluated); err != nil {
		return err
	}
	return decodeRuntimeJSON(evaluated, target)
}

func decodeRuntimeJSON(evaluated runtimeJSONEvaluation, target any) error {
	if len(evaluated.ExceptionDetails) != 0 {
		return errors.New("evaluate phone login page")
	}
	var encoded string
	if err := json.Unmarshal(evaluated.Result.Value, &encoded); err != nil {
		return errors.New("decode phone login page")
	}
	if err := json.Unmarshal([]byte(encoded), target); err != nil {
		return errors.New("decode phone login page")
	}
	return nil
}

func validOTP(value string) bool {
	if len(value) != 6 {
		return false
	}
	return strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) == -1
}
