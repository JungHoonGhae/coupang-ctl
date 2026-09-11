package products

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	coupangproducts "github.com/JungHoonGhae/coupang-ctl/internal/coupang/products"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
)

type recommendationSource struct {
	syntheticSource
	inspectionError error
	failedSort      core.ProductSort
}

type partialDocumentSource struct{ recommendationSource }

func (partialDocumentSource) Search(context.Context, core.ProductSearchRequest) ([]core.ProductCard, core.ProductCoverage, error) {
	return coupangproducts.ParseSearchDocument([]byte(`{"items":[
		{"product_id":"123","item_id":"456","vendor_item_id":"789","name":"Synthetic bowl","url":"https://www.coupang.com/vp/products/123","current_amount":1200,"sponsored":false,"observed_fields":["price.current_amount","sponsored"]},
		{"product_id":"124","name":"Synthetic missing price","url":"https://www.coupang.com/vp/products/124","observed_fields":["price.current_amount"]}
	]}`))
}

func TestRecommendPreservesValidSubsetWithoutHidingRejectedSourceCards(t *testing.T) {
	source := partialDocumentSource{recommendationSource: recommendationSource{syntheticSource: syntheticSource{inspection: core.ProductInspection{Product: syntheticRecommendationProduct()}}}}
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != core.ProductRecommendationIncomplete || result.Audit.DiscoveryStopReason != "source_read_incomplete" || len(result.Candidates) != 1 {
		t.Fatalf("partial parser output was dropped or claimed complete: status=%s stop=%s candidates=%d", result.Status, result.Audit.DiscoveryStopReason, len(result.Candidates))
	}
}

type syntheticPurchaseHistory struct {
	calls  int
	refs   []core.ProductReference
	err    error
	status string
}

func (p *syntheticPurchaseHistory) RecommendationPurchaseContext(_ context.Context, refs []core.ProductReference) (core.RecommendationPurchaseContext, error) {
	p.calls++
	p.refs = refs
	return core.RecommendationPurchaseContext{Visibility: "private_local", Status: p.status, Matches: []core.RecommendationPurchaseMatch{}}, p.err
}

func TestRecommendPurchaseHistoryRequiresOptInAndDoesNotRerank(t *testing.T) {
	product := syntheticRecommendationProduct()
	source := recommendationSource{syntheticSource: syntheticSource{items: []core.ProductCard{product}, inspection: core.ProductInspection{Product: product}}}
	history := &syntheticPurchaseHistory{status: "available"}
	service := New(source).WithPurchaseHistory(history)
	request := core.ProductRecommendationRequest{Query: "synthetic", Proceed: true}
	plain, err := service.Recommend(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if history.calls != 0 || plain.PurchaseContext != nil {
		t.Fatal("private history read without opt-in")
	}
	request.UsePurchaseHistory = true
	result, err := service.Recommend(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if history.calls != 1 || len(history.refs) != 1 || history.refs[0] != product.Reference {
		t.Fatal("history must receive final inspected candidates once")
	}
	if result.PurchaseContext == nil || result.PurchaseContext.Status != "available" || result.Status != core.ProductRecommendationComplete || result.Candidates[0].Product.Reference != plain.Candidates[0].Product.Reference || result.Candidates[0].Status != core.ProductRecommendationNeedsVerification {
		t.Fatal("history changed fit/ranking or was not returned")
	}
}

func TestRecommendPurchaseHistoryFailuresStayExplicitAndSanitized(t *testing.T) {
	product := syntheticRecommendationProduct()
	source := recommendationSource{syntheticSource: syntheticSource{items: []core.ProductCard{product}, inspection: core.ProductInspection{Product: product}}}
	for _, history := range []PurchaseHistoryRepository{nil, &syntheticPurchaseHistory{err: errors.New("sensitive-local-row")}, &syntheticPurchaseHistory{status: "partial"}} {
		result, err := New(source).WithPurchaseHistory(history).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, UsePurchaseHistory: true})
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != core.ProductRecommendationIncomplete || result.PurchaseContext == nil || len(result.Candidates) != 1 {
			t.Fatal("history failure disguised as complete or public evidence lost")
		}
		encoded, _ := json.Marshal(result)
		if strings.Contains(string(encoded), "sensitive-local-row") {
			t.Fatal("private error leaked")
		}
	}
}

func TestRecommendJoinsSQLitePurchaseEvidenceThroughPublicInterface(t *testing.T) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	if _, err := ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{{SourceRef: "synthetic-only", PurchasedAt: "2026-08-01", Currency: "KRW", Items: []core.OrderItem{{ProductID: "123", VendorItemID: "789", Name: "Synthetic bowl", Quantity: 2, PaidPrice: 2400}}}}}); err != nil {
		t.Fatal(err)
	}
	product := syntheticRecommendationProduct()
	source := recommendationSource{syntheticSource: syntheticSource{items: []core.ProductCard{product}, inspection: core.ProductInspection{Product: product}}}
	result, err := New(source).WithPurchaseHistory(ledger).Recommend(ctx, core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, UsePurchaseHistory: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.PurchaseContext == nil || len(result.PurchaseContext.Matches) != 1 || result.PurchaseContext.Matches[0].RetainedUnits != 2 || result.PurchaseContext.Sync.State != core.SyncRunNeverRun || result.Status != core.ProductRecommendationIncomplete {
		t.Fatalf("missing evidence or overstated coverage: %#v", result.PurchaseContext)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "synthetic-only") {
		t.Fatal("raw order identity leaked into recommendation")
	}
}

