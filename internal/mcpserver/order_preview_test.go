package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/orders"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type orderPreviewSource struct {
	page  core.OrderPage
	err   error
	calls int
}

func (s *orderPreviewSource) FetchPage(_ context.Context, cursor *core.OrderCursor) (core.OrderPage, error) {
	s.calls++
	if cursor != nil {
		return core.OrderPage{}, errors.New("unexpected cursor")
	}
	return s.page, s.err
}

func TestOrderPreviewMCPReadsOnePrivatePageAndPreservesFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		page core.OrderPage
		err  error
		code string
	}{
		{"one_page", core.OrderPage{Orders: []core.Order{{SourceRef: core.OrderSourceReference("synthetic-preview"), PurchasedAt: "2026-09-01", TotalAmount: 1000, Currency: "KRW"}}, Next: &core.OrderCursor{Year: 2026, Page: 1}}, nil, ""},
		{"empty", core.OrderPage{Orders: []core.Order{}}, nil, ""},
		{"missing", core.OrderPage{}, nil, "invalid_order_data"},
		{"partial", core.OrderPage{}, core.ErrPartialOrderData, "partial_order_data"},
		{"auth", core.OrderPage{}, core.ErrAuthenticationRequired, "camofox_authentication_required"},
		{"blocked", core.OrderPage{}, core.ErrBrowserAccessDenied, "camofox_access_denied"},
		{"timeout", core.OrderPage{}, context.DeadlineExceeded, "operation_timed_out"},
		{"cancelled", core.OrderPage{}, context.Canceled, "operation_cancelled"},
		{"unknown", core.OrderPage{}, errors.New("synthetic-private-detail"), "document_source_unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := &orderPreviewSource{page: tc.page, err: tc.err}
			if tc.err != nil {
				source.err = errors.Join(tc.err, errors.New("synthetic-private-detail"))
			}
			service := orders.NewWithPageSourceAndSyncSource(nil, source, core.SyncSourceCamofox)
			factoryCalls := 0
			ctx, cs := connectFactoryTest(t, ProviderFactories{OrderPreview: func(context.Context, string) (OrderPreviewProvider, error) { factoryCalls++; return service, nil }})
			list, err := cs.ListTools(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, tool := range list.Tools {
				if tool.Name != "orders_preview" {
					continue
				}
				found = true
				if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || *tool.Annotations.DestructiveHint || !*tool.Annotations.OpenWorldHint {
					t.Fatal("preview annotations lost read scope")
				}
			}
			if !found || factoryCalls != 0 {
				t.Fatal("preview discovery not lazy")
			}
			invalid, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "orders_preview", Arguments: map[string]any{"max_pages": 2}})
			if err != nil || !invalid.IsError || factoryCalls != 0 {
				t.Fatal("invalid preview input acquired source")
			}
			result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "orders_preview", Arguments: struct{}{}})
			if err != nil || source.calls != 1 {
				t.Fatal("preview did not make exactly one read")
			}
			if tc.code != "" {
				response := toolErrorJSON(t, result)
				if response.Error.Code != tc.code || response.Error.Operation != "orders_preview" {
					t.Fatal("preview failure escaped or became success")
				}
				return
			}
			data, _ := json.Marshal(result.StructuredContent)
			var got core.OrderPreview
			if result.IsError || json.Unmarshal(data, &got) != nil || got.Orders == nil || got.OrderCount != len(tc.page.Orders) || got.HasNextPage != (tc.page.Next != nil) || got.HistoryComplete || got.AccountIdentityVerified || got.Persisted || got.CapturedAt.IsZero() || got.Visibility != "private_local" || got.Scope != "source_entry_page" {
				t.Fatal("MCP changed normalized preview contract")
			}
		})
	}
}
