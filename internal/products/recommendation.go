package products

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type recommendationObservedSearch struct {
	sort   core.ProductSort
	page   int
	result core.ProductSearchResult
}

// Operational ceilings, not a claim about an optimal recommendation count.
const recommendationInspectionBudget = 20
const recommendationDocumentBudget = 40
const recommendationTimeout = 5 * time.Minute

var errRecommendationTimeBudget = errors.New("recommendation time budget exhausted")

func recommendationTimeBudgetExpired(ctx context.Context) bool {
	return errors.Is(context.Cause(ctx), errRecommendationTimeBudget)
}

// Classify only typed source-wide failures. A malformed individual document
// must not prevent other candidates from being inspected, and raw errors must
// never become user-facing audit text.
func recommendationAccessStopReason(err error) string {
	switch {
	case errors.Is(err, core.ErrBrowserAccessDenied):
		return "source_access_denied"
	case errors.Is(err, core.ErrAuthenticationRequired):
		return "source_authentication_required"
	default:
		return ""
	}
}

func (s *Service) Recommend(ctx context.Context, request core.ProductRecommendationRequest) (core.ProductRecommendationResult, error) {
	return s.recommendWithTimeBudget(ctx, request, recommendationTimeout)
}

// Keep the production deadline fixed while allowing the complete pipeline to
// exercise deadline handling without a five-minute regression test.
func (s *Service) recommendWithTimeBudget(ctx context.Context, request core.ProductRecommendationRequest, budget time.Duration) (core.ProductRecommendationResult, error) {
	request.Query = strings.TrimSpace(request.Query)
	if err := core.ValidateRequest(request); err != nil {
		return core.ProductRecommendationResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return core.ProductRecommendationResult{}, err
	}
	callerCtx := ctx
	ctx, cancel := context.WithTimeoutCause(ctx, budget, errRecommendationTimeBudget)
	defer cancel()
	if request.ReviewCap == 0 {
		request.ReviewCap = 200
	}
	if request.DiscoveryTarget == 0 {
		request.DiscoveryTarget = 100
	}
	if request.SearchPageLimit == 0 {
		request.SearchPageLimit = 5
	}

	initial, err := s.Search(ctx, recommendationSearchRequest(request, core.ProductSortCoupangRanking, 1, nil))
	if callerErr := callerCtx.Err(); callerErr != nil {
		return core.ProductRecommendationResult{}, callerErr
	}
	if err != nil && !recommendationTimeBudgetExpired(ctx) {
		return core.ProductRecommendationResult{}, err
	}
	result := core.ProductRecommendationResult{
		ComparisonAxes: append([]core.ProductPreferenceAxis{}, request.ComparisonAxes...),
		SchemaVersion:  core.ProductRecommendationSchemaVersion,
		Query:          request.Query,
		CategoryID:     request.CategoryID,
		Questions:      []core.ProductRecommendationQuestion{},
		Facets:         cloneFacetCatalog(initial.Coverage.Facets),
		Candidates:     []core.ProductRecommendationCandidate{},
		Inspected:      []core.ProductRecommendationCandidate{},
		NextActions:    []core.ProductRecommendationNextAction{},
		Discovered:     []core.ProductDiscoveryCandidate{},
		Warnings:       []string{},
		Audit: core.ProductRecommendationAudit{
			DocumentReadBudget: recommendationDocumentBudget, DocumentReadReservations: 1,
			DiscoveryTarget: request.DiscoveryTarget, SearchesRun: 1, SearchPagesRead: 1, ListingsObserved: len(initial.Items),
		},
	}
	if err != nil {
		// The first attempted document never became observed evidence.
		result.Audit.SearchPagesRead = 0
	}
	result.Audit.SourceFacetsObserved = len(result.Facets)
	if err == nil {
		var ready bool
		initial, ready = s.refineRecommendation(ctx, request, initial, &result)
		if callerErr := callerCtx.Err(); callerErr != nil {
			return core.ProductRecommendationResult{}, callerErr
		}
		if !ready {
			return result, nil
		}
		if result.AppliedCategoryID != "" {
			request.CategoryID = result.AppliedCategoryID
		}
	}
	if !request.Proceed && !recommendationTimeBudgetExpired(ctx) {
		result.Questions = unansweredFacetQuestions(result.Facets, request.Answers)
	}
	if len(result.Questions) > 0 {
		result.Status = core.ProductRecommendationNeedsInput
		retainRefinementCandidates(initial, &result)
		result.Warnings = append(result.Warnings,
			"these are optional question candidates discovered from the current source UI; the AI should ask only those that materially change the recommendation",
			"answer relevant questions, mark them unbounded, or set proceed=true to skip the remaining questions; answered facets are not asked again",
		)
		return result, nil
	}

	result, err = s.completeRecommendationResearch(ctx, request, initial, result)
	if callerErr := callerCtx.Err(); callerErr != nil {
		return core.ProductRecommendationResult{}, callerErr
	}
	return result, err
}

