package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/products"
	"testing"
)

type accessWireSource struct {
	conclusionSource
	searches, details int
	failure           error
}

func (s *accessWireSource) Search(ctx context.Context, r core.ProductSearchRequest) ([]core.ProductCard, core.ProductCoverage, error) {
	s.searches++
	if s.searches == 2 {
		return nil, core.ProductCoverage{}, s.failure
	}
	return s.conclusionSource.Search(ctx, r)
}
func (s *accessWireSource) Inspect(ctx context.Context, r core.ProductInspectRequest) (core.ProductInspection, error) {
	s.details++
	return s.conclusionSource.Inspect(ctx, r)
}
func assertAccessWire(t *testing.T, result core.ProductRecommendationResult, source *accessWireSource, reason string) {
	t.Helper()
	if source.searches != 2 || source.details != 0 || result.Status != core.ProductRecommendationIncomplete || len(result.Discovered) != 1 || result.Audit.UninspectedExactOptions != 1 {
		t.Fatal("source stop or retained discovery changed across adapter")
	}
	if result.Audit.DiscoveryStopReason != reason || result.Audit.InspectionStopReason != reason || len(result.NextActions) != 1 || result.NextActions[0].Kind != "restore_source_access" || result.NextActions[0].Reason != reason {
		t.Fatal("recovery evidence changed across adapter")
	}
}

func TestCLIRecommendationAccessStopWire(t *testing.T) {
	for _, test := range []struct {
		reason string
		err    error
	}{{"source_access_denied", core.ErrBrowserAccessDenied}, {"source_authentication_required", core.ErrAuthenticationRequired}} {
		source := &accessWireSource{conclusionSource: conclusionSource{known: true}, failure: test.err}
		var out bytes.Buffer
		if err := runProducts(context.Background(), []string{"recommend", "--query", "synthetic", "--proceed", "--no-affiliate"}, &out, products.New(source)); err != nil {
			t.Fatal(err)
		}
		var result core.ProductRecommendationResult
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		assertAccessWire(t, result, source, test.reason)
	}
}
