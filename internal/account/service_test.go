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

func (s syntheticSource) Snapshot(context.Context, core.AccountBenefitsRequest) (core.AccountBenefitsSnapshot, error) {
	return s.snapshot, nil
}

func TestSnapshotAddsVersionedProgramTermsWithoutInventingPaidFees(t *testing.T) {
	service := New(syntheticSource{snapshot: core.AccountBenefitsSnapshot{
		BenefitUsage: core.WowBenefitUsage{TotalObservedSavingsKRW: 100_000},
		NetValue:     core.MembershipNetValue{Status: "partial_missing_cost_evidence", MissingEvidence: []string{"historical_membership_payments"}},
	}})
	service.now = func() time.Time { return time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC) }

	got, err := service.Snapshot(context.Background(), core.AccountBenefitsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != 1 || got.Visibility != "private_local" || got.CardProgram.PublishedAnnualFeeKRW != 20_000 || got.CardProgram.PromotionStatus != "active" {
		t.Fatalf("unexpected account envelope: %#v", got)
	}
	if got.NetValue.ConfirmedMembershipFeeKRW != 0 || got.NetValue.ConfirmedCardAnnualFeeKRW != 0 {
		t.Fatalf("unobserved costs were invented: %#v", got.NetValue)
	}
	if got.CardProgram.InterestFreeInstallmentRewardEligible {
		t.Fatalf("interest-free installment reward eligibility was invented: %#v", got.CardProgram)
	}
}
