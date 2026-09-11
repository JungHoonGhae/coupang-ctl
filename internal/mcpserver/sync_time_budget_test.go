package mcpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/cli"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/mcpserver"
	"github.com/JungHoonGhae/coupang-ctl/internal/orders"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type interruptedSyncProvider struct {
	*orders.Service
	err error
}

func (p interruptedSyncProvider) Sync(context.Context, core.SyncRequest) (core.SyncResult, error) {
	return core.SyncResult{SchemaVersion: core.SyncResultSchemaVersion, PagesProcessed: 1}, p.err
}

func TestSyncInterruptionErrorsReachCLIAndMCPWithoutRawCauses(t *testing.T) {
	for _, tc := range []struct {
		cause error
		code  string
	}{
		{core.ErrSyncTimeBudget, "time_budget_exhausted"},
		{core.ErrSyncPageDeadline, "page_deadline_exceeded"},
		{core.ErrSyncInProgress, "sync_in_progress"},
		{core.ErrSyncCursorLoop, "cursor_loop"},
		{core.ErrPartialOrderData, "partial_order_data"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			cause := errors.Join(tc.cause, context.DeadlineExceeded, errors.New("synthetic-sensitive-cause"))
			var output bytes.Buffer
			cli.WriteError(&output, cause)
			var response core.ErrorResponse
			if err := json.Unmarshal(output.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Error.Code != tc.code || !strings.Contains(response.Error.Message, "sync-status") || strings.Contains(output.String(), "synthetic-sensitive-cause") {
				t.Fatal("CLI sync error contract was lost or leaked raw cause")
			}
			ctx := context.Background()
			provider := interruptedSyncProvider{Service: orders.New(nil, nil), err: cause}
			server := mcpserver.NewWithProviders(mcpserver.Providers{Orders: provider}, "test")
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
			for _, name := range []string{"orders_sync"} {
				result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: name})
				if err != nil {
					t.Fatal(err)
				}
				encoded, err := json.Marshal(result)
				if err != nil {
					t.Fatal(err)
				}
				if !result.IsError || !bytes.Contains(encoded, []byte(tc.code)) || !bytes.Contains(encoded, []byte("orders_sync_status")) || bytes.Contains(encoded, []byte("synthetic-sensitive-cause")) {
					t.Fatalf("%s lost sync error classification or leaked raw cause", name)
				}
			}
		})
	}
}
