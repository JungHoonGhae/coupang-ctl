package products

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
)

func TestRecommendationSQLiteHistoryIncludesCandidatesBeyondFive(t *testing.T) {
	for _, count := range []int{6, 20} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			ctx := context.Background()
			ledger, err := store.Open(ctx, filepath.Join(t.TempDir(), "synthetic.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			defer ledger.Close()
			page := core.OrderPage{}
			for i := 0; i < count; i++ {
				page.Orders = append(page.Orders, core.Order{
					SourceRef: fmt.Sprintf("synthetic-%d", i), PurchasedAt: "2026-08-01", Currency: "KRW",
					Items: []core.OrderItem{{ProductID: fmt.Sprint(1000 + i), Name: "Synthetic bowl", Quantity: i + 1, PaidPrice: 1200}},
				})
			}
			if _, err := ledger.UpsertOrderPage(ctx, page); err != nil {
				t.Fatal(err)
			}
			for _, display := range []int{0, 1} {
				result, err := New(newSelectionSource(count)).WithPurchaseHistory(ledger).Recommend(ctx, core.ProductRecommendationRequest{
					Query: "synthetic", Proceed: true, SearchPageLimit: 1, MaxItems: display, UsePurchaseHistory: true, DisableAffiliate: true,
				})
				if err != nil {
					t.Fatal(err)
				}
				if result.PurchaseContext == nil || result.PurchaseContext.Status != "partial" || len(result.PurchaseContext.Matches) != count {
					t.Fatalf("SQLite history unavailable or truncated for %d inspected candidates, display=%d", count, display)
				}
				for i, match := range result.PurchaseContext.Matches {
					if match.Reference.ProductID != fmt.Sprint(1000+i) || match.RetainedUnits != i+1 || match.OrderCount != 1 {
						t.Fatal("candidate identity or purchase aggregate changed")
					}
				}
				if result.Status != core.ProductRecommendationIncomplete || result.PurchaseContext.Sync.State != core.SyncRunNeverRun {
					t.Fatal("larger context read invented complete coverage")
				}
			}
		})
	}
}
