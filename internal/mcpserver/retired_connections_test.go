package mcpserver

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPAdvertisesOneSyncPathWithoutRetiredBrowserConnections(t *testing.T) {
	ctx := context.Background()
	server := NewWithFeatures(fixedStatusProvider{}, fixedOrderProvider{}, fixedProductProvider{}, "test")
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "synthetic", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()
	listed, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for _, tool := range listed.Tools {
		seen[tool.Name] = true
	}
	for _, name := range []string{"auth_status", "orders_sync", "orders_sync_status", "products_search", "product_inspect"} {
		if !seen[name] {
			t.Fatalf("active tool missing: %s", name)
		}
	}
	for _, name := range []string{"current_browser_status", "orders_sync_current_browser", "orders_sync_ordinary_browser"} {
		if seen[name] {
			t.Fatalf("retired connection advertised: %s", name)
		}
		result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: name})
		if err == nil && (result == nil || !result.IsError) {
			t.Fatalf("retired connection accepted: %s", name)
		}
	}
}
