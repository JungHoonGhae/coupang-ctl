package recap_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
	"github.com/JungHoonGhae/coupang-ctl/internal/insights"
	"github.com/JungHoonGhae/coupang-ctl/internal/recap"
)

// Opt-in installed-engine smoke test. It renders synthetic data only, uses no
// authenticated browser session, and does not fetch any shopping documents.
func TestCamoufoxLocalRender(t *testing.T) {
	if os.Getenv("COUPANGCTL_LIVE_LOCAL_RENDER") != "1" {
		t.Skip("set COUPANGCTL_LIVE_LOCAL_RENDER=1 to test the installed Camoufox engine")
	}
	dir := os.Getenv("COUPANGCTL_LOCAL_RENDER_OUTPUT_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	if !filepath.IsAbs(dir) {
		t.Fatal("synthetic render output directory must be absolute")
	}
	summary := syntheticInsights()
	summary.Profile = insights.BuildShoppingProfile(summary)
	result, err := recap.WritePublicShareImage(context.Background(), filepath.Join(dir, "synthetic-recap.png"), summary, browser.NewLocalPageRenderer())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Written || result.Width != 1080 || result.Height != 1350 || result.Bytes < 1 || !result.Preview.Ready {
		t.Fatal("installed-engine render did not produce the expected synthetic card")
	}
	t.Logf("synthetic card: %dx%d, %d bytes; installed Camoufox engine, disposable profile", result.Width, result.Height, result.Bytes)
}
