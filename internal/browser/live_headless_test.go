package browser_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/JungHoonGhae/oss-coupangctl/internal/browser"
	coupangorders "github.com/JungHoonGhae/oss-coupangctl/internal/coupang/orders"
)

func TestLiveHeadlessDedicatedProfile(t *testing.T) {
	profileDir := os.Getenv("COUPANGCTL_LIVE_PROFILE_DIR")
	if profileDir == "" {
		t.Skip("COUPANGCTL_LIVE_PROFILE_DIR is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	native := browser.NewNative(profileDir)
	if os.Getenv("COUPANGCTL_LIVE_HEADED") == "1" {
		native = browser.NewNativeHeadedSync(profileDir)
	}
	defer native.Close()
	document, err := native.Fetch(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	page, err := coupangorders.ParseOrderDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	if page.Orders == nil {
		t.Fatal("headless order document did not contain an order list")
	}
}
