package recommendationreport

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

// Opt-in installed-engine QA of the actual rendered decision section/CSS.
// Synthetic inputs only; no authenticated profile, shopping request or JS.
func TestDecisionSectionInstalledEngine(t *testing.T) {
	met := decisionCandidate("101", "합성 후보 A — 독립 옵션", "32GB")
	unmet := decisionCandidate("102", "합성 후보 B — RAM 조건 부족", "16GB")
	unmet.ExclusionReason = "required_condition_unmet"
	unknown := decisionCandidate("103", "합성 후보 C — 복합 옵션", "32GB × 1TB")
	unknown.Inspection.SelectedAttributes[0].Name = "RAM용량 × 저장용량"
	unknown.ExclusionReason = "required_condition_unverified"
	report := core.ProductRecommendationReport{Title: "합성 조건 검토", Recommendation: core.ProductRecommendationResult{SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete, Candidates: []core.ProductRecommendationCandidate{met}, Inspected: []core.ProductRecommendationCandidate{met, unmet, unknown}}}
	renderInstalledSection(t, report, "condition-review", 3200, 2400)
}

func TestPurchaseSectionInstalledEngine(t *testing.T) {
	r := syntheticPurchaseReport()
	r.Title = "합성 구매 맥락 검토"
	r.Recommendation.Candidates[0].Product.Name = "합성 후보 — 비교 중인 판매 옵션"
	r.Recommendation.Inspected[0].Product.Name = r.Recommendation.Candidates[0].Product.Name
	r.Recommendation.PurchaseContext.Matches = append(r.Recommendation.PurchaseContext.Matches, core.RecommendationPurchaseMatch{Reference: core.ProductReference{ProductID: "101"}, IdentityScope: "product_only", OrderCount: 3, ItemLineCount: 4, RetainedUnits: 7})
	renderInstalledSection(t, r, "purchase-context", 4096, 3000)
}

func renderInstalledSection(t *testing.T, report core.ProductRecommendationReport, section string, mobileHeight, desktopHeight int) {
	t.Helper()
	if os.Getenv("COUPANGCTL_LIVE_LOCAL_RENDER") != "1" {
		t.Skip("opt-in disposable-profile local rendering")
	}
	dir := os.Getenv("COUPANGCTL_LOCAL_RENDER_OUTPUT_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	if !filepath.IsAbs(dir) {
		t.Fatal("output directory must be absolute")
	}
	dir = filepath.Join(dir, section)
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	data, err := Render(report)
	if err != nil {
		t.Fatal(err)
	}
	full := string(data)
	start := strings.Index(full, `<section id="`+section+`">`)
	if start < 0 {
		t.Fatal("requested section missing")
	}
	endOffset := strings.Index(full[start:], "</section>")
	headEnd := strings.Index(full, "</head>")
	if endOffset < 0 || headEnd < 0 {
		t.Fatal("report section or head is not closed")
	}
	end := start + endOffset + len("</section>")
	focused := full[:headEnd+len("</head>")] + "<body><main>" + full[start:end] + "</main></body></html>"
	input, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{"report.json": string(input), "report.html": full, "section.html": focused} {
		f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.WriteString(contents)
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			t.Fatal("write synthetic HTML failed")
		}
	}
	for _, size := range []struct {
		name          string
		width, height int
	}{{"mobile", 390, mobileHeight}, {"desktop", 1440, desktopHeight}} {
		if err := browser.NewLocalPageRenderer().RenderPNG(context.Background(), filepath.Join(dir, "section.html"), filepath.Join(dir, size.name+".png"), size.width, size.height); err != nil {
			t.Fatal(err)
		}
	}
	t.Log("rendered synthetic section at mobile and desktop widths; separate disposable engine profiles")
}
