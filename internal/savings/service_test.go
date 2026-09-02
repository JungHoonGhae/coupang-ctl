package savings

import (
	"context"
	"testing"
	"time"

	"github.com/JungHoonGhae/oss-coupangctl/internal/core"
)

func TestPlanKeepsUnverifiedRewardsOutOfGuaranteedEffectivePrice(t *testing.T) {
	service := New()
	service.now = func() time.Time { return time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC) }
	plan, err := service.Plan(context.Background(), core.SavingsPlanRequest{
		PurchaseAmountKRW: 1_000_000,
		EnabledPrograms: []string{core.SavingsProgramCoupangWowCard, core.SavingsProgramKBPayEvent},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.RecommendedRouteID != core.SavingsProgramCoupangWowCard || len(plan.Routes) != 3 {
		t.Fatalf("unexpected plan: %#v", plan)
	}
	wow := plan.Routes[1]
	if wow.EstimatedRewardKRW != 40_000 || wow.Guaranteed || wow.WorstCaseEffectiveAmount != 1_000_000 {
		t.Fatalf("unexpected wow route: %#v", wow)
	}
	if plan.Routes[2].RewardUnits != 50 || !plan.Routes[2].RequiresHumanHandoff {
		t.Fatalf("unexpected KB Pay route: %#v", plan.Routes[2])
	}
}

func TestPlanCanDisableSavingsAndNeverAddsAffiliateRoute(t *testing.T) {
	plan, err := New().Plan(context.Background(), core.SavingsPlanRequest{PurchaseAmountKRW: 50_000, Mode: core.SavingsModeOff})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Routes) != 1 || plan.Routes[0].AffiliateAttribution || plan.RecommendedRouteID != "coupang_direct" {
		t.Fatalf("unexpected disabled plan: %#v", plan)
	}
}
