package account

import (
	"context"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

type syntheticSource struct {
	snapshot core.AccountBenefitsSnapshot
}

type syntheticCosts struct {
	evidence core.MembershipCostEvidence
}

func (s syntheticSource) Snapshot(context.Context, core.AccountBenefitsRequest) (core.AccountBenefitsSnapshot, error) {
	return s.snapshot, nil
}

func (s syntheticCosts) MembershipCosts(context.Context) (core.MembershipCostEvidence, error) {
	return s.evidence, nil
}

func TestSnapshotAddsVersionedProgramTermsWithoutInventingPaidFees(t *testing.T) {
	service := New(syntheticSource{snapshot: core.AccountBenefitsSnapshot{
		BenefitUsage: core.WowBenefitUsage{TotalObservedSavingsKRW: 100_000},
	}})
	service.now = func() time.Time { return time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC) }

	got, err := service.Snapshot(context.Background(), core.AccountBenefitsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != 2 || got.Visibility != "private_local" || got.CardProgram.PublishedAnnualFeeKRW != 20_000 || got.CardProgram.PromotionStatus != "active" {
		t.Fatalf("unexpected account envelope: %#v", got)
	}
	if got.MembershipCosts.Status != "unavailable" || got.NetValue.ConfirmedMembershipFeeKRW != 0 || got.NetValue.ConfirmedCardAnnualFeeKRW != 0 || got.NetValue.EstimatedNetValueKRW != 0 {
		t.Fatalf("unobserved costs were invented: %#v", got.NetValue)
	}
	if got.CardProgram.InterestFreeInstallmentRewardEligible {
		t.Fatalf("interest-free installment reward eligibility was invented: %#v", got.CardProgram)
	}
}

func TestSnapshotEstimatesRecentMembershipValueFromObservedWindowAndCurrentFee(t *testing.T) {
	now := time.Date(2026, 9, 2, 1, 0, 0, 0, time.UTC)
	service := NewWithCosts(syntheticSource{snapshot: core.AccountBenefitsSnapshot{
		Membership:   core.WowMembership{FirstJoinDate: "2025-08-01", IsPaidMember: true, CurrentMonthlyFeeKRW: 7_890},
		BenefitUsage: core.WowBenefitUsage{TotalObservedSavingsKRW: 210_000, WindowStatus: "observed", WindowKind: "rolling_recent_months", WindowMonths: 3},
		Coverage:     core.AccountBenefitsCoverage{BenefitUsageObserved: true},
	}}, syntheticCosts{evidence: core.MembershipCostEvidence{
		Status: "complete_available_history", Provenance: "derived", CompleteHistorySync: true,
	}})
	service.now = func() time.Time { return now }

	got, err := service.Snapshot(context.Background(), core.AccountBenefitsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if got.NetValue.Status != "estimated_current_fee_window" || got.NetValue.Provenance != "inferred" || got.NetValue.EstimatedMembershipFeeKRW != 23_670 || got.NetValue.EstimatedNetValueKRW != 186_330 {
		t.Fatalf("unexpected recent membership estimate: %#v", got.NetValue)
	}
	if got.NetValue.ConfirmedNetValueKRW != 0 || got.NetValue.ConfirmedCardAnnualFeeKRW != 0 || len(got.NetValue.MissingEvidence) != 1 || got.NetValue.MissingEvidence[0] != "actual_membership_payments_for_benefit_window" {
		t.Fatalf("estimated comparison was presented as confirmed or misaligned: %#v", got.NetValue)
	}
	if got.MembershipCosts.Status != "unavailable_no_explicit_membership_order_metadata" {
		t.Fatalf("zero observed orders were presented as zero historical cost: %#v", got.MembershipCosts)
	}
}
