package products

import (
	"context"
	"errors"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestInspectRejectsWrongIdentityBeforeSideEffects(t *testing.T) {
	request := core.ProductInspectRequest{ProductID: "123", ItemID: "456", VendorItemID: "789"}
	for _, ref := range []core.ProductReference{
		{},
		{ProductID: "999", ItemID: "456", VendorItemID: "789"},
		{ProductID: "123", VendorItemID: "789"},
		{ProductID: "123", ItemID: "457", VendorItemID: "789"},
		{ProductID: "123", ItemID: "456"},
		{ProductID: "123", ItemID: "456", VendorItemID: "790"},
	} {
		p := withSyntheticPriceEvidence(syntheticRecommendationProduct())
		p.Reference = ref
		linker := &syntheticAffiliateLinker{}
		history := &syntheticPriceHistory{}
		service := NewWithAffiliate(syntheticSource{inspection: core.ProductInspection{Product: p}}, linker)
		service.priceHistory = history
		result, err := service.Inspect(context.Background(), request)
		if !errors.Is(err, ErrProductIdentityMismatch) || !errors.Is(err, ErrSourceUnavailable) {
			t.Fatalf("wrong reference accepted: %v", err)
		}
		if result.Product.Reference.ProductID != "" || linker.calls != 0 || len(history.recorded) != 0 {
			t.Fatal("mismatched identity escaped or caused side effects")
		}
	}
}

func TestInspectProductOnlyAllowsDiscoveredNumericOption(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		p := syntheticRecommendationProduct()
		if invalid {
			p.Reference.ItemID = "not-numeric"
		}
		result, err := New(syntheticSource{inspection: core.ProductInspection{Product: p}}).Inspect(context.Background(), core.ProductInspectRequest{ProductID: "123"})
		if invalid {
			if !errors.Is(err, ErrProductIdentityMismatch) {
				t.Fatal("invalid returned option identity accepted")
			}
		} else if err != nil || result.Product.Reference != p.Reference {
			t.Fatal("product-only inspection lost a valid source-selected option")
		}
	}
}
