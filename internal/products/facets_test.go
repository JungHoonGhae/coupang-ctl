package products

import (
	"context"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"strings"
	"testing"
)

func TestSearchDoesNotPresentSelectedFacetAsVariantEvidence(t *testing.T) {
	source := newRefinementSource(func(r core.ProductSearchRequest) []core.ProductFacet {
		return markedCatalog([]core.ProductFacet{{Name: "Memory", Options: []core.ProductFacetOption{{Label: "32 GB"}}}}, r.FacetSelections)
	})
	source.items[0].Name = "Synthetic desktop, 8 GB option"
	result, err := New(source).Search(context.Background(), core.ProductSearchRequest{Query: "synthetic", FacetSelections: []core.ProductFacetSelection{{Name: "Memory", Label: "32 GB"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 || !strings.Contains(strings.Join(result.Warnings, " "), "not evidence that each returned option satisfies") {
		t.Fatal("selected sidebar was presented without its per-option evidence limitation")
	}
}

type facetSource struct {
	syntheticSource
	selected bool
}

func (f *facetSource) Search(context.Context, core.ProductSearchRequest) ([]core.ProductCard, core.ProductCoverage, error) {
	return nil, core.ProductCoverage{SourceNoResults: true, Facets: []core.ProductFacet{{Name: "Memory", Options: []core.ProductFacetOption{{Label: "32 GB", Selected: f.selected}}}}}, nil
}
func TestSearchRequiresVerifiedSourceFacet(t *testing.T) {
	for _, selected := range []bool{false, true} {
		_, err := New(&facetSource{selected: selected}).Search(context.Background(), core.ProductSearchRequest{Query: "desktop", FacetSelections: []core.ProductFacetSelection{{Name: "Memory", Label: "32 GB"}}})
		if (err == nil) != selected {
			t.Fatalf("unverified filter accepted: %v", selected)
		}
	}
}
