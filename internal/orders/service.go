package orders

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/coupang/categories"
	coupangorders "github.com/JungHoonGhae/coupang-ctl/internal/coupang/orders"
	"github.com/JungHoonGhae/coupang-ctl/internal/insights"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
)

const (
	defaultPageBudget     = 100
	maxPageBudget         = 1000
	defaultCategoryBudget = 25
	maxCategoryBudget     = 500
	defaultCatalogLimit   = 50
	syncTimeBudget        = 5 * time.Minute
	syncPageTimeout       = 60 * time.Second
)

var ErrDocumentSource = core.WithErrorCode("document_source_unavailable", errors.New("protected document source unavailable"))
var ErrCursorLoop = core.ErrSyncCursorLoop
var ErrSyncTimeBudget = core.ErrSyncTimeBudget
var ErrSyncPageDeadline = core.ErrSyncPageDeadline

type DocumentSource interface {
	Fetch(context.Context, *core.OrderCursor) ([]byte, error)
}

type PageSource interface {
	FetchPage(context.Context, *core.OrderCursor) (core.OrderPage, error)
}

type CategoryDocumentSource interface {
	FetchProductCategory(context.Context, core.ProductReference) ([]byte, error)
}

type Service struct {
	ledger         *store.SQLite
	source         DocumentSource
	pageSource     PageSource
	categorySource CategoryDocumentSource
	syncSource     core.SyncSource
	now            func() time.Time
}

func New(ledger *store.SQLite, source DocumentSource) *Service {
	return NewWithSyncSource(ledger, source, core.SyncSourceDedicatedBrowser)
}

func NewWithSyncSource(ledger *store.SQLite, source DocumentSource, syncSource core.SyncSource) *Service {
	categorySource, _ := source.(CategoryDocumentSource)
	return &Service{ledger: ledger, source: source, categorySource: categorySource, syncSource: syncSource, now: time.Now}
}

func NewWithPageSource(ledger *store.SQLite, source PageSource) *Service {
	return NewWithPageSourceAndSyncSource(ledger, source, core.SyncSourceOrdinaryBrowser)
}

func NewWithPageSourceAndSyncSource(ledger *store.SQLite, source PageSource, syncSource core.SyncSource) *Service {
	categorySource, _ := source.(CategoryDocumentSource)
	return &Service{ledger: ledger, pageSource: source, categorySource: categorySource, syncSource: syncSource, now: time.Now}
}

func (s *Service) Sync(ctx context.Context, request core.SyncRequest) (core.SyncResult, error) {
	return s.syncWithTimeLimits(ctx, request, syncTimeBudget, syncPageTimeout)
}

