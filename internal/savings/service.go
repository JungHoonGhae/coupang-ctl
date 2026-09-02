package savings

import (
	"context"
	"math"
	"time"

	"github.com/JungHoonGhae/oss-coupangctl/internal/core"
)

const (
	wowCardPromotionEnd = "2026-10-15"
	wowCardRate         = 0.04
	wowCardMonthlyCap   = int64(40_000)
)

type Service struct {
	now func() time.Time
}

func New() *Service {
	return &Service{now: time.Now}
}

func (s *Service) Plan(ctx context.Context, request core.SavingsPlanRequest) (core.SavingsPlan, error) {
	if err := ctx.Err(); err != nil {
		return core.SavingsPlan{}, err
	}
	if request.Mode == "" {
		request.Mode = core.SavingsModeAuto
	}
	if err := request.Validate(); err != nil {
		return core.SavingsPlan{}, err
	}
	now := s.now().UTC()
	direct := directRoute(request.PurchaseAmountKRW)
	plan := core.SavingsPlan{
		SchemaVersion: 1, Mode: request.Mode, BaselineAmountKRW: request.PurchaseAmountKRW,
		Currency: "KRW", GeneratedAt: now, RecommendedRouteID: direct.ID,
		Recommendation: "use the verified direct amount unless an eligible reward route is confirmed",
		Routes: []core.SavingsRoute{direct},
		Disclosure: []string{
			"external rewards are not subtracted from the observed Coupang price unless their eligibility and remaining cap are known",
			"affiliate attribution is disabled by default and no affiliate route is present in this plan",
			"coupangctl never submits the final order or payment",
		},
	}
	if request.Mode == core.SavingsModeOff {
		plan.Recommendation = "savings optimization is off; use the canonical direct route"
		return plan, nil
	}
	enabled := enabledPrograms(request.EnabledPrograms)
	wow := wowCardRoute(request.PurchaseAmountKRW, enabled[core.SavingsProgramCoupangWowCard], now)
	kbpay := kbPayRoute(request.PurchaseAmountKRW, enabled[core.SavingsProgramKBPayEvent])
	plan.Routes = append(plan.Routes, wow, kbpay)
	if wow.Eligibility == "declared_eligible" && wow.Status == "active" {
		plan.RecommendedRouteID = wow.ID
		plan.Recommendation = "the declared Wow Card route has the largest currently quantifiable reward, but verify the remaining monthly cap before treating it as effective price"
	} else if kbpay.Eligibility == "declared_eligible" {
		plan.RecommendedRouteID = kbpay.ID
		plan.Recommendation = "the declared KB Pay event may add a post-purchase reward, but its current availability and reward value require a human handoff and are not a guaranteed discount"
	}
	return plan, nil
}

func directRoute(amount int64) core.SavingsRoute {
	return core.SavingsRoute{
		ID: "coupang_direct", Kind: "observed_price", Status: "active", Eligibility: "eligible",
		Guaranteed: true, Automatic: true, BestCaseEffectiveAmount: amount, WorstCaseEffectiveAmount: amount,
		Conditions: []string{"uses the canonical Coupang product route and currently observed amount"},
		SourceURL: "https://www.coupang.com/", LastVerified: "2026-09-02",
	}
}

func wowCardRoute(amount int64, isEnabled bool, now time.Time) core.SavingsRoute {
	reward := int64(math.Round(float64(amount) * wowCardRate))
	if reward > wowCardMonthlyCap {
		reward = wowCardMonthlyCap
	}
	status := "active"
	if now.After(time.Date(2026, time.October, 15, 23, 59, 59, 0, time.FixedZone("Asia/Seoul", 9*60*60))) {
		status = "expired_needs_refresh"
	}
	eligibility := "not_enabled"
	if isEnabled {
		eligibility = "declared_eligible"
	}
	return core.SavingsRoute{
		ID: core.SavingsProgramCoupangWowCard, Kind: "post_purchase_cashback", Status: status, Eligibility: eligibility,
		Guaranteed: false, Automatic: true, EstimatedRewardKRW: reward, RewardRangeKRW: []int64{0, reward},
		BestCaseEffectiveAmount: amount - reward, WorstCaseEffectiveAmount: amount,
		Conditions: []string{
			"requires an eligible Coupang Wow Card and qualifying Coupang transaction",
			"4% combines product and temporary promotion benefits through " + wowCardPromotionEnd,
			"monthly reward cap and previously used benefit amount are not observable",
		},
		SourceURL: "https://m.coupang.com/vm/wowcard", LastVerified: "2026-09-02",
	}
}

func kbPayRoute(amount int64, isEnabled bool) core.SavingsRoute {
	eligibility := "not_enabled"
	if isEnabled {
		eligibility = "declared_eligible"
	}
	entries := int(amount / 10_000)
	if entries > 50 {
		entries = 50
	}
	return core.SavingsRoute{
		ID: core.SavingsProgramKBPayEvent, Kind: "post_purchase_variable_reward", Status: "live_verification_required", Eligibility: eligibility,
		Guaranteed: false, Automatic: false, RequiresHumanHandoff: true,
		BestCaseEffectiveAmount: amount, WorstCaseEffectiveAmount: amount,
		RewardUnits: entries, RewardUnitName: "scratch_lottery_entries",
		Conditions: []string{
			"requires KB Pay login and a qualifying entry path before product search and purchase",
			"the event page warns that prior direct or other-app visits, a prefilled cart, or inactivity can prevent attribution",
			"reward is delayed and variable, so it is not subtracted from effective price",
		},
		EntryURL: "https://kbpay.mycash.company/services/myCashcoupangEvent",
		SourceURL: "https://kbpay.mycash.company/services/myCashcoupangEvent", LastVerified: "2026-09-02",
	}
}

func enabledPrograms(values []string) map[string]bool {
	result := map[string]bool{}
	for _, value := range values {
		result[value] = true
	}
	return result
}
