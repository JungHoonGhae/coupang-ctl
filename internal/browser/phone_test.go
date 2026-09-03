package browser

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type recordedPhoneCall struct {
	method string
	params map[string]any
}

type recordingPhoneCaller struct {
	calls []recordedPhoneCall
}

func (caller *recordingPhoneCaller) Call(_ context.Context, method string, params any, result any) error {
	if mapped, ok := params.(map[string]any); ok {
		caller.calls = append(caller.calls, recordedPhoneCall{method: method, params: mapped})
	}
	var payload []byte
	switch method {
	case "Runtime.evaluate":
		payload = []byte(`{"result":{"objectId":"synthetic-global"}}`)
	case "Runtime.callFunctionOn":
		payload = []byte(`{"result":{"value":"{\"ready\":false}"}}`)
	default:
		return nil
	}
	if result == nil {
		return nil
	}
	return json.Unmarshal(payload, result)
}

func assertSecretPassedAsRuntimeArgument(t *testing.T, calls []recordedPhoneCall, secret string) {
	t.Helper()
	bound := 0
	for _, call := range calls {
		for key, value := range call.params {
			if key == "arguments" {
				arguments, ok := value.([]map[string]any)
				if !ok || len(arguments) != 1 || arguments[0]["value"] != secret {
					t.Fatalf("runtime arguments = %#v", value)
				}
				bound++
				continue
			}
			if text, ok := value.(string); ok && strings.Contains(text, secret) {
				t.Fatalf("secret was embedded in %s.%s", call.method, key)
			}
		}
	}
	if bound != 1 {
		t.Fatalf("structured secret argument count = %d, want 1", bound)
	}
}

func TestClassifyPhonePage(t *testing.T) {
	for _, test := range []struct {
		name  string
		state phonePageState
		want  phonePagePhase
	}{
		{name: "ready for phone", state: phonePageState{Href: loginURL, PhoneReady: true}, want: phonePhaseReady},
		{name: "waiting for OTP", state: phonePageState{Href: loginURL, OTPReady: true}, want: phonePhaseOTPReady},
		{name: "challenge", state: phonePageState{Href: loginURL, HumanChallenge: true}, want: phonePhaseChallenge},
		{name: "hidden challenge with usable phone form", state: phonePageState{Href: loginURL, HumanChallenge: true, PhoneReady: true}, want: phonePhaseReady},
		{name: "system error", state: phonePageState{Href: loginURL, SystemError: true}, want: phonePhaseSystemError},
		{name: "invalid OTP", state: phonePageState{Href: loginURL, VerificationError: true}, want: phonePhaseVerificationError},
		{name: "approved", state: phonePageState{Href: orderListURL, DocumentReady: true, StructuredOrderData: true}, want: phonePhaseApproved},
		{name: "unexpected", state: phonePageState{Href: "https://www.coupang.com/"}, want: phonePhaseUnexpected},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := classifyPhonePage(test.state); got != test.want {
				t.Fatalf("phase = %q, want %q", got, test.want)
			}
		})
	}
}

func TestNextPhoneOTPActionRequiresFreshRequest(t *testing.T) {
	if got := nextPhoneOTPAction(false, false); got != phoneOTPActionResend {
		t.Fatalf("stale OTP page action = %q, want %q", got, phoneOTPActionResend)
	}
	if got := nextPhoneOTPAction(true, false); got != phoneOTPActionRead {
		t.Fatalf("fresh OTP page action = %q, want %q", got, phoneOTPActionRead)
	}
	if got := nextPhoneOTPAction(true, true); got != phoneOTPActionNone {
		t.Fatalf("submitted OTP page action = %q, want %q", got, phoneOTPActionNone)
	}
}

func TestPreparePhoneOTPUsesStructuredRuntimeArgument(t *testing.T) {
	secret := `01012345678');globalThis.compromised=true;//`
	caller := &recordingPhoneCaller{}
	clicked, err := prepareAndRequestPhoneOTP(context.Background(), caller, secret)
	if err != nil {
		t.Fatal(err)
	}
	if clicked {
		t.Fatal("synthetic button unexpectedly clicked")
	}
	assertSecretPassedAsRuntimeArgument(t, caller.calls, secret)
}

func TestSubmitPhoneOTPUsesStructuredRuntimeArgument(t *testing.T) {
	secret := `123456');globalThis.compromised=true;//`
	caller := &recordingPhoneCaller{}
	clicked, err := submitPhoneOTP(context.Background(), caller, secret)
	if err != nil {
		t.Fatal(err)
	}
	if clicked {
		t.Fatal("synthetic button unexpectedly clicked")
	}
	assertSecretPassedAsRuntimeArgument(t, caller.calls, secret)
}
