package recommendationreport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/cli"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/mcpserver"
	"github.com/JungHoonGhae/coupang-ctl/internal/recommendationreport"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Exercise actual SQL exclusions and resumed scan evidence through both public
// adapters, using a disposable synthetic ledger and no browser/source providers.
func TestPurchaseReportSQLiteToCLIAndMCP(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	ledger, err := store.Open(ctx, filepath.Join(dir, "synthetic.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	item := func(vendor string, quantity int) core.OrderItem {
		return core.OrderItem{ProductID: "101", VendorItemID: vendor, Name: "Synthetic item", Quantity: quantity, PaidPrice: 1000, DeliveryStatus: "delivered"}
	}
	old := core.Order{SourceRef: "synthetic-order-old", PurchasedAt: "2026-06-01", Currency: "KRW", Items: []core.OrderItem{item("302", 4)}}
	if _, err := ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{old}}); err != nil {
		t.Fatal(err)
	}
	part := item("301", 4)
	part.CancelledQuantity, part.ReturnedQuantity = 1, 1
	excluded := item("301", 50)
	excluded.DeliveryStatus = "returned"
	a := core.Order{SourceRef: "synthetic-order-a", PurchasedAt: "2026-07-01", Currency: "KRW", Items: []core.OrderItem{part, excluded}}
	b := core.Order{SourceRef: "synthetic-order-b", PurchasedAt: "2026-08-01", Currency: "KRW", Items: []core.OrderItem{item("301", 1)}}
	unlock, err := ledger.AcquireSyncWriter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	next := &core.OrderCursor{Year: 2026, Page: 2}
	first, err := ledger.BeginResumableSync(ctx, core.SyncSourceCamofox, core.SyncProvenanceObservedStructuredOrderDocument, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.ApplySyncPage(ctx, first, nil, core.OrderPage{Orders: []core.Order{a}, Next: next}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.FinishSync(ctx, first, core.SyncResult{PagesProcessed: 1, OrdersSeen: 1}, ""); err != nil {
		t.Fatal(err)
	}
	second, err := ledger.BeginResumableSync(ctx, core.SyncSourceCamofox, core.SyncProvenanceObservedStructuredOrderDocument, next)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.ApplySyncPage(ctx, second, next, core.OrderPage{Orders: []core.Order{a, b}}); err != nil {
		t.Fatal(err)
	}
	if err := ledger.FinishSync(ctx, second, core.SyncResult{PagesProcessed: 1, OrdersSeen: 2}, ""); err != nil {
		t.Fatal(err)
	}
	refs := []core.ProductReference{{ProductID: "101", ItemID: "201", VendorItemID: "301"}, {ProductID: "101", ItemID: "202", VendorItemID: "302"}, {ProductID: "101"}}
	history, err := ledger.RecommendationPurchaseContext(ctx, refs)
	if err != nil {
		t.Fatal(err)
	}
	if history.Status != "partial" || len(history.Matches) != 3 || history.Matches[0].RetainedUnits != 3 || history.Matches[1].RetainedUnits != 4 || history.Matches[2].RetainedUnits != 7 || history.Sync.Scan.RetainedOrdersNotObserved != 1 {
		t.Fatal("SQL aggregate or retained scan scope changed")
	}
	shown := core.ProductRecommendationCandidate{Product: core.ProductCard{Reference: refs[0], Name: "Synthetic displayed option"}}
	hidden := core.ProductRecommendationCandidate{Product: core.ProductCard{Reference: refs[1], Name: "Synthetic hidden option"}}
	r := core.ProductRecommendationReport{Title: "Synthetic purchase report", Recommendation: core.ProductRecommendationResult{SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete, Candidates: []core.ProductRecommendationCandidate{shown}, Inspected: []core.ProductRecommendationCandidate{shown, hidden}, PurchaseContext: &history}}
	want, err := recommendationreport.RenderResult(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"Synthetic hidden option", "<dt>취소·반품 차감 수량</dt><dd>3개", "<dt>취소·반품 차감 수량</dt><dd>4개", "<dt>취소·반품 차감 수량</dt><dd>7개", "<dt>현재 로컬 주문 중 이 스캔에서 못 본 주문</dt><dd>1", "<dt>누적 저장 페이지</dt><dd>2", "<dt>마지막 시도에서 처리한 페이지</dt><dd>1", "전체 계정의 구매 이력 확보는 미검증"} {
		if !strings.Contains(want.HTML, text) {
			t.Fatalf("SQL evidence omitted from report: %q", text)
		}
	}
	if strings.Contains(want.HTML, "synthetic-order-") || strings.Contains(want.HTML, "2026-07-01") {
		t.Fatal("raw order identity or exact purchase date leaked into aggregate report")
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(dir, "report.json")
	if err := os.WriteFile(input, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COUPANGCTL_STATE_DIR", "deliberately-invalid-relative-state")
	var out, stderr bytes.Buffer
	if err := cli.Run(ctx, []string{"products", "report", "--input", input}, &out, &stderr, "test"); err != nil {
		t.Fatal(err)
	}
	var got core.ProductRecommendationReportRenderResult
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || got != want || stderr.Len() != 0 {
		t.Fatal("CLI purchase report diverged or initialized account state", err)
	}
	ct, st := mcp.NewInMemoryTransports()
	ss, err := mcpserver.NewWithProviders(mcpserver.Providers{}, "test").Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "products_report_render", Arguments: core.ProductRecommendationReportRenderRequest{Report: r}})
	if err != nil || result.IsError {
		t.Fatal("MCP purchase report failed", err)
	}
	data, err = json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &got); err != nil || got != want {
		t.Fatal("MCP purchase report diverged from SQL/CLI evidence", err)
	}
}
