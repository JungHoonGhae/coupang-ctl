package browser

import (
	"context"
	"os"
	"testing"
	"time"
)

// This opt-in smoke test opens only about:blank twice in one fresh temporary
// profile. It does not navigate to Coupang or read any existing browser state.
func TestLiveInstalledChromeHeadlessLaunch(t *testing.T) {
	if os.Getenv("COUPANGCTL_LIVE_BROWSER_LAUNCH") != "1" {
		t.Skip("COUPANGCTL_LIVE_BROWSER_LAUNCH is not set")
	}
	profileDir := t.TempDir()
	native := NewNative(profileDir)
	executable, err := native.discover()
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		session, err := startChromeSession(ctx, executable, profileDir, true)
		if err != nil {
			cancel()
			t.Fatalf("headless launch %d: %v", attempt, err)
		}
		chrome, ok := session.(*chromeSession)
		if !ok {
			_ = session.Close()
			cancel()
			t.Fatal("headless launch returned an unexpected session")
		}
		assertLiveRuntimeArgumentBinding(t, ctx, chrome)
		if err := session.Close(); err != nil {
			cancel()
			t.Fatalf("headless close %d: %v", attempt, err)
		}
		cancel()
	}
}

func assertLiveRuntimeArgumentBinding(t *testing.T, ctx context.Context, session *chromeSession) {
	t.Helper()
	var created struct {
		TargetID string `json:"targetId"`
	}
	if err := session.browser.Call(ctx, "Target.createTarget", map[string]any{"url": "about:blank"}, &created); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = session.browser.Call(ctx, "Target.closeTarget", map[string]any{"targetId": created.TargetID}, nil)
	}()
	target, err := session.waitForTarget(ctx, created.TargetID)
	if err != nil {
		t.Fatal(err)
	}
	page, err := dialCDP(ctx, target.WebSocketDebuggerURL)
	if err != nil {
		t.Fatal(err)
	}
	defer page.Close()

	const syntheticValue = `synthetic');globalThis.compromised=true;//`
	var result struct {
		Value string `json:"value"`
	}
	if err := evaluateJSONWithStringArgument(
		ctx,
		page,
		`function(value) { return JSON.stringify({value}); }`,
		syntheticValue,
		&result,
	); err != nil {
		t.Fatal(err)
	}
	if result.Value != syntheticValue {
		t.Fatalf("bound runtime value = %q", result.Value)
	}
}
