package mcpserver_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/cli"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/mcpserver"
	"github.com/JungHoonGhae/coupang-ctl/internal/orders"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCLIAndMCPSyncStatusV3PreserveUnknownAndCursorExhaustion(t *testing.T) {
	for _, exhausted := range []bool{false, true} {
		ctx := context.Background()
		root := t.TempDir()
		t.Setenv("COUPANGCTL_STATE_DIR", root)
		ledger, err := store.Open(ctx, filepath.Join(root, "camofox-coupangctl.sqlite3"))
		if err != nil {
			t.Fatal(err)
		}
		defer ledger.Close()
		if exhausted {
			run, err := ledger.BeginSync(ctx, core.SyncSourceDedicatedBrowser, core.SyncProvenanceObservedStructuredOrderDocument)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ledger.ApplySyncPage(ctx, run, nil, core.OrderPage{}); err != nil {
				t.Fatal(err)
			}
			if err := ledger.FinishSync(ctx, run, core.SyncResult{Complete: true, PagesProcessed: 1}, ""); err != nil {
				t.Fatal(err)
			}
		}
		var stdout, stderr bytes.Buffer
		if err := cli.Run(ctx, []string{"orders", "sync-status"}, &stdout, &stderr, "test"); err != nil {
			t.Fatal(err)
		}
		var cliWire map[string]any
		if err := json.Unmarshal(stdout.Bytes(), &cliWire); err != nil {
			t.Fatal(err)
		}
		if stderr.Len() != 0 {
			t.Fatal("local status requested browser setup")
		}
		if _, err := os.Stat(filepath.Join(root, "browser-profile")); !os.IsNotExist(err) {
			t.Fatal("local status initialized a browser profile")
		}
		server := mcpserver.NewWithProviders(mcpserver.Providers{Orders: orders.New(ledger, nil)}, "test")
		ct, st := mcp.NewInMemoryTransports()
		ss, err := server.Connect(ctx, st, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer ss.Close()
		client := mcp.NewClient(&mcp.Implementation{Name: "synthetic", Version: "test"}, nil)
		cs, err := client.Connect(ctx, ct, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer cs.Close()
		result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "orders_sync_status", Arguments: struct{}{}})
		if err != nil || result.IsError {
			t.Fatal("MCP status failed")
		}
		encoded, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var mcpWire map[string]any
		if err := json.Unmarshal(encoded, &mcpWire); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(cliWire, mcpWire) {
			t.Fatal("CLI/MCP disagree on actual ledger coverage")
		}
		if cliWire["schema_version"] != float64(3) || cliWire["history_complete"] != false {
			t.Fatal("sync status v3 completion boundary lost")
		}
		if value, exists := cliWire["scan"]; !exists || value != nil {
			t.Fatal("unmanaged attempt invented cumulative scan evidence")
		}
		value, present := cliWire["cursor_exhausted"]
		if !present {
			t.Fatal("unknown cursor evidence omitted")
		}
		if exhausted {
			if value != true || cliWire["coverage_status"] != string(core.SyncCoverageUnverified) {
				t.Fatal("cursor exhaustion mistaken for coverage")
			}
		} else if value != nil || cliWire["coverage_status"] != string(core.SyncCoverageNotAssessed) {
			t.Fatal("never-run status invented evidence")
		}
	}
}

type scanEvidenceSource struct{ reads int }

func (s *scanEvidenceSource) FetchPage(_ context.Context, _ *core.OrderCursor) (core.OrderPage, error) {
	s.reads++
	page := core.OrderPage{Orders: []core.Order{{SourceRef: "synthetic-observed", PurchasedAt: "2026-08-01", Currency: "KRW"}}}
	if s.reads == 1 {
		page.Next = &core.OrderCursor{Year: 2026, Page: 2}
	}
	return page, nil
}

func TestCLIAndMCPExposeSameResumedScanWithoutAdditionalSourceReads(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	t.Setenv("COUPANGCTL_STATE_DIR", root)
	ledger, err := store.Open(ctx, filepath.Join(root, "camofox-coupangctl.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()
	if _, err := ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: []core.Order{{SourceRef: "synthetic-retained", PurchasedAt: "2026-08-02", Currency: "KRW"}}}); err != nil {
		t.Fatal(err)
	}
	source := &scanEvidenceSource{}
	service := orders.NewWithPageSourceAndSyncSource(ledger, source, core.SyncSourceCamofox)
	for i := 0; i < 2; i++ {
		if _, err := service.Sync(ctx, core.SyncRequest{MaxPages: 1}); err != nil {
			t.Fatal(err)
		}
	}
	server := mcpserver.NewWithProviders(mcpserver.Providers{Orders: service}, "test")
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "synthetic", Version: "test"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	for _, command := range []struct {
		cli, mcp string
		filtered bool
	}{{"sync-status", "orders_sync_status", false}, {"stats", "orders_stats", false}, {"stats", "orders_stats", true}} {
		var stdout, stderr bytes.Buffer
		args := []string{"orders", command.cli}
		toolArgs := map[string]any{}
		if command.filtered {
			args = append(args, "--from", "2026-08-01", "--to", "2026-08-01")
			toolArgs["from"], toolArgs["to"] = "2026-08-01", "2026-08-01"
		}
		if err := cli.Run(ctx, args, &stdout, &stderr, "test"); err != nil {
			t.Fatal(err)
		}
		if stderr.Len() != 0 {
			t.Fatal("local status unexpectedly needed runtime setup")
		}
		var cliStatus core.SyncStatus
		if command.cli == "stats" {
			var stats core.OrderStats
			if err := json.Unmarshal(stdout.Bytes(), &stats); err != nil {
				t.Fatal(err)
			}
			cliStatus = stats.Evidence.LatestAttempt
			wantCount := 2
			if command.filtered {
				wantCount = 1
			}
			if stats.OrderCount != wantCount {
				t.Fatal("retained history changed aggregate scope")
			}
		} else if err := json.Unmarshal(stdout.Bytes(), &cliStatus); err != nil {
			t.Fatal(err)
		}
		result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: command.mcp, Arguments: toolArgs})
		if err != nil || result.IsError {
			t.Fatal("MCP rejected cumulative scan response", err)
		}
		encoded, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var mcpStatus core.SyncStatus
		if command.cli == "stats" {
			var stats core.OrderStats
			if err := json.Unmarshal(encoded, &stats); err != nil {
				t.Fatal(err)
			}
			mcpStatus = stats.Evidence.LatestAttempt
		} else if err := json.Unmarshal(encoded, &mcpStatus); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(cliStatus, mcpStatus) {
			t.Fatal("CLI and MCP disagree on the same scan")
		}
		scan := cliStatus.Scan
		if cliStatus.SchemaVersion != 3 || cliStatus.HistoryComplete || scan == nil || scan.Attempts != 2 || scan.PagesProcessed != 2 || scan.RetainedOrdersObserved != 1 || scan.RetainedOrdersNotObserved != 1 || scan.State != "cursor_exhausted" {
			t.Fatal("wire lost resumed scan, distinct observations, or retained-history scope")
		}
		if bytes.Contains(encoded, []byte("synthetic-observed")) || bytes.Contains(stdout.Bytes(), []byte("synthetic-retained")) {
			t.Fatal("metadata exposed order references")
		}
	}
	if source.reads != 2 {
		t.Fatal("local status or statistics contacted source")
	}
}