func recommendationSearchRequest(request core.ProductRecommendationRequest, sortOrder core.ProductSort, page int, refinement *core.ProductSearchRefinement) core.ProductSearchRequest {
	// Discovery is not budget verification. Missing or stale listing prices
	// must not erase options before their detail evidence can be checked.
	// The declared budget is assessed against each inspected option below;
	// explicit Search requests retain their normal price-filter semantics.
	search := core.ProductSearchRequest{
		DocumentReadLimit: 1,
		Query:             request.Query, CategoryID: request.CategoryID, Limit: 20, IncludeVariants: true,
		Sort: sortOrder, Page: page, DisableAffiliate: request.DisableAffiliate,
	}
	if refinement != nil {
		search.FacetSelections = append([]core.ProductFacetSelection(nil), refinement.AppliedSelections...)
		search.CategoryTrail = append([]string(nil), refinement.CategoryTrail...)
		search.DocumentReadLimit += len(search.FacetSelections) + len(search.CategoryTrail)
	}
	return search
}

// Questions are optional preference prompts, not authorization gates. An
// answer suppresses only its own facet; it does not establish a verified hard
// condition. Filter before applying the presentation limit so answered facets
// cannot hide the next unanswered one.
func unansweredFacetQuestions(facets []core.ProductFacet, answers []core.ProductRecommendationAnswer) []core.ProductRecommendationQuestion {
	answered := make(map[string]bool, len(answers))
	for _, answer := range answers {
		if answer.Unbounded || strings.TrimSpace(answer.Choice) != "" {
			answered[strings.TrimSpace(answer.QuestionID)] = true
		}
	}
	remaining := make([]core.ProductFacet, 0, len(facets))
	for _, facet := range facets {
		if !answered["facet:"+strings.TrimSpace(facet.Name)] {
			remaining = append(remaining, facet)
		}
	}
	return questionsFromSourceFacets(remaining)
}

func questionsFromSourceFacets(facets []core.ProductFacet) []core.ProductRecommendationQuestion {
	questions := make([]core.ProductRecommendationQuestion, 0, len(facets))
	for _, facet := range facets {
		options := make([]core.ProductRecommendationOption, 0, len(facet.Options))
		for _, option := range facet.Options {
			label := strings.TrimSpace(option.Label)
			if label == "" || option.Selected || option.Disabled {
				continue
			}
			options = append(options, core.ProductRecommendationOption{Value: label, Label: label})
		}
		if len(options) == 0 {
			continue
		}
		questions = append(questions, core.ProductRecommendationQuestion{
			ID: "facet:" + facet.Name, Prompt: facet.Name + "에서 원하는 조건이 있나요? 필요 없으면 건너뛰세요.",
			Kind: core.ProductRecommendationValueChoice, Required: false, AllowUnbounded: true,
			Options: options, SourceFacet: facet.Name,
		})
		if len(questions) >= 24 {
			break
		}
	}
	return questions
}

