package loginassist

import (
	"context"
	"errors"
	"testing"
)

func TestResendTargetsDedicatedBrowserAndReportsRequestedNotDelivered(t *testing.T) {
	assistant := New("/synthetic/dedicated-profile")
	assistant.platformSupported = func() bool { return true }
	assistant.findBrowserPID = func(context.Context, string) (int, error) { return 4242, nil }
	assistant.pressResend = func(context.Context, int) error { return nil }
	result, err := assistant.Resend(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Requested || !result.UITransitionVerified || result.DeliveryVerified {
		t.Fatalf("unexpected resend result: %#v", result)
	}
}

func TestResendOutputRequiresVerifiedUITransition(t *testing.T) {
	for _, output := range []string{
		`{"requested":true,"ui_verified":false}`,
		`{"requested":false,"ui_verified":true}`,
		`not-json`,
	} {
		if err := validateResendOutput([]byte(output)); err == nil {
			t.Fatalf("output %q was accepted", output)
		}
	}
	if err := validateResendOutput([]byte(`{"requested":true,"ui_verified":true}`)); err != nil {
		t.Fatalf("verified output was rejected: %v", err)
	}
}

func TestResendDoesNotPressWhenDedicatedBrowserIsMissing(t *testing.T) {
	want := errors.New("missing")
	assistant := New("/synthetic/dedicated-profile")
	assistant.platformSupported = func() bool { return true }
	assistant.findBrowserPID = func(context.Context, string) (int, error) { return 0, want }
	assistant.pressResend = func(context.Context, int) error {
		t.Fatal("resend was pressed without a dedicated browser")
		return nil
	}
	_, err := assistant.Resend(context.Background())
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want %v", err, want)
	}
}

func TestResendRejectsUnsupportedPlatformBeforeInspectingProcesses(t *testing.T) {
	assistant := New("/synthetic/dedicated-profile")
	assistant.platformSupported = func() bool { return false }
	assistant.findBrowserPID = func(context.Context, string) (int, error) {
		t.Fatal("browser processes were inspected on an unsupported platform")
		return 0, nil
	}
	_, err := assistant.Resend(context.Background())
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("error = %v, want %v", err, ErrUnsupported)
	}
}
