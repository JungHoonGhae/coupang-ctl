package recap_test

import (
	"bytes"
	"context"
	"encoding/json"
	"html/template"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/insights"
	"github.com/JungHoonGhae/coupang-ctl/internal/recap"
)

type syntheticPNGRenderer struct {
	html []byte
}

func TestPublicShareMissingDeliveryEvidenceIsNotZeroPercent(t *testing.T) {
	for _, tc := range []struct {
		name    string
		rate    *float64
		samples int
		want    string
	}{
		{"missing", nil, 0, "기록 부족"},
		{"missing_rate", nil, 10, "기록 부족"},
		{"missing_sample", observedRate(0.65), 0, "기록 부족"},
		{"observed_zero", observedRate(0), 10, "0.0%"},
		{"observed_rate", observedRate(0.65), 20, "65.0%"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			summary := syntheticInsights()
			summary.Profile = insights.BuildShoppingProfile(summary)
			summary.DeliveredWithin24HoursRate = tc.rate
			summary.Samples.DeliveryEvents = tc.samples
			preview := recap.PublicSharePreview(summary)
			found := false
			for _, field := range preview.Fields {
				if field.ID == "delivered_within_24_hours_rate" {
					found = true
					if field.Value != tc.want {
						t.Fatalf("delivery evidence presented as %q, want %q", field.Value, tc.want)
					}
				}
			}
			if !found {
				t.Fatal("preview missing delivery field")
			}
			var rendered bytes.Buffer
			if err := recap.RenderPublicShareCard(&rendered, summary); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(rendered.String(), `data-share-field="delivered_within_24_hours_rate" data-share-value="`+tc.want+`"`) {
				t.Fatal("public card disagrees with delivery evidence")
			}
			var full bytes.Buffer
			if err := recap.Render(&full, summary); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(full.String(), `<strong>`+tc.want+`</strong><span>24시간 안에 도착`) {
				t.Fatal("full recap disagrees with delivery evidence")
			}
		})
	}
}

func TestPublicShareDoesNotExposePrivateSnapshotEvidence(t *testing.T) {
	summary := syntheticInsights()
	summary.Profile = insights.BuildShoppingProfile(summary)
	summary.Evidence = core.OrderReadEvidence{
		Visibility: "private_local", Dataset: "retained_local_history",
		SnapshotCapturedAt: time.Date(2026, 9, 8, 12, 34, 56, 0, time.UTC),
		LatestAttempt:      core.SyncStatus{ErrorCode: "synthetic_private_attempt_marker", StartedAt: "2026-09-08T11:22:33Z"},
		Limitations:        []string{"synthetic_private_evidence_marker"},
	}
	preview, err := json.Marshal(recap.PublicSharePreview(summary))
	if err != nil {
		t.Fatal(err)
	}
	var html bytes.Buffer
	if err := recap.RenderPublicShareCard(&html, summary); err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"2026-09-08", "12:34:56", "11:22:33", "synthetic_private_attempt_marker", "synthetic_private_evidence_marker", "retained_local_history"} {
		if bytes.Contains(preview, []byte(private)) || bytes.Contains(html.Bytes(), []byte(private)) {
			t.Fatalf("public output exposed snapshot detail %q", private)
		}
	}
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

func TestRenderedShareCardCarriesEveryPreviewedFieldContract(t *testing.T) {
	summary := syntheticInsights()
	summary.Profile = insights.BuildShoppingProfile(summary)
	preview := recap.PublicSharePreview(summary)
	var rendered bytes.Buffer
	if err := recap.RenderPublicShareCard(&rendered, summary); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.String(), `content="default-src 'none'; img-src data:; font-src data:; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'"`) {
		t.Fatal("public card must restrict resources to embedded assets")
	}
	for _, field := range preview.Fields {
		marker := `data-share-field="` + template.HTMLEscapeString(field.ID) + `" data-share-value="` + template.HTMLEscapeString(field.Value) + `"`
		if !bytes.Contains(rendered.Bytes(), []byte(marker)) {
			t.Fatalf("rendered share card omits preview field %q=%q", field.ID, field.Value)
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