func (s recommendationSource) Search(ctx context.Context, request core.ProductSearchRequest) ([]core.ProductCard, core.ProductCoverage, error) {
	if request.Sort == s.failedSort {
		return nil, core.ProductCoverage{}, errors.New("sensitive-search-detail")
	}
	// This fixture models a finite one-page source. Its end is an explicit
	// source envelope, not an inference from the number of returned products.
	if request.Page > 1 || len(s.items) == 0 {
		return coupangproducts.ParseSearchDocument([]byte(`{"items":[],"no_results":true}`))
	}
	return s.syntheticSource.Search(ctx, request)
}

func (s recommendationSource) Inspect(context.Context, core.ProductInspectRequest) (core.ProductInspection, error) {
	return s.inspection, s.inspectionError
}

func syntheticRecommendationProduct() core.ProductCard {
	return core.ProductCard{Reference: core.ProductReference{ProductID: "123", ItemID: "456", VendorItemID: "789"},
		Name: "Synthetic bowl", URL: "https://www.coupang.com/vp/products/123?itemId=456&vendorItemId=789",
		// This synthetic fixture explicitly represents a known non-sponsored offer.
		Price: core.ProductPrice{CurrentAmount: 1200}, ObservedFields: []string{"price.current_amount", "sponsored"},
		SearchPosition: 1, RankSource: "coupang_search_order"}
}

func TestRecommendDoesNotClaimCompletionWhenEveryInspectionFails(t *testing.T) {
	source := recommendationSource{syntheticSource: syntheticSource{items: []core.ProductCard{syntheticRecommendationProduct()}},
		inspectionError: errors.New("sensitive-source-detail")}
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "incomplete" || len(result.Candidates) != 0 || len(result.Discovered) != 1 {
		t.Fatalf("failed inspection must preserve discovery without claiming complete: status=%s candidates=%d discovered=%d", result.Status, len(result.Candidates), len(result.Discovered))
	}
	if strings.Contains(strings.Join(result.Warnings, " "), "sensitive-source-detail") {
		t.Fatal("raw source error leaked")
	}
}

func TestRecommendRechecksBudgetAgainstInspectedPrice(t *testing.T) {
	product := withSyntheticPriceEvidence(syntheticRecommendationProduct())
	detail := product
	detail.Price.CurrentAmount = 5000
	source := recommendationSource{syntheticSource: syntheticSource{items: []core.ProductCard{product}, inspection: core.ProductInspection{Product: detail}}}
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, MaxPrice: 2000})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "no_matches" || len(result.Candidates) != 0 || result.Audit.DetailsInspected != 1 {
		t.Fatalf("over-budget inspected product returned: %s, %d candidates", result.Status, len(result.Candidates))
	}
}

func TestRecommendKeepsNoMatchesDistinctFromInspectionFailure(t *testing.T) {
	result, err := New(recommendationSource{}).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "no_matches" {
		t.Fatalf("status=%s", result.Status)
	}
}

func TestRecommendUnknownAdvertisingIsIncompleteNotNoMatches(t *testing.T) {
	product := syntheticRecommendationProduct()
	product.ObservedFields = []string{"price.current_amount"}
	result, err := New(recommendationSource{syntheticSource: syntheticSource{items: []core.ProductCard{product}}}).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != core.ProductRecommendationIncomplete || len(result.Discovered) != 1 || result.Audit.InspectionAttempts != 1 || result.Audit.DetailsInspected != 0 {
		t.Fatalf("unknown ad evidence was discarded or missing detail treated as success: status=%s discovered=%d", result.Status, len(result.Discovered))
	}
}

func TestRecommendReturnsObservedCandidateWithStableJSONEnvelope(t *testing.T) {
	product := withSyntheticPriceEvidence(syntheticRecommendationProduct())
	source := recommendationSource{syntheticSource: syntheticSource{items: []core.ProductCard{product}, inspection: core.ProductInspection{Product: product}}}
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, MaxPrice: 2000})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != core.ProductRecommendationComplete || len(result.Candidates) != 1 || result.Candidates[0].Product.Reference != product.Reference {
		t.Fatal("missing observed candidate")
	}
	if result.Candidates[0].Status != core.ProductRecommendationNeedsVerification {
		t.Fatal("category fit was asserted without evidence")
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"schema_version", "status", "query", "questions", "candidates", "discovered", "audit", "warnings"} {
		if _, exists := envelope[key]; !exists {
			t.Errorf("missing JSON key %s", key)
		}
	}
}

func TestRecommendReportsPartialSearchCoverageWithoutLosingInspectedCandidate(t *testing.T) {
	product := syntheticRecommendationProduct()
	source := recommendationSource{syntheticSource: syntheticSource{items: []core.ProductCard{product}, inspection: core.ProductInspection{Product: product}}, failedSort: core.ProductSortPriceAsc}
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != core.ProductRecommendationIncomplete || len(result.Candidates) != 1 || result.Audit.SearchesRun != 9 || result.Audit.SearchPagesRead != 8 {
		t.Fatalf("partial result: status=%s candidates=%d audit=%#v", result.Status, len(result.Candidates), result.Audit)
	}
	if result.Audit.DiscoveryStopReason != "source_read_incomplete" {
		t.Fatal("partial source coverage mislabeled as source exhaustion")
	}
}

func TestRecommendDoesNotTreatUnknownInspectedPriceAsWithinBudget(t *testing.T) {
	product := withSyntheticPriceEvidence(syntheticRecommendationProduct())
	detail := product
	detail.ObservedFields = nil
	result, err := New(recommendationSource{syntheticSource: syntheticSource{items: []core.ProductCard{product}, inspection: core.ProductInspection{Product: detail}}}).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, MaxPrice: 2000})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != core.ProductRecommendationIncomplete || len(result.Candidates) != 0 {
		t.Fatal("unknown detail price was treated as verified within budget")
	}
}
