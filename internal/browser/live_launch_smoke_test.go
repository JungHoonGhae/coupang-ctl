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
		if err := session.Close(); err != nil {
			cancel()
			t.Fatalf("headless close %d: %v", attempt, err)
		}
		cancel()
	}
}
