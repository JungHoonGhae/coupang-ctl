package core

import (
	"errors"
	"time"
)

// Validate the supplied aggregate's shape and candidate scope, not its truth.
// The report never opens a ledger or authenticates an account to verify it.
func validateReportPurchase(r ProductRecommendationResult) error {
	p := r.PurchaseContext
	if p == nil {
		return nil
	}
	if p.Visibility != "private_local" || len(p.Matches) > 200 {
		return errors.New("report purchase context must be private-local and bounded")
	}
	switch p.Status {
	case "available", "partial":
	case "unavailable":
		if len(p.Matches) != 0 {
			return errors.New("unavailable purchase context cannot contain aggregates")
		}
	default:
		return errors.New("unsupported report purchase context status")
	}
	if len(p.Matches) > 0 && p.Provenance != RecommendationPurchaseProvenance {
		return errors.New("report purchase aggregates require the normalized-order derivation")
	}
	seen := make(map[ProductReference]bool)
	for _, m := range p.Matches {
		if !validReportReference(m.Reference) || m.Reference.ItemID != "" || seen[m.Reference] {
			return errors.New("report purchase references must be unique product/vendor identities without item ids")
		}
		seen[m.Reference] = true
		if m.IdentityScope != "product_only" && m.IdentityScope != "product_and_vendor_item" ||
			(m.IdentityScope == "product_only") != (m.Reference.VendorItemID == "") {
			return errors.New("report purchase identity scope contradicts its reference")
		}
		matched := false
		for _, records := range [][]ProductRecommendationCandidate{r.Candidates, r.Inspected} {
			for _, c := range records {
				ref := c.Product.Reference
				if ref.ProductID == m.Reference.ProductID && (m.Reference.VendorItemID == "" || ref.VendorItemID == m.Reference.VendorItemID) {
					matched = true
				}
			}
		}
		if !matched {
			return errors.New("report purchase aggregate does not belong to an inspected or displayed candidate")
		}
		if m.OrderCount < 1 || m.ItemLineCount < m.OrderCount || m.RetainedUnits < m.ItemLineCount {
			return errors.New("report purchase aggregate counts are inconsistent")
		}
		for _, month := range []string{m.FirstPurchaseMonth, m.LatestPurchaseMonth} {
			if month != "" {
				t, err := time.Parse("2006-01", month)
				if err != nil || t.Year() < 1 || t.Format("2006-01") != month {
					return errors.New("report purchase dates require valid month precision")
				}
			}
		}
		if m.FirstPurchaseMonth != "" && m.LatestPurchaseMonth != "" && m.FirstPurchaseMonth > m.LatestPurchaseMonth {
			return errors.New("report purchase month range is reversed")
		}
	}
	return validateReportPurchaseSync(p.Sync)
}

func validateReportPurchaseSync(s *SyncStatus) error {
	if s == nil {
		return nil
	}
	if (s.SchemaVersion != 2 && s.SchemaVersion != SyncStatusSchemaVersion) || s.Visibility != "private_local" {
		return errors.New("report requires private-local sync evidence v2 or v3")
	}
	switch s.State {
	case SyncRunNeverRun, SyncRunRunning, SyncRunCompleted, SyncRunFailed:
	default:
		return errors.New("unsupported report sync attempt state")
	}
	switch s.CoverageStatus {
	case SyncCoverageNotAssessed, SyncCoverageUnverified, SyncCoverageUnknownLegacy:
	default:
		return errors.New("unsupported report sync coverage")
	}
	// None of today's coverage states proves account continuity and full history.
	if s.HistoryComplete || s.PagesProcessed < 0 || s.OrdersSeen < 0 {
		return errors.New("report sync counts or full-history claim contradict coverage")
	}
	stamps := []string{s.StartedAt, s.CompletedAt}
	if scan := s.Scan; scan != nil {
		if s.SchemaVersion != SyncStatusSchemaVersion || scan.Attempts < 1 || scan.PagesProcessed < 0 || scan.RetainedOrdersObserved < 0 || scan.RetainedOrdersNotObserved < 0 {
			return errors.New("report cumulative scan counts or schema are invalid")
		}
		switch scan.State {
		case SyncScanActive, SyncScanCursorExhausted, SyncScanSuperseded:
		default:
			return errors.New("unsupported report cumulative scan state")
		}
		stamps = append(stamps, scan.StartedAt, scan.EndedAt, scan.SupersededAt)
	}
	for _, value := range stamps {
		if value != "" {
			if _, err := time.Parse(time.RFC3339Nano, value); err != nil {
				return errors.New("report sync timestamps must be RFC3339")
			}
		}
	}
	return nil
}
