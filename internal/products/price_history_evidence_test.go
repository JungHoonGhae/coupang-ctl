package products

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func withSyntheticObservationEvidence(o core.ProductPriceObservation) core.ProductPriceObservation {
	scope := "product"
	if o.Reference.ItemID != "" || o.Reference.VendorItemID != "" {
		scope = "selected_option"
	}
	o.FieldEvidence = []core.ProductFieldEvidence{{Field: "price.current_amount", Source: "json_ld", Locator: "jsonld.Product.offers.price", Method: "native_field", Provenance: "observed", Scope: scope, Reference: o.Reference, CapturedAt: o.ObservedAt}}
	return o
}

func TestPriceRecordingPreservesDerivedZeroAndDoesNotInventEvidence(t *testing.T) {
	p := withSyntheticPriceEvidence(syntheticRecommendationProduct())
	p.Price.CurrentAmount = 0
	p.FieldEvidence[0].Source = "dom"
	p.FieldEvidence[0].Method = "numeric_parse"
	p.FieldEvidence[0].Provenance = "derived"
	p.Price.OriginalAmount = 9999
	p.Price.DiscountRate = 50
	unknown := p
	unknown.FieldEvidence = nil
	history := &syntheticPriceHistory{}
	s := NewWithAffiliateAndPrices(syntheticSource{items: []core.ProductCard{p, unknown}}, nil, history)
	_, err := s.Search(context.Background(), core.ProductSearchRequest{Query: "synthetic", IncludeVariants: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(history.recorded) != 1 {
		t.Fatalf("records=%d", len(history.recorded))
	}
	o := history.recorded[0]
	if o.CurrentAmount != 0 || o.Provenance != "derived" || !o.ObservedAt.Equal(p.FieldEvidence[0].CapturedAt) || !o.HasPriceEvidence() {
		t.Fatal("derived zero evidence was lost")
	}
	if o.OriginalAmount != 0 || o.DiscountRate != 0 {
		t.Fatal("unverified optional amounts were persisted")
	}
}

func TestPriceTrendDoesNotInventLegacyEvidenceOrZeroBaselinePercentage(t *testing.T) {
	o := withSyntheticObservationEvidence(core.ProductPriceObservation{Reference: core.ProductReference{ProductID: "101"}, Currency: "KRW", Source: "coupang_product_search", Provenance: "observed", ObservedAt: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)})
	next := o
	next.CurrentAmount = 100
	trend := priceTrend([]core.ProductPriceObservation{o, next})
	if trend == nil || trend.ChangeFromFirstReturnedKRW != 100 || trend.ChangeFromFirstReturnedPercent != nil {
		t.Fatal("zero baseline was assigned a percentage")
	}
	data, err := json.Marshal(trend)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	json.Unmarshal(data, &fields)
	if _, ok := fields["change_from_first_returned_percent"]; ok {
		t.Fatal("undefined percent serialized")
	}
	next.FieldEvidence = nil
	if priceTrend([]core.ProductPriceObservation{o, next}) != nil {
		t.Fatal("legacy row manufactured a verified trend")
	}
	if watchedIdentityMatches(core.ProductReference{ProductID: "101"}, core.ProductReference{ProductID: "101", ItemID: "201"}) {
		t.Fatal("option price could replace product watch")
	}
}

func TestWatchRefreshPreservesDerivedZeroAndRejectsChangedOption(t *testing.T) {
	for _, wrongOption := range []bool{false, true} {
		p := withSyntheticPriceEvidence(syntheticRecommendationProduct())
		p.Price.CurrentAmount = 0
		p.FieldEvidence[0].Source = "dom"
		p.FieldEvidence[0].Method = "numeric_parse"
		p.FieldEvidence[0].Provenance = "derived"
		ref := p.Reference
		if wrongOption {
			p.Reference.ItemID = "999"
			p.FieldEvidence[0].Reference = p.Reference
		}
		history := &syntheticPriceHistory{watches: []core.ProductWatchEntry{{Reference: ref}}}
		s := NewWithAffiliateAndPrices(syntheticSource{inspection: core.ProductInspection{Product: p}}, nil, history)
		result, err := s.RefreshPriceWatches(context.Background(), core.ProductWatchRefreshRequest{Limit: 1})
		if err != nil {
			t.Fatal(err)
		}
		if wrongOption {
			if result.Unavailable != 1 || len(history.recorded) != 0 {
				t.Fatal("changed option recorded")
			}
		} else if result.Observed != 1 || len(history.recorded) != 1 || history.recorded[0].CurrentAmount != 0 || result.Items[0].Provenance != "derived" {
			t.Fatal("watch lost derived zero")
		}
	}
}
