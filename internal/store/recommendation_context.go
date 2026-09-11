package store

import (
	"context"
	"database/sql"
	"errors"
	"regexp"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

var recommendationIdentity = regexp.MustCompile(`^[0-9]{1,32}$`)

// A bounded local aggregation request, not a shortlist length or a claim about
// sufficient research. Includes hidden candidates before presentation limits.
const maxPurchaseContextReferences = 200

// RecommendationPurchaseContext reads candidate-specific aggregates and sync
// coverage in one SQLite snapshot. No raw order rows leave this adapter.
func (s *SQLite) RecommendationPurchaseContext(ctx context.Context, refs []core.ProductReference) (core.RecommendationPurchaseContext, error) {
	result := core.RecommendationPurchaseContext{
		Visibility: "private_local", Status: "partial",
		Provenance: core.RecommendationPurchaseProvenance,
		Matches:    []core.RecommendationPurchaseMatch{},
		Limitations: []string{
			"scope is all locally stored eligible product-purchase lines for returned candidate identities, not all account purchases",
			"retained units are max(quantity minus cancelled quantity minus returned quantity, zero); fully cancelled orders and cancelled or returned lines are excluded",
			"product-only matches combine variants; exact matches require both product_id and vendor_item_id; item_id is not present in order history",
			"no match does not mean never purchased; missing identities, incomplete sync, and later source changes may hide purchases",
			"purchase dates are reported at month precision; counts do not establish preference, satisfaction, consumption, or reorder need",
		},
	}
	if len(refs) > maxPurchaseContextReferences {
		return result, errors.New("purchase context accepts at most 200 candidate identities per snapshot")
	}
	for _, ref := range refs {
		if !recommendationIdentity.MatchString(ref.ProductID) || (ref.VendorItemID != "" && !recommendationIdentity.MatchString(ref.VendorItemID)) {
			return result, errors.New("purchase context requires numeric product and optional vendor item identities")
		}
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	syncStatus, err := latestSyncStatus(ctx, tx)
	if err != nil {
		return result, err
	}
	result.Sync = &syncStatus
	if syncStatus.State == core.SyncRunCompleted && syncStatus.HistoryComplete {
		result.Status = "available"
	}
	seen := make(map[string]bool)
	for _, ref := range refs {
		key := ref.ProductID + "/" + ref.VendorItemID
		if seen[key] {
			continue
		}
		seen[key] = true
		match := core.RecommendationPurchaseMatch{
			Reference:     core.ProductReference{ProductID: ref.ProductID, VendorItemID: ref.VendorItemID},
			IdentityScope: "product_only",
		}
		if ref.VendorItemID != "" {
			match.IdentityScope = "product_and_vendor_item"
		}
		err := tx.QueryRowContext(ctx, `SELECT COUNT(DISTINCT o.source_ref), COUNT(*),
			COALESCE(SUM(MAX(i.quantity - i.cancelled_quantity - i.returned_quantity, 0)), 0),
			COALESCE(SUBSTR(MIN(NULLIF(o.purchased_at, '')), 1, 7), ''),
			COALESCE(SUBSTR(MAX(NULLIF(o.purchased_at, '')), 1, 7), '')
			FROM order_items i JOIN orders o ON o.source_ref = i.order_ref
			WHERE i.product_id = ? AND (? = '' OR i.vendor_item_id = ?)
			AND o.fully_canceled = 0 AND i.commerce_kind = 'product_purchase'
			AND COALESCE(i.delivery_status, '') NOT IN ('cancelled', 'returned')
			AND i.quantity - i.cancelled_quantity - i.returned_quantity > 0`,
			ref.ProductID, ref.VendorItemID, ref.VendorItemID).Scan(
			&match.OrderCount, &match.ItemLineCount, &match.RetainedUnits,
			&match.FirstPurchaseMonth, &match.LatestPurchaseMonth)
		if err != nil {
			return result, err
		}
		if match.ItemLineCount > 0 {
			result.Matches = append(result.Matches, match)
		}
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}