func (s *Service) completeRecommendationResearch(ctx context.Context, request core.ProductRecommendationRequest, initial core.ProductSearchResult, result core.ProductRecommendationResult) (core.ProductRecommendationResult, error) {
	var selections []core.ProductFacetSelection
	if result.Refinement != nil {
		selections = result.Refinement.AppliedSelections
	}
	// Sidebar application currently supports first-page navigation only. Never
	// silently drop selected conditions in order to fetch another page.
	if len(selections) > 0 && request.SearchPageLimit > 1 {
		request.SearchPageLimit = 1
		result.Warnings = append(result.Warnings, "verified sidebar refinement is limited to the first page of each supported sort; source exhaustion is not established")
	}
	allSorts := []core.ProductSort{
		core.ProductSortCoupangRanking, core.ProductSortSales, core.ProductSortLatest,
		core.ProductSortPriceAsc, core.ProductSortPriceDesc,
	}
	observations := []recommendationObservedSearch{{sort: core.ProductSortCoupangRanking, page: 1, result: initial}}
	incomplete := false
	uniqueCount := func() int {
		items := make([]core.ProductCard, 0)
		for _, observation := range observations {
			items = append(items, observation.result.Items...)
		}
		return len(uniqueModelFamilies(uniqueExactOptions(items)))
	}
	type searchOutcome struct {
		sort   core.ProductSort
		page   int
		result core.ProductSearchResult
		err    error
	}
	// A short (or empty) filtered list says nothing about the next source page.
	// Until the adapter reports explicit no-results, continue within the page
	// budget. Missing pagination evidence must not masquerade as exhaustion.
	active := map[core.ProductSort]bool{core.ProductSortCoupangRanking: !initial.Coverage.SourceNoResults}
	knownOptions := make(map[core.ProductReference]bool)
	for _, item := range initial.Items {
		knownOptions[item.Reference] = true
	}
	discoveryDocumentLimited := false
	sourceAccessStop := ""
	fetchRound := func(page int, sorts []core.ProductSort) []searchOutcome {
		outcomes := make([]searchOutcome, 0, len(sorts))
		for _, sortOrder := range sorts {
			if ctx.Err() != nil || sourceAccessStop != "" {
				break
			}
			// Keep one document per known option (within inspection/review
			// ceilings). Discovery must not consume every slot before any detail
			// can be read. This is resource scheduling, not preference ranking.
			minimumDetailSlots := min(len(knownOptions), recommendationInspectionBudget, request.ReviewCap)
			search := recommendationSearchRequest(request, sortOrder, page, result.Refinement)
			if recommendationDocumentBudget-result.Audit.DocumentReadReservations < minimumDetailSlots+search.DocumentReadLimit {
				discoveryDocumentLimited = true
				result.Audit.DocumentBudgetConstrained = true
				break
			}
			result.Audit.DocumentReadReservations += search.DocumentReadLimit
			// A selected minimized tab is one shared navigation surface.
			value, err := s.Search(ctx, search)
			if err == nil && value.Coverage.AppliedCategoryID != "" && value.Coverage.AppliedCategoryID != request.CategoryID {
				err = errors.New("source category changed during comparison research")
				sourceAccessStop = "source_category_changed"
			}
			outcomes = append(outcomes, searchOutcome{sort: sortOrder, page: page, result: value, err: err})
			if reason := recommendationAccessStopReason(err); reason != "" {
				sourceAccessStop = reason
				break
			}
			if err != nil && len(selections) > 0 {
				if sourceAccessStop == "" {
					sourceAccessStop = "source_filter_unverified"
				}
				break
			}
			if err == nil {
				for _, item := range value.Items {
					knownOptions[item.Reference] = true
				}
			}
		}
		return outcomes
	}
	remainingPageOneSorts := allSorts[1:]
	for _, outcome := range fetchRound(1, remainingPageOneSorts) {
		result.Audit.SearchesRun++
		if outcome.err != nil {
			incomplete = true
			result.Warnings = append(result.Warnings, "one comparison search order was unavailable")
			continue
		}
		result.Audit.SearchPagesRead++
		observations = append(observations, recommendationObservedSearch{sort: outcome.sort, page: outcome.page, result: outcome.result})
		active[outcome.sort] = !outcome.result.Coverage.SourceNoResults
	}
	for page := 2; ctx.Err() == nil && sourceAccessStop == "" && !discoveryDocumentLimited && page <= request.SearchPageLimit && uniqueCount() < request.DiscoveryTarget; page++ {
		pageSorts := make([]core.ProductSort, 0, len(allSorts))
		for _, sortOrder := range allSorts {
			if active[sortOrder] {
				pageSorts = append(pageSorts, sortOrder)
			}
		}
		if len(pageSorts) == 0 {
			break
		}
		for _, outcome := range fetchRound(page, pageSorts) {
			result.Audit.SearchesRun++
			if outcome.err != nil {
				incomplete = true
				active[outcome.sort] = false
				result.Warnings = append(result.Warnings, "one paged comparison search was unavailable")
				continue
			}
			result.Audit.SearchPagesRead++
			observations = append(observations, recommendationObservedSearch{sort: outcome.sort, page: outcome.page, result: outcome.result})
			active[outcome.sort] = !outcome.result.Coverage.SourceNoResults
		}
	}

	if err := ctx.Err(); err != nil && !recommendationTimeBudgetExpired(ctx) {
		return core.ProductRecommendationResult{}, err
	}
	allItems := make([]core.ProductCard, 0)
	for _, observation := range observations {
		if len(observation.result.UnavailableFilterFields) > 0 || observation.result.Coverage.RejectedItems > 0 {
			incomplete = true
		}
		result.Audit.ListingsObserved += len(observation.result.Items)
		allItems = append(allItems, observation.result.Items...)
	}
	result.Audit.ListingsObserved -= len(initial.Items)
	// Keep the last verified refinement catalog. Sorting observations are
	// comparison evidence, not additional choices in the current sidebar.
	result.Audit.SourceFacetsObserved = len(result.Facets)
	if incomplete {
		result.Warnings = append(result.Warnings, "discovery coverage is incomplete; missing source or filter evidence is not proof that no alternatives exist")
	}
	exact := uniqueExactOptions(allItems)
	result.Audit.UniqueExactOptions = len(exact)
	families := uniqueModelFamilies(exact)
	result.Audit.UniqueModelFamilies = len(families)
	result.Discovered = productDiscoveryCandidates(exact, observations, len(allSorts))
	switch {
	case recommendationTimeBudgetExpired(ctx):
		result.Audit.DiscoveryStopReason = "time_budget_reached"
		incomplete = true
	case sourceAccessStop != "":
		result.Audit.DiscoveryStopReason = sourceAccessStop
		incomplete = true
	case incomplete:
		result.Audit.DiscoveryStopReason = "source_read_incomplete"
	case discoveryDocumentLimited:
		result.Audit.DiscoveryStopReason = "document_budget_reserved_for_inspection"
		if result.Audit.DocumentReadReservations == recommendationDocumentBudget {
			result.Audit.DiscoveryStopReason = "document_budget_reached"
		}
		incomplete = true
	case !anyActiveSearch(active):
		result.Audit.DiscoveryStopReason = "source_results_exhausted"
	case len(families) >= request.DiscoveryTarget:
		result.Audit.DiscoveryStopReason = "unique_model_target_reached"
		incomplete = true
	default:
		result.Audit.DiscoveryStopReason = "search_page_limit_reached"
		incomplete = true
	}
	if anyActiveSearch(active) && sourceAccessStop == "" {
		result.Warnings = append(result.Warnings, "source pagination has not been exhausted; discovery stopped within an operational budget, not because no further alternatives exist")
	}

	inspectionLimit := recommendationInspectionBudget
	result.Audit.InspectionBudget = inspectionLimit
	if inspectionLimit > len(exact) {
		inspectionLimit = len(exact)
	}
	if inspectionLimit > request.ReviewCap {
		inspectionLimit = request.ReviewCap
	}
	result.Audit.InspectionStopReason = "discovered_candidates_inspected"
	if inspectionLimit < len(exact) {
		incomplete = true
		result.Audit.InspectionStopReason = "inspection_budget_reached"
		if request.ReviewCap < recommendationInspectionBudget {
			// Detail reads currently require at least one review slot. Preserve
			// the explicit review budget rather than silently defaulting zero.
			result.Audit.InspectionStopReason = "review_budget_reached"
		}
		result.Warnings = append(result.Warnings, "uninspected discovered candidates remain because an operational budget was reached; recommendation sufficiency is not established")
	}
	if remaining := recommendationDocumentBudget - result.Audit.DocumentReadReservations; inspectionLimit > remaining {
		inspectionLimit = remaining
		incomplete = true
		result.Audit.DocumentBudgetConstrained = true
		result.Audit.InspectionStopReason = "document_budget_reached"
	}
	reviewsRemaining := request.ReviewCap
	reviewCounts := recommendationReviewCounts{}
	conditions := append([]core.ProductRecommendationCondition(nil), request.RequiredConditions...)
	if request.MaxPrice > 0 {
		maximum := request.MaxPrice
		conditions = append([]core.ProductRecommendationCondition{{ID: "max_price", Field: "price.current_amount", Operator: "lte", Integer: &maximum}}, conditions...)
	}
	inspectedFamilies := make(map[string]struct{})
	// Breadth first across product groups, then their alternate options. This
	// operational schedule prevents a group with many variants consuming every
	// slot; it is not a preference ranking or a sufficiency claim.
	inspectionQueue := interleaveDiscoveryOptions(result.Discovered)
	for index, product := range inspectionQueue[:inspectionLimit] {
		if sourceAccessStop != "" {
			break
		}
		if err := ctx.Err(); err != nil {
			if recommendationTimeBudgetExpired(ctx) {
				break
			}
			return core.ProductRecommendationResult{}, err
		}
		remainingInspections := inspectionLimit - index
		// Allocate at least the detail document to each queued option, then
		// distribute remaining auxiliary slots without exceeding the total.
		// Never refund a failed call: it may already have made all allowed reads.
		documentLimit := min(3, (recommendationDocumentBudget-result.Audit.DocumentReadReservations+remainingInspections-1)/remainingInspections)
		result.Audit.DocumentReadReservations += documentLimit
		if documentLimit < 3 {
			result.Audit.DocumentBudgetConstrained = true
		}
		reviewLimit := (reviewsRemaining + remainingInspections - 1) / remainingInspections
		// ProductInspectRequest.Validate permits at most 20 reviews per detail read.
		if reviewLimit > 20 {
			reviewLimit = 20
		}
		result.Audit.InspectionAttempts++
		inspection, err := s.Inspect(ctx, core.ProductInspectRequest{
			DocumentReadLimit: documentLimit,
			ProductID:         product.Reference.ProductID, ItemID: product.Reference.ItemID,
			VendorItemID: product.Reference.VendorItemID, ReviewLimit: reviewLimit, DetailImageLimit: 20, DisableAffiliate: request.DisableAffiliate,
		})
		if err != nil {
			incomplete = true
			result.Audit.InspectionStopReason = "source_read_incomplete"
			if reason := recommendationAccessStopReason(err); reason != "" {
				sourceAccessStop = reason
				break
			}
			result.Warnings = append(result.Warnings, "one exact candidate could not be inspected; other options remain separate candidates")
			continue
		}
		candidate := genericRecommendationCandidate(inspection)
		if len(inspection.Coverage.BudgetOmittedFields) > 0 {
			incomplete = true
			if result.Audit.InspectionStopReason == "discovered_candidates_inspected" {
				result.Audit.InspectionStopReason = "auxiliary_document_budget_limited"
			}
		}
		result.Audit.DetailsInspected++
		inspectedFamilies[product.Reference.ProductID] = struct{}{}
		result.Audit.ReviewsExamined += candidate.ReviewsExamined
		reviewCounts.add(candidate)
		result.Audit.DetailImagesFound += candidate.DetailImagesFound
		reviewsRemaining -= candidate.ReviewsExamined
		if reviewsRemaining < 0 {
			reviewsRemaining = 0
		}
		unmet, unknown := "", ""
		for _, condition := range conditions {
			assessment := core.AssessProductCondition(inspection, condition)
			candidate.Conditions = append(candidate.Conditions, assessment)
			switch assessment.Status {
			case core.ProductConditionUnknown:
				incomplete = true
				candidate.MissingEvidence = append(candidate.MissingEvidence, assessment.MissingEvidence...)
				if unknown == "" {
					unknown = "required_condition_unverified"
					if condition.ID == "max_price" {
						unknown = "budget_condition_unverified"
					}
				}
			case core.ProductConditionUnmet:
				if unmet == "" {
					unmet = "required_condition_unmet"
					if condition.ID == "max_price" {
						unmet = "over_budget"
					}
				}
			}
		}
		candidate.ExclusionReason = unmet
		if unmet == "" {
			candidate.ExclusionReason = unknown
		}
		result.Inspected = append(result.Inspected, candidate)
		if candidate.ExclusionReason != "" {
			continue
		}
		result.Candidates = append(result.Candidates, candidate)
	}
	if recommendationTimeBudgetExpired(ctx) {
		incomplete = true
		result.Audit.InspectionStopReason = "time_budget_reached"
	}
	if sourceAccessStop != "" {
		incomplete = true
		result.Audit.InspectionStopReason = sourceAccessStop
		if sourceAccessStop == "source_category_changed" {
			result.Warnings = append(result.Warnings, "comparison research stopped because the source category changed; retained candidates belong only to previously verified scope; rediscover category choices before continuing")
		} else {
			result.Warnings = append(result.Warnings,
				"source access stopped further search and detail reads; previously completed evidence is retained but does not establish a complete recommendation; resolve the reported access state in the selected session before requesting further reads",
			)
		}
	}
	result.Audit.UninspectedModelFamilies = len(families) - len(inspectedFamilies)
	result.Audit.UninspectedExactOptions = len(exact) - result.Audit.DetailsInspected
	reviewCounts.apply(&result.Audit)
	if err := ctx.Err(); err != nil && !recommendationTimeBudgetExpired(ctx) {
		return core.ProductRecommendationResult{}, err
	}
	if request.UsePurchaseHistory {
		refs := make([]core.ProductReference, 0, len(result.Candidates))
		for _, candidate := range result.Candidates {
			refs = append(refs, candidate.Product.Reference)
		}
		purchaseContext := core.RecommendationPurchaseContext{
			Visibility: "private_local", Status: "unavailable",
			Matches:     []core.RecommendationPurchaseMatch{},
			Limitations: []string{"local purchase evidence could not be read; no preference or reorder need was inferred"},
		}
		if s.purchaseHistory != nil && ctx.Err() == nil {
			value, err := s.purchaseHistory.RecommendationPurchaseContext(ctx, refs)
			if err == nil {
				purchaseContext = value
			}
		}
		if err := ctx.Err(); err != nil && !recommendationTimeBudgetExpired(ctx) {
			return core.ProductRecommendationResult{}, err
		}
		result.PurchaseContext = &purchaseContext
		if purchaseContext.Status != "available" {
			incomplete = true
			result.Warnings = append(result.Warnings, "requested purchase context is partial or unavailable; do not claim complete personal history")
		}
	}
	// Presentation cannot erase investigation evidence or alter history scope.
	if applyPreferenceComparison(&result, request.ComparisonAxes) {
		incomplete = true
	}
	finalizeRecommendationConclusions(&result, conditions)
	result.Audit.ComparisonCandidates = len(result.Candidates)
	if request.MaxItems > 0 && len(result.Candidates) > request.MaxItems {
		result.Candidates = result.Candidates[:request.MaxItems]
	}
	result.Audit.DisplayedCandidates = len(result.Candidates)
	result.Audit.PresentationOmittedCandidates = result.Audit.ComparisonCandidates - result.Audit.DisplayedCandidates
	if recommendationTimeBudgetExpired(ctx) {
		incomplete = true
		result.Audit.TimeBudgetExhausted = true
		result.Warnings = append(result.Warnings, "internal research time budget was exhausted; completed evidence is retained, no further source or history reads were started, and recommendation sufficiency is not established")
	}
	switch {
	case incomplete:
		result.Status = core.ProductRecommendationIncomplete
	case len(result.Candidates) == 0:
		result.Status = core.ProductRecommendationNoMatches
	default:
		result.Status = core.ProductRecommendationComplete
	}
	result.Warnings = append(result.Warnings,
		"discovery may include sponsored or sponsorship-unverified listings; source positions are not quality or sales evidence",
		"discovery listing prices do not enforce the declared budget; budget eligibility is assessed from inspected option evidence, and uninspected options remain unverified",
		"document_read_reservations bound explicit source attempts including auxiliary reads; they are not observed network counts and exclude automatic browser subresources",
		"source facet choices are applied only after catalog validation and selected-state verification; sidebar selection does not establish detailed product suitability",
		"category-specific fit must be reasoned from the returned inspection evidence instead of a hard-coded universal score",
		"detail image URLs were counted, but no image was claimed as visually reviewed by this workflow",
	)
	return result, nil
}

