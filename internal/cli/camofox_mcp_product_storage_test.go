package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Substitute only the external runtime executable. Calls still cross the real
// CLI bootstrap, MCP stdio, product service and document parser. No site access.
func syntheticCamofoxSearchRuntime(t *testing.T, state, response string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX executable fixture")
	}
	node := filepath.Join(t.TempDir(), "synthetic-node")
	if strings.Contains(response, "'") || !json.Valid([]byte(response)) {
		t.Fatal("invalid synthetic response")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(response)); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nwhile IFS= read -r line; do :; done\nprintf '%s\\n' 'COUPANGCTL_RESULT " + compact.String() + "'\n"
	if err := os.WriteFile(node, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := browser.CamofoxConfig{SchemaVersion: 1, Node: node, Server: filepath.Join(state, "absent-server"), EngineDir: filepath.Join(state, "absent-engine"), UserID: "synthetic"}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(state, "camofox.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCamofoxMCPPublicSearchSurvivesUnavailableLocalHistory(t *testing.T) {
	state := t.TempDir()
	syntheticCamofoxSearchRuntime(t, state, `{"result":{"status":"ok","search":{"items":[],"no_results":true}}}`)
	db := filepath.Join(state, "camofox-coupangctl.sqlite3")
	legacy := []byte("synthetic unavailable legacy data")
	if err := os.WriteFile(db, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, session := startCamofoxMCPTest(t, state)
	for _, tool := range []string{"products_search", "products_recommend"} {
		result := callCamofoxMCPTest(t, ctx, session, tool, map[string]any{"query": "synthetic", "disable_affiliate": true})
		if result.IsError {
			t.Fatalf("%s was blocked by the unavailable local history", tool)
		}
		data, _ := json.Marshal(result.StructuredContent)
		if tool == "products_search" {
			var search core.ProductSearchResult
			if json.Unmarshal(data, &search) != nil || !search.Coverage.SourceNoResults || len(search.Items) != 0 {
				t.Fatal("source-declared empty search lost its evidence")
			}
		} else {
			var recommendation core.ProductRecommendationResult
			if json.Unmarshal(data, &recommendation) != nil || recommendation.Status != core.ProductRecommendationNoMatches || recommendation.PurchaseContext != nil {
				t.Fatal("public recommendation acquired private purchase history")
			}
		}
	}
	personal := callCamofoxMCPTest(t, ctx, session, "products_recommend", core.ProductRecommendationRequest{Query: "synthetic", DisableAffiliate: true, UsePurchaseHistory: true})
	personalData, _ := json.Marshal(personal.StructuredContent)
	var recommendation core.ProductRecommendationResult
	if personal.IsError || json.Unmarshal(personalData, &recommendation) != nil || recommendation.PurchaseContext == nil || recommendation.PurchaseContext.Status != "unavailable" || recommendation.Status != core.ProductRecommendationIncomplete {
		t.Fatal("unavailable requested purchase history was skipped or represented as complete")
	}
	local := callCamofoxMCPTest(t, ctx, session, "orders_stats", struct{}{})
	data, _ := json.Marshal(local.Content)
	if !local.IsError || !bytes.Contains(data, []byte("local_data_unavailable")) {
		t.Fatal("unavailable order data was silently treated as empty history")
	}
	got, err := os.ReadFile(db)
	if err != nil || !bytes.Equal(got, legacy) {
		t.Fatal("existing unavailable history was replaced")
	}
}

const syntheticPricedSearch = `{"result":{"status":"ok","search":{"items":[{
"product_id":"101","item_id":"201","vendor_item_id":"301","name":"Synthetic plate",
"url":"https://www.coupang.com/vp/products/101?itemId=201&vendorItemId=301",
"current_amount":12000,"currency":"KRW","observed_fields":["name","search_position","price.current_amount"],
"search_position":1,"rank_source":"dom_search_list_order","field_evidence":[{
"field":"price.current_amount","source":"dom","locator":"dom.search_card.price","method":"numeric_parse",
"scope":"selected_option","provenance":"derived","captured_at":"2026-09-01T00:00:00Z",
"reference":{"product_id":"101","item_id":"201","vendor_item_id":"301"}}]}]}}}`

func TestCamofoxMCPPricePersistenceIsOptionalButPreserved(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		t.Run(map[bool]string{false: "writable", true: "unavailable"}[unavailable], func(t *testing.T) {
			state := t.TempDir()
			syntheticCamofoxSearchRuntime(t, state, syntheticPricedSearch)
			db := filepath.Join(state, "camofox-coupangctl.sqlite3")
			legacy := []byte("synthetic unavailable legacy data")
			if unavailable {
				if err := os.WriteFile(db, legacy, 0600); err != nil {
					t.Fatal(err)
				}
			}
			ctx, session := startCamofoxMCPTest(t, state)
			result := callCamofoxMCPTest(t, ctx, session, "products_search", core.ProductSearchRequest{Query: "synthetic", Limit: 1, DisableAffiliate: true})
			data, _ := json.Marshal(result.StructuredContent)
			var search core.ProductSearchResult
			if result.IsError || json.Unmarshal(data, &search) != nil || len(search.Items) != 1 || search.Items[0].Price.CurrentAmount != 12000 {
				t.Fatal("local persistence failure erased current source evidence")
			}
			warned := strings.Contains(strings.Join(search.Warnings, " "), "could not be added to local price history")
			if warned != unavailable || bytes.Contains(data, []byte(state)) || bytes.Contains(data, legacy) {
				t.Fatal("persistence outcome was hidden or leaked storage details")
			}
			history := callCamofoxMCPTest(t, ctx, session, "product_price_history", core.ProductPriceHistoryRequest{ProductID: "101", VendorItemID: "301"})
			if unavailable {
				if !history.IsError {
					t.Fatal("unavailable price history was represented as empty")
				}
				got, err := os.ReadFile(db)
				if err != nil || !bytes.Equal(got, legacy) {
					t.Fatal("unavailable ledger was replaced")
				}
			} else {
				data, _ = json.Marshal(history.StructuredContent)
				var prices core.ProductPriceHistory
				if history.IsError || json.Unmarshal(data, &prices) != nil || prices.ObservationCount != 1 || prices.SeriesCount != 1 {
					t.Fatal("successful search no longer records exact-option price evidence")
				}
			}
		})
	}
}

func TestCamofoxMCPPurchaseContextRemainsExplicit(t *testing.T) {
	for _, useHistory := range []bool{false, true} {
		t.Run(map[bool]string{false: "public", true: "purchase_aware"}[useHistory], func(t *testing.T) {
			state := t.TempDir()
			syntheticCamofoxSearchRuntime(t, state, `{"result":{"status":"ok","search":{"items":[],"no_results":true}}}`)
			ctx, session := startCamofoxMCPTest(t, state)
			result := callCamofoxMCPTest(t, ctx, session, "products_recommend", core.ProductRecommendationRequest{Query: "synthetic", DisableAffiliate: true, UsePurchaseHistory: useHistory})
			data, _ := json.Marshal(result.StructuredContent)
			var recommendation core.ProductRecommendationResult
			if result.IsError || json.Unmarshal(data, &recommendation) != nil {
				t.Fatal("recommendation failed")
			}
			_, err := os.Stat(filepath.Join(state, "camofox-coupangctl.sqlite3"))
			if useHistory {
				if err != nil || recommendation.PurchaseContext == nil || recommendation.PurchaseContext.Status != "partial" || recommendation.Status != core.ProductRecommendationIncomplete {
					t.Fatal("requested purchase evidence was skipped or treated as complete")
				}
			} else if !os.IsNotExist(err) || recommendation.PurchaseContext != nil {
				t.Fatal("public recommendation acquired a purchase ledger without observed prices")
			}
		})
	}
}

func TestCamofoxMCPSourceFailureDoesNotAcquireProductStorage(t *testing.T) {
	state := t.TempDir()
	ctx, session := startCamofoxMCPTest(t, state)
	for _, tc := range []struct {
		tool     string
		args     any
		response string
	}{
		{"products_search", core.ProductSearchRequest{Query: "synthetic", DisableAffiliate: true}, `{"result":{"status":"access_denied"}}`},
		{"products_recommend", core.ProductRecommendationRequest{Query: "synthetic", DisableAffiliate: true, UsePurchaseHistory: true}, `{"result":{"status":"access_denied"}}`},
		// The detail reader returns the document directly; only sidebar search
		// wraps it in result alongside facets. Exercise the actual contracts.
		{"product_inspect", core.ProductInspectRequest{ProductID: "101", DisableAffiliate: true}, `{"status":"access_denied"}`},
	} {
		syntheticCamofoxSearchRuntime(t, state, tc.response)
		result := callCamofoxMCPTest(t, ctx, session, tc.tool, tc.args)
		if !result.IsError || len(result.Content) != 1 || result.StructuredContent != nil {
			t.Fatal("source failure became a success")
		}
		text, ok := result.Content[0].(*mcp.TextContent)
		var response core.ErrorResponse
		if !ok || json.Unmarshal([]byte(text.Text), &response) != nil || response.Error.Reason != "access_denied" || response.Error.Operation != tc.tool || response.Error.Retryable {
			t.Fatalf("%s replaced the source failure with a storage outcome", tc.tool)
		}
		if _, err := os.Stat(filepath.Join(state, "camofox-coupangctl.sqlite3")); !os.IsNotExist(err) {
			t.Fatal("failed source read created or opened product storage")
		}
	}
}
