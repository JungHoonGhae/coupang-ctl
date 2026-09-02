package orders

import (
	"context"
	"errors"
	"fmt"
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
)

var ErrDocumentSource = errors.New("protected document source unavailable")
var ErrCursorLoop = errors.New("order pagination cursor repeated")

type DocumentSource interface {
	Fetch(context.Context, *core.OrderCursor) ([]byte, error)
}

type CategoryDocumentSource interface {
	FetchProductCategory(context.Context, core.ProductReference) ([]byte, error)
}

type Service struct {
	ledger         *store.SQLite
	source         DocumentSource
	categorySource CategoryDocumentSource
	now            func() time.Time
}

func New(ledger *store.SQLite, source DocumentSource) *Service {
	categorySource, _ := source.(CategoryDocumentSource)
	return &Service{ledger: ledger, source: source, categorySource: categorySource, now: time.Now}
}

func (s *Service) Sync(ctx context.Context, request core.SyncRequest) (core.SyncResult, error) {
	budget := request.MaxPages
	if budget == 0 {
		budget = defaultPageBudget
	}
	if budget < 1 || budget > maxPageBudget {
		return core.SyncResult{}, errors.New("max_pages must be between 1 and 1000")
	}
	if s.source == nil {
		return core.SyncResult{}, ErrDocumentSource
	}
	cursor, err := s.ledger.LoadSyncCursor(ctx)
	if err != nil {
		return core.SyncResult{}, err
	}
	runID, err := s.ledger.BeginSync(ctx)
	if err != nil {
		return core.SyncResult{}, err
	}
	result := core.SyncResult{Next: cursor}
	completeHistoryWalk := cursor == nil
	seenCursors := map[string]bool{}
	seenOrderRefs := map[string]struct{}{}

	fail := func(code string, cause error) (core.SyncResult, error) {
		if finishErr := s.ledger.FinishSync(ctx, runID, result, code); finishErr != nil {
			return result, finishErr
		}
		return result, cause
	}

	for result.PagesProcessed < budget {
		key := cursorKey(cursor)
		if seenCursors[key] {
			return fail("cursor_loop", ErrCursorLoop)
		}
		seenCursors[key] = true

		document, err := s.source.Fetch(ctx, cursor)
		if err != nil {
			return fail("document_source", errors.Join(ErrDocumentSource, err))
		}
		page, err := coupangorders.ParseOrderDocument(document)
		if err != nil {
			return fail("invalid_document", err)
		}
		for _, order := range page.Orders {
			seenOrderRefs[order.SourceRef] = struct{}{}
		}
		upserted, err := s.ledger.UpsertOrderPage(ctx, page)
		if err != nil {
			return fail("storage", err)
		}
		if err := s.ledger.SaveSyncCursor(ctx, page.Next); err != nil {
			return fail("checkpoint", err)
		}
		result.PagesProcessed++
		result.OrdersSeen += upserted.OrdersSeen
		result.ItemsSeen += upserted.ItemsSeen
		result.Next = page.Next
		cursor = page.Next
		if cursor == nil {
			result.Complete = true
			if completeHistoryWalk {
				references := make([]string, 0, len(seenOrderRefs))
				for sourceRef := range seenOrderRefs {
					references = append(references, sourceRef)
				}
				removed, reconcileErr := s.ledger.ReconcileOrders(ctx, references)
				if reconcileErr != nil {
					return fail("reconciliation", reconcileErr)
				}
				result.OrdersRemoved = removed
			}
			break
		}
	}
	if err := s.ledger.FinishSync(ctx, runID, result, ""); err != nil {
		return result, err
	}
	return result, nil
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
	result.Categories, err = s.ledger.CategoryBreakdown(ctx, filter)
	if err != nil {
		return core.ShoppingInsights{}, err
	}
	result.Profile = insights.BuildShoppingProfile(result)
	return result, nil
}

func (s *Service) ProductInsights(ctx context.Context, filter core.OrderFilter) (core.ProductInsights, error) {
	return s.ledger.ProductInsights(ctx, filter)
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
	pending, err := s.ledger.PendingCategoryProducts(ctx, budget)
	if err != nil {
		return core.CategoryEnrichmentResult{}, err
	}
	result := core.CategoryEnrichmentResult{}
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
	result.Complete = result.RemainingProducts == 0
	return result, nil
}

func (s *Service) ReorderCandidates(ctx context.Context, filter core.OrderFilter) ([]core.ReorderCandidate, error) {
	return s.ledger.ReorderCandidates(ctx, filter)
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
