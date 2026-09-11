package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/orders"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
)

type capturingRestartWorkflow struct {
	orderWorkflow
	request core.SyncRequest
}

type restartCLIPageSource struct{ cursors []*core.OrderCursor }

func (s *restartCLIPageSource) FetchPage(_ context.Context, cursor *core.OrderCursor) (core.OrderPage, error) {
	s.cursors = append(s.cursors, cursor)
	return core.OrderPage{Orders: []core.Order{}, Next: &core.OrderCursor{Year: 2026, Page: 2}}, nil
}

func TestOrderSyncRestartFlagReachesRealService(t *testing.T) {
	for _, restart := range []bool{false, true} {
		ctx := context.Background()
		ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ledger.Close() })
		source := &restartCLIPageSource{}
		service := orders.NewWithPageSource(ledger, source)
		if _, err := service.Sync(ctx, core.SyncRequest{MaxPages: 1}); err != nil {
			t.Fatal(err)
		}
		args := []string{"sync", "--max-pages", "1"}
		if restart {
			args = append(args, "--restart-scan")
		}
		var output bytes.Buffer
		if err := runOrders(ctx, args, &output, service); err != nil {
			t.Fatal(err)
		}
		var result core.SyncResult
		if err := json.Unmarshal(output.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.PagesProcessed != 1 || result.Complete || len(source.cursors) != 2 || (source.cursors[1] == nil) != restart {
			t.Fatal("CLI service restart/resume contract lost")
		}
	}
}

func (w *capturingRestartWorkflow) Sync(_ context.Context, request core.SyncRequest) (core.SyncResult, error) {
	w.request = request
	return core.SyncResult{SchemaVersion: core.SyncResultSchemaVersion}, nil
}

func TestOrderSyncRestartFlagIsExplicitAndPreservesBudget(t *testing.T) {
	for _, restart := range []bool{false, true} {
		w := &capturingRestartWorkflow{}
		args := []string{"sync", "--max-pages", "2"}
		if restart {
			args = append(args, "--restart-scan")
		}
		var output bytes.Buffer
		if err := runOrders(context.Background(), args, &output, w); err != nil {
			t.Fatal(err)
		}
		if w.request.RestartScan != restart || w.request.MaxPages != 2 {
			t.Fatal("restart or budget was lost")
		}
	}
}
