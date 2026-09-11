package products

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type accessStopSource struct {
	*selectionSource
	searches, details      int
	failSearch, failDetail int
	failure                error
}

func (s *accessStopSource) Search(ctx context.Context, r core.ProductSearchRequest) ([]core.ProductCard, core.ProductCoverage, error) {
	s.searches++
	if s.searches == s.failSearch {
		return nil, core.ProductCoverage{}, errors.Join(s.failure, errors.New("synthetic-private-error-do-not-output"))
	}
	return s.selectionSource.Search(ctx, r)
}

func (s *accessStopSource) Inspect(ctx context.Context, r core.ProductInspectRequest) (core.ProductInspection, error) {
	s.details++
	if s.details == s.failDetail {
		return core.ProductInspection{}, errors.Join(s.failure, errors.New("synthetic-private-error-do-not-output"))
	}
	return s.selectionSource.Inspect(ctx, r)
}

func TestRecommendationAccessFailureStopsFurtherSourceReads(t *testing.T) {
	for _, failure := range []struct {
		name, reason string
		err          error
	}{
		{"denied", "source_access_denied", core.ErrBrowserAccessDenied},
		{"authentication", "source_authentication_required", core.ErrAuthenticationRequired},
	} {
		for _, stage := range []struct {
			name                                         string
			search, detail, searches, details, inspected int
		}{
			{"discovery", 2, 0, 2, 0, 0},
			{"inspection", 0, 2, 5, 2, 1},
		} {
			t.Run(failure.name+"/"+stage.name, func(t *testing.T) {
				source := &accessStopSource{selectionSource: newSelectionSource(3), failSearch: stage.search, failDetail: stage.detail, failure: failure.err}
				result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, SearchPageLimit: 1, DisableAffiliate: true})
				if err != nil {
					t.Fatal("later source failure discarded completed evidence")
				}
				if source.searches != stage.searches || source.details != stage.details {
					t.Fatalf("continued source reads after access failure: searches=%d details=%d", source.searches, source.details)
				}
				if result.Status != core.ProductRecommendationIncomplete || len(result.Discovered) != 3 || len(result.Inspected) != stage.inspected || result.Audit.UninspectedExactOptions != 3-stage.inspected {
					t.Fatal("access failure erased evidence or became complete")
				}
				if result.Audit.InspectionStopReason != failure.reason || stage.search > 0 && result.Audit.DiscoveryStopReason != failure.reason {
					t.Fatal("access reason lost")
				}
				found := false
				for _, action := range result.NextActions {
					if action.Kind == "restore_source_access" && action.Reason == failure.reason {
						found = true
					}
					if action.Kind == "inspect_details" || action.Kind == "inspect_evidence" || action.Kind == "continue_discovery" {
						t.Fatal("suggested more source reads before access is restored")
					}
				}
				if !found {
					t.Fatal("missing source recovery next action")
				}
				if strings.Contains(strings.Join(result.Warnings, " "), "synthetic-private-error") {
					t.Fatal("raw source error leaked")
				}
			})
		}
	}
}

func TestRecommendationInitialAccessFailureRemainsTypedError(t *testing.T) {
	for _, failure := range []error{core.ErrBrowserAccessDenied, core.ErrAuthenticationRequired} {
		source := &accessStopSource{selectionSource: newSelectionSource(3), failSearch: 1, failure: failure}
		_, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, DisableAffiliate: true})
		if !errors.Is(err, failure) || source.searches != 1 || source.details != 0 {
			t.Fatal("initial access failure lost or retried")
		}
	}
}

func TestRecommendationCandidateFailureIsNotGlobalAccessFailure(t *testing.T) {
	source := &accessStopSource{selectionSource: newSelectionSource(3), failDetail: 2, failure: errors.New("synthetic malformed detail")}
	result, err := New(source).Recommend(context.Background(), core.ProductRecommendationRequest{Query: "synthetic", Proceed: true, SearchPageLimit: 1, DisableAffiliate: true})
	if err != nil || source.details != 3 || len(result.Inspected) != 2 || result.Status != core.ProductRecommendationIncomplete {
		t.Fatal("one malformed detail prevented other independent inspections")
	}
}