func uniqueExactOptions(items []core.ProductCard) []core.ProductCard {
	result := make([]core.ProductCard, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		key := item.Reference.ProductID + "/" + item.Reference.ItemID + "/" + item.Reference.VendorItemID
		if item.Reference.ProductID == "" {
			key = item.URL
		}
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	return result
}

func uniqueModelFamilies(items []core.ProductCard) []core.ProductCard {
	result := make([]core.ProductCard, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		family := item.Reference.ProductID
		if family == "" {
			family = item.URL
		}
		if _, exists := seen[family]; exists {
			continue
		}
		seen[family] = struct{}{}
		result = append(result, item)
	}
	return result
}

func productDiscoveryCandidates(exact []core.ProductCard, observations []recommendationObservedSearch, axisTotal int) []core.ProductDiscoveryCandidate {
	families := uniqueModelFamilies(exact)
	result := make([]core.ProductDiscoveryCandidate, 0, len(families))
	indexByFamily := make(map[string]int, len(families))
	for _, product := range families {
		key := product.Reference.ProductID
		if key == "" {
			key = product.URL
		}
		indexByFamily[key] = len(result)
		result = append(result, core.ProductDiscoveryCandidate{
			Product: product, Options: []core.ProductCard{}, Appearances: []core.ProductDiscoveryAppearance{}, Provenance: "derived_from_observed_search_positions",
		})
	}
	for _, product := range exact {
		key := product.Reference.ProductID
		if key == "" {
			key = product.URL
		}
		index := indexByFamily[key]
		result[index].Options = append(result[index].Options, product)
	}
	seenAxis := make([]map[core.ProductSort]bool, len(result))
	bestPosition := make([]map[core.ProductSort]int, len(result))
	for index := range result {
		seenAxis[index] = make(map[core.ProductSort]bool)
		bestPosition[index] = make(map[core.ProductSort]int)
	}
	for _, observation := range observations {
		for _, product := range observation.result.Items {
			key := product.Reference.ProductID
			if key == "" {
				key = product.URL
			}
			index, exists := indexByFamily[key]
			if !exists {
				continue
			}
			position := product.SearchPosition
			result[index].Appearances = append(result[index].Appearances, core.ProductDiscoveryAppearance{
				Sort: observation.sort, Page: observation.page, Position: position, RankSource: product.RankSource,
			})
			seenAxis[index][observation.sort] = true
			derivedPosition := (observation.page-1)*20 + position
			if position <= 0 {
				continue
			}
			if previous, found := bestPosition[index][observation.sort]; !found || derivedPosition < previous {
				bestPosition[index][observation.sort] = derivedPosition
			}
		}
	}
	for index := range result {
		result[index].AxisCount = len(seenAxis[index])
		if axisTotal > 0 {
			result[index].CrossListCoverage = float64(result[index].AxisCount) / float64(axisTotal)
		}
		for _, position := range bestPosition[index] {
			result[index].ReciprocalRankFusion += 1 / float64(60+position)
		}
	}
	return result
}

func interleaveDiscoveryOptions(groups []core.ProductDiscoveryCandidate) []core.ProductCard {
	result := make([]core.ProductCard, 0)
	for optionIndex := 0; ; optionIndex++ {
		added := false
		for _, group := range groups {
			if optionIndex < len(group.Options) {
				result = append(result, group.Options[optionIndex])
				added = true
			}
		}
		if !added {
			return result
		}
	}
}

func anyActiveSearch(active map[core.ProductSort]bool) bool {
	for _, available := range active {
		if available {
			return true
		}
	}
	return false
}

func genericRecommendationCandidate(inspection core.ProductInspection) core.ProductRecommendationCandidate {
	candidate := core.ProductRecommendationCandidate{
		ComparisonValues: []core.ProductComparableValue{}, Comparisons: []core.ProductPairComparison{}, ComparisonReason: "comparison_axes_not_requested",
		Product: inspection.Product, Inspection: inspection,
		Status: core.ProductRecommendationNeedsVerification,
		Claims: []core.ProductRecommendationClaim{}, MissingEvidence: []string{"category_specific_fit"},
		Conditions:      []core.ProductConditionAssessment{},
		ReviewsExamined: len(inspection.Reviews), ReviewsAvailable: inspection.AvailableReviewCount(),
		DetailImagesFound: len(inspection.DetailImages),
	}
	if candidate.ReviewsAvailable == nil {
		candidate.MissingEvidence = append(candidate.MissingEvidence, "review_count")
	} else {
		candidate.ReviewCountScope = inspection.Product.ReviewScope
		if candidate.ReviewCountScope == "" {
			candidate.ReviewCountScope = "unknown"
			candidate.MissingEvidence = append(candidate.MissingEvidence, "review_count_scope")
		}
	}
	if contains(inspection.Product.ObservedFields, "price.current_amount") {
		if evidence, ok := inspection.Product.EvidenceFor("price.current_amount"); ok {
			candidate.Claims = append(candidate.Claims, core.ProductRecommendationClaim{Field: "current_price", Value: strconv.FormatInt(inspection.Product.Price.CurrentAmount, 10) + " " + inspection.Product.Price.Currency, Provenance: evidence.Provenance, Source: evidence.Source, Evidence: &evidence})
		} else {
			candidate.MissingEvidence = append(candidate.MissingEvidence, "price.current_amount.provenance")
		}
	}
	if contains(inspection.Product.ObservedFields, "rating") {
		if evidence, ok := inspection.Product.EvidenceFor("rating"); ok {
			candidate.Claims = append(candidate.Claims, core.ProductRecommendationClaim{Field: "rating", Value: fmt.Sprintf("%.1f", inspection.Product.Rating), Provenance: evidence.Provenance, Source: evidence.Source, Evidence: &evidence})
		} else {
			candidate.MissingEvidence = append(candidate.MissingEvidence, "rating.provenance")
		}
	}
	if len(inspection.SelectedOptions) > 0 {
		if evidence, ok := inspection.EvidenceFor("selected_options"); ok {
			candidate.Claims = append(candidate.Claims, core.ProductRecommendationClaim{Field: "selected_options", Value: strings.Join(inspection.SelectedOptions, " / "), Provenance: evidence.Provenance, Source: evidence.Source, Evidence: &evidence})
		} else {
			candidate.MissingEvidence = append(candidate.MissingEvidence, "selected_options.provenance")
		}
	}
	candidate.MissingEvidence = append(candidate.MissingEvidence, inspection.Coverage.UnavailableFields...)
	return candidate
}
