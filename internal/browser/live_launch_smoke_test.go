package browser

import (
	"context"
	"os"
	"testing"
	"time"
)

// This opt-in smoke test opens only about:blank in a fresh temporary profile.
// It does not navigate to Coupang or read any existing browser state.
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
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	session, err := startChromeSession(ctx, executable, profileDir, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
}
