package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/recommendationreport"
)

const syntheticReportJSON = `{"title":"Synthetic report","recommendation":{"schema_version":5,"status":"incomplete","warnings":["Synthetic missing source"]}}`

func TestProductReportRunPreservesExcludedDecisionsOffline(t *testing.T) {
	t.Setenv("COUPANGCTL_STATE_DIR", "deliberately-invalid-relative-state")
	limit := int64(10000)
	r := core.ProductRecommendationReport{Title: "Synthetic decisions", Recommendation: core.ProductRecommendationResult{SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete, Audit: core.ProductRecommendationAudit{InspectionStopReason: "source_access_denied", UninspectedExactOptions: 7}, NextActions: []core.ProductRecommendationNextAction{{Kind: "restore_source_access", Reason: "source_access_denied"}}}}
	for n, id := range []string{"101", "102", "103"} {
		ref := core.ProductReference{ProductID: id, ItemID: id + "1", VendorItemID: id + "2"}
		p := core.ProductCard{Reference: ref, Name: "Synthetic " + id, Price: core.ProductPrice{Currency: "KRW", CurrentAmount: int64(5000 + n*10000)}, ObservedFields: []string{"price.current_amount"}, FieldEvidence: []core.ProductFieldEvidence{{Field: "price.current_amount", Source: "json_ld", Locator: "offers.price", Method: "native_field", Provenance: "observed", Scope: "selected_option", Reference: ref, CapturedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}}}
		if n == 2 {
			p.FieldEvidence = nil
		}
		c := core.ProductRecommendationCandidate{Product: p, Inspection: core.ProductInspection{Product: p}, Conditions: []core.ProductConditionAssessment{{Condition: core.ProductRecommendationCondition{ID: "price", Field: "price.current_amount", Operator: "lte", Integer: &limit}, Status: core.ProductConditionMet}}}
		if n == 0 {
			r.Recommendation.Candidates = append(r.Recommendation.Candidates, c)
		} else {
			c.ExclusionReason = []string{"", "required_condition_unmet", "required_condition_unverified"}[n]
		}
		r.Recommendation.Inspected = append(r.Recommendation.Inspected, c)
	}
	input := filepath.Join(t.TempDir(), "synthetic.json")
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(input, data, 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"products", "report", "--input", input}, &out, &stderr, "test"); err != nil {
		t.Fatal(err)
	}
	var got core.ProductRecommendationReportRenderResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Synthetic 102", "Synthetic 103", "조건 판정: 충족", "조건 판정: 불충족", "조건 판정: 미확인", "required_condition_unmet", "required_condition_unverified", "source_access_denied", "상세 확인을 마치지 못한 판매 옵션: 7개", "현재 연결의 접속 상태부터 확인"} {
		if !strings.Contains(got.HTML, want) {
			t.Fatalf("CLI omitted %q", want)
		}
	}
	want, err := recommendationreport.RenderResult(context.Background(), r)
	if err != nil || got != want || stderr.Len() > 0 {
		t.Fatal("offline CLI diverged from shared renderer or prompted for setup")
	}
}

func TestProductReportRunDoesNotInitializeAccountState(t *testing.T) {
	t.Setenv("COUPANGCTL_STATE_DIR", "deliberately-invalid-relative-state")
	input := filepath.Join(t.TempDir(), "synthetic.json")
	if err := os.WriteFile(input, []byte(syntheticReportJSON), 0600); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"products", "report", "--input", input}, &out, &stderr, "test"); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["visibility"] != "private_local" || result["schema_version"] != float64(2) || result["recommendation_status"] != "incomplete" || !strings.Contains(result["html"].(string), "Synthetic missing source") {
		t.Fatal("report envelope lost outcome")
	}
	if stderr.Len() > 0 {
		t.Fatal("offline render produced setup prompts")
	}
}

func TestProductReportInputAndExplicitFileOutput(t *testing.T) {
	var out bytes.Buffer
	if err := runProductReport(context.Background(), []string{"--input", "-"}, strings.NewReader(syntheticReportJSON), &out); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "report.html")
	out.Reset()
	if err := runProductReport(context.Background(), []string{"--input", "-", "--output", target}, strings.NewReader(syntheticReportJSON), &out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil || !strings.Contains(string(data), "검증 미완료") {
		t.Fatal("explicit report file missing")
	}
	info, err := os.Stat(target)
	if err != nil || info.Mode().Perm()&0077 != 0 {
		t.Fatal("report file must be private")
	}
	if err := runProductReport(context.Background(), []string{"--input", "-", "--output", target}, strings.NewReader(syntheticReportJSON), &out); err == nil {
		t.Fatal("overwrote existing file")
	}
	after, _ := os.ReadFile(target)
	if !bytes.Equal(data, after) {
		t.Fatal("existing report changed")
	}
	for _, input := range []string{syntheticReportJSON + " {}", `{"title":"Synthetic","unexpected":"synthetic-sensitive-marker"}`, "null", strings.Repeat(" ", core.ProductReportMaxInputBytes+1)} {
		out.Reset()
		err := runProductReport(context.Background(), []string{"--input", "-"}, strings.NewReader(input), &out)
		if err == nil || out.Len() != 0 || strings.Contains(err.Error(), "synthetic-sensitive-marker") {
			t.Fatal("invalid input accepted or echoed")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	target = filepath.Join(dir, "cancelled.html")
	if err := runProductReport(ctx, []string{"--input", "-", "--output", target}, strings.NewReader(syntheticReportJSON), &out); err == nil {
		t.Fatal("cancelled request continued")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("cancelled request created a file")
	}
}
