package store_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
)

// Entirely synthetic, local-only evidence: this measures aggregation latency,
// not network access, account identity, recommendation quality, or optimal count.
func BenchmarkRecommendationPurchaseContext200(b *testing.B) {
	ctx := context.Background()
	ledger, err := store.Open(ctx, filepath.Join(b.TempDir(), "synthetic.sqlite3"))
	if err != nil {
		b.Fatal(err)
	}
	defer ledger.Close()
	page := core.OrderPage{}
	for i := 0; i < 1000; i++ {
		order := core.Order{SourceRef: fmt.Sprintf("synthetic-%d", i), PurchasedAt: "2026-08-01", Currency: "KRW"}
		for j := 0; j < 10; j++ {
			order.Items = append(order.Items, core.OrderItem{ProductID: fmt.Sprint(1000 + (i*10+j)%200), Name: "Synthetic bowl", Quantity: 1, PaidPrice: 1200})
		}
		page.Orders = append(page.Orders, order)
	}
	if _, err := ledger.UpsertOrderPage(ctx, page); err != nil {
		b.Fatal(err)
	}
	refs := make([]core.ProductReference, 200)
	for i := range refs {
		refs[i].ProductID = fmt.Sprint(1000 + i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result, err := ledger.RecommendationPurchaseContext(ctx, refs)
		if err != nil {
			b.Fatal(err)
		}
		if len(result.Matches) != 200 {
			b.Fatal("aggregation lost requested candidates")
		}
		for _, match := range result.Matches {
			if match.OrderCount != 50 || match.ItemLineCount != 50 || match.RetainedUnits != 50 {
				b.Fatal("synthetic denominator mismatch")
			}
		}
	}
}
