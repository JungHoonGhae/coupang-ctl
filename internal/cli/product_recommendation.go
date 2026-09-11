package cli

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type productRecommendationWorkflow interface {
	Recommend(context.Context, core.ProductRecommendationRequest) (core.ProductRecommendationResult, error)
}

func runProductRecommendation(ctx context.Context, args []string, stdout io.Writer, workflow productWorkflow) error {
	const usage = "usage: coupangctl products recommend (--query TEXT | --category-id ID) [--proceed] [--max-price KRW] [--limit N] [--review-cap N] [--discovery-target N] [--search-page-limit N] [--answers-json JSON] [--conditions-json JSON] [--comparison-axes-json JSON] [--use-purchase-history] [--no-affiliate]"
	flags := newFlagSet("products recommend")
	query := flags.String("query", "", "product request to research")
	categoryID := flags.String("category-id", "", "observed numeric Coupang category ID, instead of query")
	proceed := flags.Bool("proceed", false, "skip remaining optional facet questions; comparison continues automatically when none remain")
	maxPrice := flags.Int64("max-price", 0, "maximum observed current price in KRW")
	limit := flags.Int("limit", 0, "optional presentation cap, 1 through 200; 0 returns all retained candidates without changing investigation depth")
	reviewCap := flags.Int("review-cap", 200, "total review sample bound")
	discoveryTarget := flags.Int("discovery-target", 100, "target unique product families")
	pageLimit := flags.Int("search-page-limit", 5, "maximum pages per source sort")
	answers := flags.String("answers-json", "[]", "ordered observed facet answers with rediscovery; repeat facet:카테고리 to follow an observed path while retaining the query or verified category scope (Camofox)")
	conditions := flags.String("conditions-json", "[]", "typed explicit hard conditions; unsupported fields fail before source access")
	axes := flags.String("comparison-axes-json", "[]", "explicit comparison fields and preference directions; no default axes or weights")
	noAffiliate := flags.Bool("no-affiliate", false, "return only canonical product URLs")
	useHistory := flags.Bool("use-purchase-history", false, "include private local purchase aggregates and sync coverage, without inferring preference")
	if isFlagHelp(args) {
		return writeCommandFlagHelp(stdout, flags, usage)
	}
	if err := parseFlags(flags, args, usage); err != nil {
		return err
	}
	request := core.ProductRecommendationRequest{Query: strings.TrimSpace(*query), CategoryID: *categoryID, Proceed: *proceed, MaxPrice: *maxPrice,
		MaxItems: *limit, ReviewCap: *reviewCap, DiscoveryTarget: *discoveryTarget, SearchPageLimit: *pageLimit, DisableAffiliate: *noAffiliate, UsePurchaseHistory: *useHistory}
	if len(*answers) > 64<<10 {
		return core.WithErrorCode("invalid_command", errors.New(usage))
	}
	decoder := json.NewDecoder(strings.NewReader(*answers))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request.Answers); err != nil {
		return core.WithErrorCode("invalid_command", errors.New(usage))
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return core.WithErrorCode("invalid_command", errors.New(usage))
	}
	if len(*conditions) > 64<<10 {
		return core.WithErrorCode("invalid_command", errors.New(usage))
	}
	conditionDecoder := json.NewDecoder(strings.NewReader(*conditions))
	conditionDecoder.DisallowUnknownFields()
	if err := conditionDecoder.Decode(&request.RequiredConditions); err != nil {
		return core.WithErrorCode("invalid_command", errors.New(usage))
	}
	if err := conditionDecoder.Decode(&trailing); err != io.EOF {
		return core.WithErrorCode("invalid_command", errors.New(usage))
	}
	if len(*axes) > 64<<10 {
		return core.WithErrorCode("invalid_command", errors.New(usage))
	}
	axisDecoder := json.NewDecoder(strings.NewReader(*axes))
	axisDecoder.DisallowUnknownFields()
	if err := axisDecoder.Decode(&request.ComparisonAxes); err != nil {
		return core.WithErrorCode("invalid_command", errors.New(usage))
	}
	if err := axisDecoder.Decode(&trailing); err != io.EOF {
		return core.WithErrorCode("invalid_command", errors.New(usage))
	}
	if err := core.ValidateRequest(request); err != nil {
		return err
	}
	provider, ok := workflow.(productRecommendationWorkflow)
	if !ok {
		return errors.New("product recommendation provider unavailable")
	}
	result, err := provider.Recommend(ctx, request)
	if err != nil {
		return err
	}
	return writeJSON(stdout, result)
}