func (s *Service) syncWithTimeLimits(ctx context.Context, request core.SyncRequest, total, perPage time.Duration) (result core.SyncResult, err error) {
	budget := request.MaxPages
	if budget == 0 {
		budget = defaultPageBudget
	}
	if budget < 1 || budget > maxPageBudget {
		return core.SyncResult{}, errors.New("max_pages must be between 1 and 1000")
	}
	if s.source == nil && s.pageSource == nil {
		return core.SyncResult{}, ErrDocumentSource
	}
	if !s.syncSource.ValidForAcquisition() {
		return core.SyncResult{}, errors.New("invalid sync acquisition source")
	}
	ctx, cancel := context.WithTimeoutCause(ctx, total, ErrSyncTimeBudget)
	defer cancel()
	unlock, err := s.ledger.AcquireSyncWriter(ctx)
	if err != nil {
		return core.SyncResult{}, err
	}
	defer func() { err = errors.Join(err, unlock()) }()
	cursor, err := s.ledger.LoadSyncCursor(ctx)
	if err != nil {
		return core.SyncResult{}, err
	}
	var runID int64
	if request.RestartScan {
		runID, err = s.ledger.RestartSyncScan(ctx, s.syncSource, core.SyncProvenanceObservedStructuredOrderDocument, cursor)
	} else {
		runID, err = s.ledger.BeginResumableSync(ctx, s.syncSource, core.SyncProvenanceObservedStructuredOrderDocument, cursor)
	}
	if err != nil {
		return core.SyncResult{}, err
	}
	if request.RestartScan {
		cursor = nil
	}
	result = core.SyncResult{
		SchemaVersion:  core.SyncResultSchemaVersion,
		Source:         s.syncSource,
		Provenance:     core.SyncProvenanceObservedStructuredOrderDocument,
		Next:           cursor,
		CoverageStatus: core.SyncCoverageUnverified,
	}
	seenCursors := map[string]bool{}

	finish := func(code string) error {
		// Cancellation stops acquisition, not the bounded attempt bookkeeping.
		// No source read or page commit is performed with this cleanup context.
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		return s.ledger.FinishSync(cleanupCtx, runID, result, code)
	}
	fail := func(code string, cause error) (core.SyncResult, error) {
		if ctx.Err() != nil {
			cause = errors.Join(cause, ctx.Err(), context.Cause(ctx))
		}
		if errors.Is(cause, context.Canceled) {
			code = "canceled"
		} else if errors.Is(cause, ErrSyncTimeBudget) {
			code = "time_budget_exhausted"
		} else if errors.Is(cause, ErrSyncPageDeadline) {
			code = "page_deadline_exceeded"
		} else if errors.Is(cause, context.DeadlineExceeded) {
			code = "deadline_exceeded"
		}
		if finishErr := finish(code); finishErr != nil {
			return result, errors.Join(cause, finishErr)
		}
		return result, cause
	}

	for result.PagesProcessed < budget {
		if err := ctx.Err(); err != nil {
			return fail("canceled", err)
		}
		key := cursorKey(cursor)
		if seenCursors[key] {
			return fail("cursor_loop", ErrCursorLoop)
		}
		seenCursors[key] = true
		if err := s.ledger.CheckSyncCursor(ctx, runID, cursor); err != nil {
			if errors.Is(err, ErrCursorLoop) {
				return fail("cursor_loop", err)
			}
			return fail("storage", err)
		}

		// The same deadline covers acquisition, parsing, and the atomic page
		// commit. A fresh page cannot extend the enclosing attempt deadline.
		pageCtx, cancelPage := context.WithTimeoutCause(ctx, perPage, ErrSyncPageDeadline)
		page, upserted, code, err := s.syncPage(pageCtx, runID, cursor)
		// A successfully committed page must remain counted even if the timer
		// fires immediately after Commit. Subsequent work checks the deadline.
		if err != nil && pageCtx.Err() != nil {
			err = errors.Join(err, pageCtx.Err(), context.Cause(pageCtx))
		}
		cancelPage()
		if err != nil {
			return fail(code, err)
		}
		result.PagesProcessed++
		result.OrdersSeen += upserted.OrdersSeen
		result.ItemsSeen += upserted.ItemsSeen
		result.Next = page.Next
		cursor = page.Next
		if cursor == nil {
			result.CursorExhausted = true
			// Cursor exhaustion is not proof of account identity, coverage,
			// or deletion upstream. Retain orders not observed by this attempt.
			break
		}
	}
	if err := ctx.Err(); err != nil {
		return fail("canceled", err)
	}
	if err := finish(""); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Service) syncPage(ctx context.Context, runID int64, cursor *core.OrderCursor) (core.OrderPage, core.UpsertResult, string, error) {
	var page core.OrderPage
	var err error
	errorCode := "document_source"
	if s.pageSource != nil {
		page, err = s.pageSource.FetchPage(ctx, cursor)
	} else {
		var document []byte
		document, err = s.source.Fetch(ctx, cursor)
		if err == nil {
			if err = ctx.Err(); err != nil {
				return page, core.UpsertResult{}, "deadline_exceeded", err
			}
			page, err = coupangorders.ParseOrderDocument(document)
			errorCode = "invalid_document"
		}
	}
	if err != nil {
		if errors.Is(err, core.ErrPartialOrderData) {
			return core.OrderPage{}, core.UpsertResult{}, "partial_order_data", err
		}
		if errorCode == "document_source" {
			err = errors.Join(ErrDocumentSource, err)
		}
		return page, core.UpsertResult{}, errorCode, err
	}
	if err := ctx.Err(); err != nil {
		return page, core.UpsertResult{}, "deadline_exceeded", err
	}
	upserted, err := s.ledger.ApplySyncPage(ctx, runID, cursor, page)
	return page, upserted, "storage", err
}

func (s *Service) SyncStatus(ctx context.Context) (core.SyncStatus, error) {
	return s.ledger.LatestSyncStatus(ctx)
}

func (s *Service) List(ctx context.Context, filter core.OrderFilter) ([]core.Order, error) {
	return s.ledger.ListOrders(ctx, filter)
}

func (s *Service) Spend(ctx context.Context, filter core.OrderFilter) (core.SpendSummary, error) {
	return s.ledger.Spend(ctx, filter)
}

func (s *Service) Stats(ctx context.Context, filter core.OrderFilter) (core.OrderStats, error) {
	return s.ledger.Stats(ctx, filter)
}

func (s *Service) Insights(ctx context.Context, filter core.OrderFilter) (core.ShoppingInsights, error) {
	result, err := s.ledger.Insights(ctx, filter)
	if err != nil {
		return core.ShoppingInsights{}, err
	}
	result.Profile = insights.BuildShoppingProfile(result)
	return result, nil
}

func (s *Service) ShoppingAnalysis(ctx context.Context, filter core.OrderFilter, includeProducts bool) (core.ShoppingAnalysis, error) {
	result, err := s.ledger.ShoppingAnalysis(ctx, filter, includeProducts)
	if err != nil {
		return core.ShoppingAnalysis{}, err
	}
	result.Insights.Profile = insights.BuildShoppingProfile(result.Insights)
	return result, nil
}

func (s *Service) ProductInsights(ctx context.Context, filter core.OrderFilter) (core.ProductInsights, error) {
	return s.ledger.ProductInsights(ctx, filter)
}

