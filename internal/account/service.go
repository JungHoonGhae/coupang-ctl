package account

import (
	"context"
	"errors"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

const defaultCashTransactionPages = 50

var ErrSourceUnavailable = errors.New("account benefits source unavailable")

type Source interface {
	Snapshot(context.Context, core.AccountBenefitsRequest) (core.AccountBenefitsSnapshot, error)
}

type CostProvider interface {
	MembershipCosts(context.Context) (core.MembershipCostEvidence, error)
}

type Service struct {
	source Source
	costs  CostProvider
	now    func() time.Time
}

func New(source Source) *Service {
	return &Service{source: source, now: time.Now}
}

func NewWithCosts(source Source, costs CostProvider) *Service {
	return &Service{source: source, costs: costs, now: time.Now}
}

func (s *Service) Snapshot(ctx context.Context, request core.AccountBenefitsRequest) (core.AccountBenefitsSnapshot, error) {
	if request.MaxCashTransactionPages == 0 {
		request.MaxCashTransactionPages = defaultCashTransactionPages
	}
	if err := request.Validate(); err != nil {
		return core.AccountBenefitsSnapshot{}, err
	}
	if s.source == nil {
		return core.AccountBenefitsSnapshot{}, ErrSourceUnavailable
	}
	result, err := s.source.Snapshot(ctx, request)
	if err != nil {
		return core.AccountBenefitsSnapshot{}, errors.Join(ErrSourceUnavailable, err)
	}
	now := s.now().UTC()
	result.SchemaVersion = core.AccountBenefitsSchemaVersion
	result.Visibility = "private_local"
	result.FetchedAt = now
	result.CardProgram = wowCardProgramTerms(now)
	if result.Warnings == nil {
		result.Warnings = []string{}
	}
	if s.costs == nil {
		result.MembershipCosts = unavailableMembershipCosts()
		applyMembershipValue(&result)
		return result, nil
	}
	costs, costErr := s.costs.MembershipCosts(ctx)
	if costErr != nil {
		result.MembershipCosts = unavailableMembershipCosts()
		result.Warnings = append(result.Warnings, "the private local order ledger could not be read, so historical membership costs remain unavailable")
	} else {
		if result.Membership.IsPaidMember && costs.ObservedPaymentCount == 0 && costs.CompleteHistorySync {
			costs.Status = "unavailable_no_explicit_membership_order_metadata"
			costs.Limitations = append(costs.Limitations, "the complete live order history exposed no membership-specific item metadata, so zero observed charges must not be read as zero fees paid")
		}
		result.MembershipCosts = costs
		result.Coverage.MembershipPaymentsObserved = costs.ObservedPaymentCount > 0
	}
	applyMembershipValue(&result)
	return result, nil
}

func unavailableMembershipCosts() core.MembershipCostEvidence {
	return core.MembershipCostEvidence{
		Status: "unavailable", Source: "normalized_order_ledger_explicit_membership_metadata", Provenance: "derived",
		Limitations: []string{"run a complete orders sync before treating observed membership costs as historical coverage"},
	}
}

func applyMembershipValue(result *core.AccountBenefitsSnapshot) {
	costs := result.MembershipCosts
	value := core.MembershipNetValue{
		ObservedBenefitKRW:        result.BenefitUsage.TotalObservedSavingsKRW,
		ConfirmedMembershipFeeKRW: costs.ObservedPaidAmountKRW,
		Status:                    "not_computed_unaligned_windows",
		Provenance:                "not_computed",
		WindowBasis:               "benefit and actual membership-payment windows are not both available",
		MissingEvidence:           []string{},
		Limitations:               []string{"card rewards and published card annual fees are intentionally excluded from the membership-only comparison"},
	}
	if !result.Coverage.BenefitUsageObserved {
		value.MissingEvidence = append(value.MissingEvidence, "coupang_reported_membership_benefit_total")
	}
	if result.BenefitUsage.WindowStatus != "observed" || result.BenefitUsage.WindowKind != "rolling_recent_months" || result.BenefitUsage.WindowMonths < 1 {
		value.MissingEvidence = append(value.MissingEvidence, "benefit_window")
	}
	if result.Membership.CurrentMonthlyFeeKRW <= 0 {
		value.MissingEvidence = append(value.MissingEvidence, "current_monthly_membership_fee")
	}
	value.MissingEvidence = append(value.MissingEvidence, "actual_membership_payments_for_benefit_window")
	if result.Coverage.BenefitUsageObserved && result.BenefitUsage.WindowStatus == "observed" && result.BenefitUsage.WindowKind == "rolling_recent_months" && result.BenefitUsage.WindowMonths > 0 && result.Membership.CurrentMonthlyFeeKRW > 0 {
		value.EstimatedMembershipFeeKRW = result.Membership.CurrentMonthlyFeeKRW * int64(result.BenefitUsage.WindowMonths)
		value.EstimatedNetValueKRW = value.ObservedBenefitKRW - value.EstimatedMembershipFeeKRW
		value.Status = "estimated_current_fee_window"
		value.Provenance = "inferred"
		value.WindowBasis = "source UI labels the benefit total as recent months; comparison cost is current_monthly_fee_krw multiplied by window_months, not observed historical charges"
		value.Limitations = append(value.Limitations, "membership pauses, refunds, free periods, failed charges, and fee changes inside the window are not represented in the estimated cost")
	} else {
		value.Limitations = append(value.Limitations, "no comparison is calculated until the source benefit window and current monthly fee are both observed")
	}
	result.NetValue = value
}

func wowCardProgramTerms(now time.Time) core.WowCardProgramTerms {
	status := "active"
	date := now.In(time.FixedZone("KST", 9*60*60)).Format(time.DateOnly)
	if date < "2026-04-15" {
		status = "scheduled"
	} else if date > "2026-10-15" {
		status = "ended_or_unverified"
	}
	return core.WowCardProgramTerms{
		ProductName: "쿠팡 와우 카드", PublishedAnnualFeeKRW: 20_000,
		CoupangBaseRewardRate: 0.02, CoupangPromotionRewardRate: 0.02, CoupangMaximumRewardRate: 0.04,
		CoupangBaseMonthlyCapKRW: 20_000, CoupangPromotionMonthlyCapKRW: 20_000,
		PromotionFrom: "2026-04-15", PromotionTo: "2026-10-15", PromotionStatus: status,
		InterestFreeInstallmentRewardEligible: false,
		SourceURL:                             "https://m.coupang.com/vm/wowcard", LastVerified: "2026-09-02",
	}
}
