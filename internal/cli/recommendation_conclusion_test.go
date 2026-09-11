package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/products"
	"testing"
	"time"
)

type conclusionSource struct{ known bool }

func (s conclusionSource) product() core.ProductCard {
	ref := core.ProductReference{ProductID: "123"}
	p := core.ProductCard{Reference: ref, Name: "Synthetic bowl", URL: "https://www.coupang.com/vp/products/123", Price: core.ProductPrice{CurrentAmount: 1200, Currency: "KRW"}, ObservedFields: []string{"sponsored", "price.current_amount"}}
	if s.known {
		p.FieldEvidence = []core.ProductFieldEvidence{{Field: "price.current_amount", Source: "json_ld", Locator: "synthetic.offer.price", Method: "native_field", Provenance: "observed", Scope: "product", Reference: ref, CapturedAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)}}
	}
	return p
}
func (s conclusionSource) Search(_ context.Context, r core.ProductSearchRequest) ([]core.ProductCard, core.ProductCoverage, error) {
	if r.Page > 1 {
		return []core.ProductCard{}, core.ProductCoverage{SourceNoResults: true}, nil
	}
	return []core.ProductCard{s.product()}, core.ProductCoverage{}, nil
}
func (s conclusionSource) Inspect(context.Context, core.ProductInspectRequest) (core.ProductInspection, error) {
	return core.ProductInspection{Product: s.product()}, nil
}
func (conclusionSource) AddToCart(context.Context, core.CartAddRequest) (core.CartAddResult, error) {
	return core.CartAddResult{}, errors.New("unexpected synthetic write")
}

func TestCLIRecommendationServiceConclusionsAndActions(t *testing.T) {
	for _, known := range []bool{false, true} {
		var out bytes.Buffer
		service := products.New(conclusionSource{known: known})
		err := runProducts(context.Background(), []string{"recommend", "--query", "synthetic", "--proceed", "--conditions-json", `[{"id":"budget","field":"price.current_amount","operator":"lte","integer":2000}]`}, &out, service)
		if err != nil {
			t.Fatal(err)
		}
		var result core.ProductRecommendationResult
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatal(err)
		}

		if len(result.Inspected) != 1 || result.Inspected[0].Conclusion == nil {
			t.Fatal("service conclusion absent on adapter wire")
		}
		c := result.Inspected[0]
		if c.Status != core.ProductRecommendationNeedsVerification || c.Conclusion.Scope != "declared_conditions_on_observed_reference" {
			t.Fatal("scope became overall fitness")
		}
		if known {
			if c.Conclusion.Outcome != "conditions_met" || len(c.Conclusion.SupportingConditionIDs) != 1 || c.Conclusion.SupportingConditionIDs[0] != "budget" || len(result.Candidates) != 1 || result.Status != core.ProductRecommendationComplete || len(result.NextActions) != 0 {
				t.Fatal("known scoped conclusion changed on wire")
			}
		} else {
			if c.Conclusion.Outcome != "conditions_unverified" || len(result.Candidates) != 0 || result.Status != core.ProductRecommendationIncomplete || len(result.NextActions) != 1 {
				t.Fatal("unknown condition promoted or next action missing")
			}
			action := result.NextActions[0]
			if action.Kind != "inspect_evidence" || len(action.References) != 1 || action.References[0].ProductID != "123" || len(action.ConditionIDs) != 1 || action.ConditionIDs[0] != "budget" || len(action.Fields) != 1 || action.Fields[0] != "price.current_amount" {
				t.Fatal("targeted action changed on wire")
			}
		}

	}
}