func (s *Service) CategoryCatalog(ctx context.Context, request core.CategoryCatalogRequest) (core.CategoryCatalog, error) {
	if err := core.ValidateRequest(request); err != nil {
		return core.CategoryCatalog{}, err
	}
	request.Query = strings.TrimSpace(request.Query)
	if request.Limit == 0 {
		request.Limit = defaultCatalogLimit
	}
	return s.ledger.CategoryCatalog(ctx, request)
}

func (s *Service) CategoryStability(ctx context.Context) (core.CategoryStabilityReport, error) {
	return s.ledger.CategoryStability(ctx)
}

func (s *Service) EnrichCategories(ctx context.Context, request core.CategoryEnrichmentRequest) (core.CategoryEnrichmentResult, error) {
	budget := request.MaxProducts
	if budget == 0 {
		budget = defaultCategoryBudget
	}
	if budget < 1 || budget > maxCategoryBudget {
		return core.CategoryEnrichmentResult{}, errors.New("max_products must be between 1 and 500")
	}
	if s.categorySource == nil {
		return core.CategoryEnrichmentResult{}, ErrDocumentSource
	}
	pending, err := s.ledger.CategoryProductsForEnrichment(ctx, budget, request.Recheck)
	if err != nil {
		return core.CategoryEnrichmentResult{}, err
	}
	result := core.CategoryEnrichmentResult{Recheck: request.Recheck}
	if request.Recheck {
		result.RecheckCandidateCount, err = s.ledger.CategoryRecheckCandidateCount(ctx)
		if err != nil {
			return result, err
		}
		result.RecheckTruncated = result.RecheckCandidateCount > len(pending)
	}
	for _, reference := range pending {
		document, err := s.categorySource.FetchProductCategory(ctx, reference)
		if errors.Is(err, core.ErrProductCategoryUnavailable) {
			if err := s.ledger.SaveUnavailableProductCategory(ctx, reference); err != nil {
				return result, err
			}
			result.ProductsProcessed++
			result.CategoriesUnavailable++
			continue
		}
		if err != nil {
			return result, errors.Join(ErrDocumentSource, err)
		}
		category, err := categories.ParseProductCategory(document)
		if errors.Is(err, categories.ErrCategoryDataMissing) {
			if err := s.ledger.SaveMissingProductCategory(ctx, reference); err != nil {
				return result, err
			}
			result.ProductsProcessed++
			result.CategoriesMissing++
			continue
		}
		if err != nil {
			return result, err
		}
		if err := s.ledger.SaveProductCategory(ctx, reference, category); err != nil {
			return result, err
		}
		result.ProductsProcessed++
		result.CategoriesStored++
	}
	result.RemainingProducts, err = s.ledger.RemainingCategoryProducts(ctx)
	if err != nil {
		return result, err
	}
	result.Complete = result.RemainingProducts == 0 && !result.RecheckTruncated
	return result, nil
}

func (s *Service) ReorderCandidates(ctx context.Context, filter core.OrderFilter) ([]core.ReorderCandidate, error) {
	candidates, err := s.ledger.ReorderCandidates(ctx, filter)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	for index := range candidates {
		comparison := &candidates[index].PriceComparison
		if comparison.Status != "available" {
			continue
		}
		observedAt, parseErr := time.Parse(time.RFC3339Nano, comparison.ObservedAt)
		if parseErr != nil || observedAt.After(now.Add(5*time.Minute)) {
			comparison.Freshness = "unknown"
			comparison.Limitations = append(comparison.Limitations, "the local observation timestamp could not be compared with the current clock")
			continue
		}
		age := now.Sub(observedAt)
		comparison.ObservationAgeHours = int(age / time.Hour)
		comparison.Freshness = "recent_under_24h"
		if age >= 24*time.Hour {
			comparison.Freshness = "stale_24h_or_more"
			comparison.Limitations = append(comparison.Limitations, "the latest local observation is at least 24 hours old; refresh the exact product before acting")
		}
	}
	return candidates, nil
}

func (s *Service) Export(ctx context.Context, filter core.OrderFilter) (core.OrderExport, error) {
	return s.ledger.Export(ctx, filter, s.now())
}

func (s *Service) Purge(ctx context.Context) (core.PurgeResult, error) {
	return s.ledger.Purge(ctx)
}

func (s *Service) Import(ctx context.Context, exported core.OrderExport) (core.UpsertResult, error) {
	if exported.SchemaVersion != 1 {
		return core.UpsertResult{}, errors.New("unsupported normalized export schema")
	}
	if len(exported.Orders) > 10000 {
		return core.UpsertResult{}, errors.New("normalized export exceeds the order limit")
	}
	return s.ledger.UpsertOrderPage(ctx, core.OrderPage{Orders: exported.Orders})
}

func cursorKey(cursor *core.OrderCursor) string {
	if cursor == nil {
		return "initial"
	}
	return fmt.Sprintf("%d/%d", cursor.Year, cursor.Page)
}
