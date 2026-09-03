package products

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestParseSearchDocumentAcceptsOnlyCanonicalPublicProductData(t *testing.T) {
	document := []byte(`{
		"items":[
			{"product_id":"101","item_id":"201","vendor_item_id":"301","name":"Synthetic USB hub","url":"https://www.coupang.com/vp/products/101?itemId=201&vendorItemId=301","image_url":"https://thumbnail.coupangcdn.com/example.jpg","current_amount":79000,"rating":4.8,"review_count":123,"rocket":true,"search_position":7,"rank_source":"coupang_search_order","review_scope":"product_page_observed","observed_fields":["price.current_amount","rating","review_count","search_position"]},
			{"product_id":"999","name":"External","url":"https://example.com/vp/products/999"}
		],
		"coverage":{"source":"synthetic","observed_fields":["name","price"]}
	}`)
	items, coverage, err := ParseSearchDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Reference.VendorItemID != "301" || items[0].Price.Currency != "KRW" || items[0].SearchPosition != 7 || items[0].RankSource != "coupang_search_order" || items[0].ReviewScope != "product_page_observed" || coverage.Source != "synthetic" {
		t.Fatalf("unexpected search parse: %#v %#v", items, coverage)
	}
}

func TestParseSearchDocumentAcceptsExplicitNoResults(t *testing.T) {
	items, _, err := ParseSearchDocument([]byte(`{"items":[],"no_results":true,"coverage":{"source":"synthetic"}}`))
	if err != nil || len(items) != 0 {
		t.Fatalf("explicit empty search was rejected: items=%#v err=%v", items, err)
	}
}

func TestParseInspectionRedactsObviousReviewerPIIAndOmitsUntrustedImages(t *testing.T) {
	document := []byte(`{
		"product":{"product_id":"101","vendor_item_id":"301","name":"Synthetic hub","url":"https://www.coupang.com/vp/products/101?vendorItemId=301","current_amount":79000,"observed_fields":["price.current_amount"]},
		"selected_options":["16GB / 512GB","16GB / 512GB"],
		"gallery_images":["https://thumbnail.coupangcdn.com/example.jpg","https://tracker.example/image.jpg"],
		"reviews":[{"rating":5,"content":"연락은 buyer@example.com 또는 010-1234-5678","image_urls":[]}],
		"coverage":{"source":"synthetic"}
	}`)
	result, err := ParseInspectionDocument(document, core.ProductInspectRequest{ProductID: "101", VendorItemID: "301"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.GalleryImages) != 1 || len(result.Reviews) != 1 || len(result.SelectedOptions) != 1 || result.SelectedOptions[0] != "16GB / 512GB" {
		t.Fatalf("unexpected inspection parse: %#v", result)
	}
	if strings.Contains(result.Reviews[0].Content, "buyer@example.com") || strings.Contains(result.Reviews[0].Content, "010-1234-5678") {
		t.Fatalf("review PII was not redacted: %q", result.Reviews[0].Content)
	}
}

func TestParseInspectionMakesMissingOptionAndCardBenefitCoverageExplicit(t *testing.T) {
	document := []byte(`{
		"product":{"product_id":"101","vendor_item_id":"301","name":"Synthetic hub","url":"https://www.coupang.com/vp/products/101?vendorItemId=301","current_amount":79000},
		"selected_options":[],
		"benefits":[{"kind":"coupon","title":"Synthetic coupon","source":"product_page"}],
		"coverage":{"source":"synthetic","observed_fields":["selected_options"],"unavailable_fields":[]}
	}`)
	result, err := ParseInspectionDocument(document, core.ProductInspectRequest{ProductID: "101", VendorItemID: "301"})
	if err != nil {
		t.Fatal(err)
	}
	if containsString(result.Coverage.ObservedFields, "selected_options") {
		t.Fatalf("missing option labels remained observed: %#v", result.Coverage)
	}
	for _, field := range []string{"selected_options", "card_benefit"} {
		if !containsString(result.Coverage.UnavailableFields, field) {
			t.Fatalf("missing %q coverage: %#v", field, result.Coverage)
		}
	}
}

func TestParseInspectionKeepsObservedEvidenceAtCoverageLimit(t *testing.T) {
	observed := make([]string, 80)
	for index := range observed {
		observed[index] = "a_source_field_" + strconv.Itoa(index)
	}
	document, err := json.Marshal(map[string]any{
		"product": map[string]any{
			"product_id": "101", "vendor_item_id": "301", "name": "Synthetic hub",
			"url": "https://www.coupang.com/vp/products/101?vendorItemId=301", "current_amount": 79000,
		},
		"selected_options": []string{"Synthetic 16GB option"},
		"benefits":         []map[string]any{{"kind": "card", "title": "Synthetic card benefit", "source": "product_page"}},
		"coverage": map[string]any{
			"source": "synthetic", "observed_fields": observed,
			"unavailable_fields": []string{"selected_options", "card_benefit"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := ParseInspectionDocument(document, core.ProductInspectRequest{ProductID: "101", VendorItemID: "301"})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"selected_options", "card_benefit"} {
		if !containsString(result.Coverage.ObservedFields, field) || containsString(result.Coverage.UnavailableFields, field) {
			t.Fatalf("observed %q evidence was lost or contradictory at the coverage limit: %#v", field, result.Coverage)
		}
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func TestParseCartAddPreservesUnverifiedAttemptWithoutEncouragingRetry(t *testing.T) {
	request := core.CartAddRequest{ProductID: "101", VendorItemID: "301", Quantity: 2, Confirmed: true}
	result, err := ParseCartAddDocument([]byte(`{"attempted":true,"added":false,"quantity":2,"cart_url":"https://cart.coupang.com/cartView.pang","source":"coupang_product_page"}`), request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Attempted || result.Verified || len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "do not retry") {
		t.Fatalf("unexpected unverified cart response: %#v", result)
	}
}
