package recap_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/insights"
	"github.com/JungHoonGhae/coupang-ctl/internal/recap"
)

type syntheticPNGRenderer struct {
	html []byte
}

func (renderer *syntheticPNGRenderer) RenderPNG(_ context.Context, htmlPath, outputPath string, width, height int) error {
	content, err := os.ReadFile(htmlPath)
	if err != nil {
		return err
	}
	renderer.html = content
	output, err := os.OpenFile(outputPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	encodeErr := png.Encode(output, canvas)
	closeErr := output.Close()
	if encodeErr != nil {
		return encodeErr
	}
	return closeErr
}

func TestPublicSharePreviewListsExactVisibleFieldsAndExclusions(t *testing.T) {
	summary := syntheticInsights()
	summary.Profile = insights.BuildShoppingProfile(summary)
	preview := recap.PublicSharePreview(summary)
	if preview.Visibility != "public_safe" || preview.Format != "png" || preview.Width != 1080 || preview.Height != 1350 || !preview.Ready || preview.ConfirmationFlag != "--confirm-public-safe-image" {
		t.Fatalf("unexpected share preview: %#v", preview)
	}
	encoded, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, required := range []string{"analysis_period_month", "shopping_profile_code", "axis.rhythm", "order_count", "delivered_within_24_hours_rate", "product_names", "exact_order_dates"} {
		if !strings.Contains(text, required) {
			t.Fatalf("share preview omits %q: %s", required, text)
		}
	}
	for _, private := range []string{"Synthetic private brand", "2024-01-05", "2025-12-28"} {
		if strings.Contains(text, private) {
			t.Fatalf("share preview exposed private detail %q: %s", private, text)
		}
	}
}

func TestWritePublicShareImageUsesPrivateNewFileAndNeverOverwrites(t *testing.T) {
	summary := syntheticInsights()
	summary.Profile = insights.BuildShoppingProfile(summary)
	outputPath := filepath.Join(t.TempDir(), "shopping-recap.png")
	renderer := &syntheticPNGRenderer{}
	result, err := recap.WritePublicShareImage(context.Background(), outputPath, summary, renderer)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Written || result.Format != "png" || result.Visibility != "public_safe" || result.Width != 1080 || result.Height != 1350 || result.Bytes < 1 || !result.Preview.Ready {
		t.Fatalf("unexpected image result: %#v", result)
	}
	if !bytes.Contains(renderer.html, []byte("BDFO")) || bytes.Contains(renderer.html, []byte("Synthetic private brand")) || bytes.Contains(renderer.html, []byte("2024-01-05")) {
		t.Fatal("share-card HTML did not preserve the public-safe field boundary")
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("share image mode = %o, want 600", info.Mode().Perm())
	}
	if _, err := recap.WritePublicShareImage(context.Background(), outputPath, summary, renderer); err == nil {
		t.Fatal("existing share image was overwritten")
	}
}
