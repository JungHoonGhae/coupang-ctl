package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestScanAndAttemptEvidenceShareSnapshotDuringPageCommit(t *testing.T) {
	for _, operation := range []string{"sync-status", "stats"} {
		t.Run(operation, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			path := filepath.Join(t.TempDir(), "synthetic.sqlite3")
			reader, writer := openWriterLedger(t, path), openWriterLedger(t, path)
			release, err := writer.AcquireSyncWriter(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			run := startSyntheticScan(t, writer, nil)
			fired := false
			connector := &snapshotConnector{driver: reader.db.Driver(), path: path,
				match: func(q string) bool { return strings.Contains(q, "FROM sync_runs ORDER BY id DESC LIMIT 1") },
				afterAggregate: func() error {
					fired = true
					_, err := writer.ApplySyncPage(ctx, run, nil, core.OrderPage{Orders: []core.Order{{SourceRef: "synthetic-concurrent", PurchasedAt: "2026-08-01", Currency: "KRW"}}})
					return err
				},
			}
			reader.db.Close()
			reader.db = sql.OpenDB(connector)
			reader.db.SetMaxOpenConns(1)
			var status core.SyncStatus
			if operation == "sync-status" {
				status, err = reader.LatestSyncStatus(ctx)
			} else {
				stats, readErr := reader.Stats(ctx, core.OrderFilter{})
				status, err = stats.Evidence.LatestAttempt, readErr
				if stats.OrderCount != 0 {
					t.Fatal("aggregate escaped scan snapshot")
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if !fired || status.PagesProcessed != 0 || status.Scan == nil || status.Scan.PagesProcessed != 0 || status.Scan.RetainedOrdersObserved != 0 || status.Scan.State != "active" {
				t.Fatal("scan and attempt mixed concurrent commits")
			}
			latest, err := writer.LatestSyncStatus(ctx)
			if err != nil || latest.Scan == nil || latest.Scan.PagesProcessed != 1 || latest.Scan.RetainedOrdersObserved != 1 || latest.Scan.State != "cursor_exhausted" {
				t.Fatal("concurrent page was not committed", err)
			}
		})
	}
}
