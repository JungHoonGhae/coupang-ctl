package cli

import (
	"context"

	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	orderworkflow "github.com/JungHoonGhae/coupang-ctl/internal/orders"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
)

type currentBrowserOrderSync struct {
	ledger *store.SQLite
}

func (provider currentBrowserOrderSync) Sync(ctx context.Context, request core.SyncRequest) (core.SyncResult, error) {
	browserAdapter := browser.NewNativeCurrentBrowser()
	defer browserAdapter.Close()
	return orderworkflow.NewWithSyncSource(provider.ledger, browserAdapter, core.SyncSourceCurrentBrowser).Sync(ctx, request)
}
