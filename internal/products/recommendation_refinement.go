package products

import (
	"context"
	"strings"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

// refineRecommendation owns ordering, catalog validation, read reservations
// and partial-result retention. CLI/MCP never assemble a verified selection.
// The returned search is always the last successful state, never a union.
func (s *Service) refineRecommendation(ctx context.Context, request core.ProductRecommendationRequest, initial core.ProductSearchResult, result *core.ProductRecommendationResult) (core.ProductSearchResult, bool) {
	result.Refinement = &core.ProductSearchRefinement{AppliedSelections: []core.ProductFacetSelection{}, Steps: []core.ProductFacetObservation{}}
	record := func(selection *core.ProductFacetSelection) {
		result.Facets = cloneFacetCatalog(initial.Coverage.Facets)
		result.Audit.SourceFacetsObserved = len(result.Facets)
		result.Refinement.Steps = append(result.Refinement.Steps, core.ProductFacetObservation{CategoryID: request.CategoryID, Selection: selection, Facets: cloneFacetCatalog(result.Facets), FetchedAt: initial.FetchedAt})
	}
	record(nil)
	stop := func(selection core.ProductFacetSelection, reason string) (core.ProductSearchResult, bool) {
		result.Status = core.ProductRecommendationIncomplete
		if reason == "choice_unavailable" {
			result.Status = core.ProductRecommendationNeedsInput
		}
		result.Refinement.Issue = &core.ProductFacetSelectionIssue{Selection: selection, Reason: reason}
		result.Audit.DiscoveryStopReason = reason
		result.Audit.InspectionStopReason = "refinement_not_verified"
		result.Questions = questionsFromSourceFacets(result.Facets)
		result.Warnings = append(result.Warnings, "sidebar refinement stopped; retained search candidates belong only to the last verified selection state, not all requested choices; rediscover or correct the reported choice before continuing")
		retainRefinementCandidates(initial, result)
		kind := "rediscover_filters"
		if reason == "source_access_denied" || reason == "source_authentication_required" {
			kind = "restore_source_access"
		}
		result.NextActions = append(result.NextActions, core.ProductRecommendationNextAction{Kind: kind, Reason: reason})
		return initial, false
	}
	for _, answer := range request.Answers {
		id, choice := strings.TrimSpace(answer.QuestionID), strings.TrimSpace(answer.Choice)
		if !strings.HasPrefix(id, "facet:") || answer.Unbounded || choice == "" {
			continue
		}
		selection := core.ProductFacetSelection{Name: strings.TrimPrefix(id, "facet:"), Label: choice}
		if !availableFacetChoice(result.Facets, selection) {
			return stop(selection, "choice_unavailable")
		}
		selections := append([]core.ProductFacetSelection{}, result.Refinement.AppliedSelections...)
		trail := append([]string{}, result.Refinement.CategoryTrail...)
		replaced := false
		if selection.Name == "카테고리" {
			for i := range selections {
				if selections[i].Name == selection.Name {
					if request.Query != "" {
						trail = append(trail, selections[i].Label)
					}
					selections[i], replaced = selection, true
					break
				}
			}
		}
		if !replaced {
			selections = append(selections, selection)
		}
		search := recommendationSearchRequest(request, core.ProductSortCoupangRanking, 1, &core.ProductSearchRefinement{AppliedSelections: selections, CategoryTrail: trail})
		if ctx.Err() != nil {
			result.Audit.TimeBudgetExhausted = recommendationTimeBudgetExpired(ctx)
			return stop(selection, "time_budget_reached")
		}
		if result.Audit.DocumentReadReservations+search.DocumentReadLimit > recommendationDocumentBudget {
			result.Audit.DocumentBudgetConstrained = true
			return stop(selection, "document_budget_reached")
		}
		result.Audit.DocumentReadReservations += search.DocumentReadLimit
		result.Audit.SearchesRun++
		narrowed, err := s.Search(ctx, search)
		if err != nil {
			reason := recommendationAccessStopReason(err)
			if reason == "" {
				reason = "selection_unverified"
			}
			if recommendationTimeBudgetExpired(ctx) {
				reason = "time_budget_reached"
				result.Audit.TimeBudgetExhausted = true
			}
			return stop(selection, reason)
		}
		if narrowed.Coverage.AppliedCategoryID != "" && narrowed.Coverage.AppliedCategoryID != request.CategoryID && selection.Name != "카테고리" {
			return stop(selection, "source_category_changed")
		}
		result.Audit.SearchPagesRead++
		result.Audit.ListingsObserved += len(narrowed.Items)
		initial = narrowed
		if narrowed.Coverage.AppliedCategoryID != "" {
			request.CategoryID = narrowed.Coverage.AppliedCategoryID
			result.AppliedCategoryID = narrowed.Coverage.AppliedCategoryID
		}
		result.Refinement.AppliedSelections = selections
		result.Refinement.CategoryTrail = trail
		record(&selection)
	}
	return initial, true
}

func retainRefinementCandidates(initial core.ProductSearchResult, result *core.ProductRecommendationResult) {
	result.Discovered = productDiscoveryCandidates(uniqueExactOptions(initial.Items), []recommendationObservedSearch{{sort: core.ProductSortCoupangRanking, page: 1, result: initial}}, 1)
	result.Audit.UniqueExactOptions = len(uniqueExactOptions(initial.Items))
	result.Audit.UniqueModelFamilies = len(result.Discovered)
	result.Audit.UninspectedExactOptions = result.Audit.UniqueExactOptions
	result.Audit.UninspectedModelFamilies = len(result.Discovered)
	result.Warnings = append(result.Warnings, "retained search candidates are not detail-verified recommendations; discovery may include sponsored or sponsorship-unverified listings")
}

func availableFacetChoice(facets []core.ProductFacet, selection core.ProductFacetSelection) bool {
	groups, choices := 0, 0
	enabled := false
	for _, group := range facets {
		if group.Name != selection.Name {
			continue
		}
		groups++
		for _, option := range group.Options {
			if option.Label == selection.Label {
				choices++
				enabled = !option.Disabled
			}
		}
	}
	return groups == 1 && choices == 1 && enabled
}

func cloneFacetCatalog(facets []core.ProductFacet) []core.ProductFacet {
	copy := append([]core.ProductFacet{}, facets...)
	for i := range copy {
		copy[i].Options = append([]core.ProductFacetOption{}, facets[i].Options...)
	}
	return copy
}
