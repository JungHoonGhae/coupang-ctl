package account_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/account"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	coupangaccount "github.com/JungHoonGhae/coupang-ctl/internal/coupang/account"
)

type evidenceDocument string

func (d evidenceDocument) FetchAccountBenefits(context.Context, core.AccountBenefitsRequest) ([]byte, error) {
	return []byte(d), nil
}

func TestSnapshotRequiresObservedBenefitTotal(t *testing.T) {
	for _, tc := range []struct {
		name, usage string
		observed    bool
	}{
		{"absent", `{}`, false},
		{"days_only", `{"membershipDays":100}`, false},
		{"null", `{"totalAmount":null,"membershipDays":100}`, false},
		{"observed_zero", `{"totalAmount":0}`, true},
		{"positive", `{"totalAmount":100}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			document := evidenceDocument(fmt.Sprintf(`{"membership":{"benefit_window_months":3,"data":{"loyaltyMemberInfo":{"membershipStatus":"ACTIVE","currentFee":7890},"wowBenefitUsage":%s}}}`, tc.usage))
			got, err := account.New(coupangaccount.New(document)).Snapshot(context.Background(), core.AccountBenefitsRequest{})
			if err != nil {
				t.Fatal(err)
			}
			if got.Coverage.BenefitUsageObserved != tc.observed {
				t.Errorf("benefit coverage = %v, want %v", got.Coverage.BenefitUsageObserved, tc.observed)
			}
			if estimated := got.NetValue.Status == "estimated_current_fee_window"; estimated != tc.observed {
				t.Errorf("estimate = %v, want %v", estimated, tc.observed)
			}
		})
	}
}

func TestSnapshotPreservesObservedZeroFee(t *testing.T) {
	document := evidenceDocument(`{"membership":{"benefit_window_months":3,"data":{"loyaltyMemberInfo":{"membershipStatus":"ACTIVE","currentFee":0,"paymentProperty":{"unitAmount":7890}},"wowBenefitUsage":{"totalAmount":100}}}}`)
	got, err := account.New(coupangaccount.New(document)).Snapshot(context.Background(), core.AccountBenefitsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Membership.CurrentMonthlyFeeKRW != 0 || got.NetValue.Status == "estimated_current_fee_window" {
		t.Fatal("observed zero fee replaced by fallback fee")
	}
}
