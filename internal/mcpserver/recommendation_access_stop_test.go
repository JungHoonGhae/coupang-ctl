package mcpserver

import (
	"context"
	"encoding/json"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/products"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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

func TestMCPRecommendationAccessStopWire(t *testing.T) {
	for _, test := range []struct {
		reason string
		err    error
	}{{"source_access_denied", core.ErrBrowserAccessDenied}, {"source_authentication_required", core.ErrAuthenticationRequired}} {
		t.Run(test.reason, func(t *testing.T) {
			ctx := context.Background()
			source := &accessWireSource{conclusionSource: conclusionSource{known: true}, failure: test.err}
			server := NewWithFeatures(fixedStatusProvider{}, nil, products.New(source), "test")
			ct, st := mcp.NewInMemoryTransports()
			ss, err := server.Connect(ctx, st, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer ss.Close()
			client := mcp.NewClient(&mcp.Implementation{Name: "access-test", Version: "test"}, nil)
			cs, err := client.Connect(ctx, ct, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer cs.Close()
			response, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "products_recommend", Arguments: map[string]any{"query": "synthetic", "proceed": true, "disable_affiliate": true}})
			if err != nil {
				t.Fatal(err)
			}
			if response.IsError {
				t.Fatal("partial recommendation became tool failure")
			}
			encoded, err := json.Marshal(response.StructuredContent)
			if err != nil {
				t.Fatal(err)
			}
			var result core.ProductRecommendationResult
			if err := json.Unmarshal(encoded, &result); err != nil {
				t.Fatal(err)
			}
			assertAccessWire(t, result, source, test.reason)
		})
	}
}
