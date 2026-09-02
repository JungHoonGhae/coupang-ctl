package browser

import "testing"

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
