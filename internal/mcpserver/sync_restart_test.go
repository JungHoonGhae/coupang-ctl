package mcpserver_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/mcpserver"
	"github.com/JungHoonGhae/coupang-ctl/internal/orders"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type restartPageSource struct{ cursors []*core.OrderCursor }

func (s *restartPageSource) FetchPage(_ context.Context, cursor *core.OrderCursor) (core.OrderPage, error) {
	s.cursors = append(s.cursors, cursor)
	return core.OrderPage{Orders: []core.Order{}, Next: &core.OrderCursor{Year: 2026, Page: 2}}, nil
}

func TestMCPSyncDeliversExplicitRestartToRealService(t *testing.T) {
	for _, name := range []string{"orders_sync"} {
		for _, restart := range []bool{false, true} {
			ctx := context.Background()
			ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { ledger.Close() })
			source := &restartPageSource{}
			service := orders.NewWithPageSource(ledger, source)
			if _, err := service.Sync(ctx, core.SyncRequest{MaxPages: 1}); err != nil {
				t.Fatal(err)
			}
			server := mcpserver.NewWithProviders(mcpserver.Providers{Orders: service}, "test")
			clientTransport, serverTransport := mcp.NewInMemoryTransports()
			serverSession, err := server.Connect(ctx, serverTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { serverSession.Close() })
			client := mcp.NewClient(&mcp.Implementation{Name: "synthetic", Version: "test"}, nil)
			clientSession, err := client.Connect(ctx, clientTransport, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { clientSession.Close() })
			listed, err := clientSession.ListTools(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, tool := range listed.Tools {
				if tool.Name == name {
					found = true
					if tool.Annotations != nil && tool.Annotations.IdempotentHint {
						t.Fatal("restart-capable sync must not promise retry idempotency")
					}
				}
			}
			if !found {
				t.Fatal("sync tool missing")
			}
			result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: map[string]any{"max_pages": 1, "restart_scan": restart}})
			if err != nil || result.IsError {
				t.Fatalf("%s restart=%v failed: %v", name, restart, err)
			}
			if len(source.cursors) != 2 || (source.cursors[1] == nil) != restart {
				t.Fatalf("%s changed restart/resume semantics", name)
			}
		}
	}
}
