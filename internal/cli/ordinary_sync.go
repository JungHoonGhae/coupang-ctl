package cli

import (
	"context"

	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	orderworkflow "github.com/JungHoonGhae/coupang-ctl/internal/orders"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
)

type ordinaryBrowserOrderSync struct {
	ledger   *store.SQLite
	stateDir string
}

func (provider ordinaryBrowserOrderSync) Sync(ctx context.Context, request core.SyncRequest) (core.SyncResult, error) {
	bridge, err := browser.StartOrdinaryBrowserBridge(provider.stateDir)
	if err != nil {
		return core.SyncResult{}, err
	}
	defer bridge.Close()
	return orderworkflow.NewWithPageSource(provider.ledger, bridge).Sync(ctx, request)
}
