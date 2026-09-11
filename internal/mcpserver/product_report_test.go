package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/recommendationreport"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestProductReportMCPWorksWithoutSourceProviders(t *testing.T) {
	ctx := context.Background()
	server := NewWithProviders(Providers{}, "test")
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	// Use the complete typed envelope produced by the recommendation adapter;
	// MCP validates required fields from that schema before invoking the handler.
	report := core.ProductRecommendationReport{Title: "Synthetic report", Recommendation: core.ProductRecommendationResult{SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete, Warnings: []string{"Synthetic missing source"}}}
	limit := int64(10000)
	for n, id := range []string{"101", "102", "103"} {
		ref := core.ProductReference{ProductID: id, ItemID: id + "1", VendorItemID: id + "2"}
		p := core.ProductCard{Reference: ref, Name: "Synthetic " + id, Price: core.ProductPrice{Currency: "KRW", CurrentAmount: int64(5000 + n*10000)}, ObservedFields: []string{"price.current_amount"}, FieldEvidence: []core.ProductFieldEvidence{{Field: "price.current_amount", Source: "json_ld", Locator: "offers.price", Method: "native_field", Provenance: "observed", Scope: "selected_option", Reference: ref, CapturedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}}}
		if n == 2 {
			p.FieldEvidence = nil
		}
		c := core.ProductRecommendationCandidate{Product: p, Inspection: core.ProductInspection{Product: p}, Conditions: []core.ProductConditionAssessment{{Condition: core.ProductRecommendationCondition{ID: "price", Field: "price.current_amount", Operator: "lte", Integer: &limit}, Status: core.ProductConditionMet}}}
		if n == 0 {
			report.Recommendation.Candidates = append(report.Recommendation.Candidates, c)
		} else {
			c.ExclusionReason = []string{"", "required_condition_unmet", "required_condition_unverified"}[n]
		}
		report.Recommendation.Inspected = append(report.Recommendation.Inspected, c)
	}
	report.Recommendation.Audit = core.ProductRecommendationAudit{InspectionStopReason: "source_access_denied", UninspectedExactOptions: 7}
	report.Recommendation.NextActions = []core.ProductRecommendationNextAction{{Kind: "restore_source_access", Reason: "source_access_denied"}}
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "products_report_render", Arguments: core.ProductRecommendationReportRenderRequest{Report: report}})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		detail, _ := json.Marshal(result.Content)
		t.Fatalf("synthetic report tool failed: %s", detail)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var output map[string]any
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	if output["visibility"] != "private_local" || output["recommendation_status"] != "incomplete" || !strings.Contains(output["html"].(string), "Synthetic missing source") {
		t.Fatal("report did not preserve typed outcome")
	}
	want, err := recommendationreport.RenderResult(ctx, report)
	if err != nil {
		t.Fatal(err)
	}
	if output["html"] != want.HTML || output["bytes_rendered"] != float64(want.BytesRendered) {
		t.Fatal("MCP diverged from shared renderer")
	}
	for _, fragment := range []string{"Synthetic 102", "Synthetic 103", "조건 판정: 충족", "조건 판정: 불충족", "조건 판정: 미확인", "required_condition_unmet", "required_condition_unverified", "source_access_denied", "상세 확인을 마치지 못한 판매 옵션: 7개", "현재 연결의 접속 상태부터 확인"} {
		if !strings.Contains(output["html"].(string), fragment) {
			t.Fatalf("MCP omitted %q", fragment)
		}
	}
	report.Recommendation.Inspected = append(report.Recommendation.Inspected, report.Recommendation.Inspected[0])
	invalid, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "products_report_render", Arguments: core.ProductRecommendationReportRenderRequest{Report: report}})
	if err != nil || !invalid.IsError || invalid.StructuredContent != nil {
		t.Fatal("ambiguous duplicate decision accepted on MCP wire")
	}
}
