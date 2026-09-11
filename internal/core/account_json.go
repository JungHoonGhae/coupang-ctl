package core

import "encoding/json"

// Coverage gates the money fields used in headline comparisons. A source's
// explicit zero is retained; absent evidence never becomes a zero-valued fact.
func accountValue[T any](observed bool, value T) *T {
	if !observed {
		return nil
	}
	return &value
}

func (s AccountBenefitsSnapshot) MarshalJSON() ([]byte, error) {
	type plain AccountBenefitsSnapshot
	type membershipJSON struct {
		WowMembership
		Fee *int64 `json:"current_monthly_fee_krw,omitempty"`
	}
	type benefitJSON struct {
		WowBenefitUsage
		Total *int64 `json:"total_observed_savings_krw,omitempty"`
	}
	type rewardsJSON struct {
		WowCardRewardSummary
		Expected  *int64 `json:"expected_accumulation_krw,omitempty"`
		ThisMonth *int64 `json:"expected_this_month_krw,omitempty"`
		NextMonth *int64 `json:"expected_next_month_krw,omitempty"`
		Count     *int   `json:"observed_transaction_count,omitempty"`
		Amount    *int64 `json:"observed_accumulation_krw,omitempty"`
		Average   *int64 `json:"average_monthly_accumulation_krw,omitempty"`
	}
	type netJSON struct {
		MembershipNetValue
		Benefit      *int64 `json:"observed_benefit_krw,omitempty"`
		ConfirmedFee *int64 `json:"confirmed_membership_fee_krw,omitempty"`
		EstimatedFee *int64 `json:"estimated_membership_fee_krw,omitempty"`
		EstimatedNet *int64 `json:"estimated_net_value_krw,omitempty"`
	}
	c := s.Coverage
	transactionsRead := c.CashTransactionPagesRead > 0
	estimated := s.NetValue.Status == "estimated_current_fee_window" && c.BenefitUsageObserved && c.CurrentMembershipFeeObserved
	return json.Marshal(struct {
		plain
		Membership membershipJSON `json:"membership"`
		Benefits   benefitJSON    `json:"benefit_usage"`
		Rewards    rewardsJSON    `json:"wow_card_rewards"`
		Net        netJSON        `json:"net_value"`
	}{
		plain:      plain(s),
		Membership: membershipJSON{s.Membership, accountValue(c.CurrentMembershipFeeObserved, s.Membership.CurrentMonthlyFeeKRW)},
		Benefits:   benefitJSON{s.BenefitUsage, accountValue(c.BenefitUsageObserved, s.BenefitUsage.TotalObservedSavingsKRW)},
		Rewards: rewardsJSON{
			WowCardRewardSummary: s.CardRewards,
			Expected:             accountValue(c.CardRewardSummaryObserved, s.CardRewards.ExpectedAccumulationKRW),
			ThisMonth:            accountValue(c.CardRewardThisMonthObserved, s.CardRewards.ExpectedThisMonthKRW),
			NextMonth:            accountValue(c.CardRewardNextMonthObserved, s.CardRewards.ExpectedNextMonthKRW),
			Count:                accountValue(transactionsRead, s.CardRewards.ObservedTransactionCount),
			Amount:               accountValue(transactionsRead, s.CardRewards.ObservedAccumulationKRW),
			Average:              accountValue(transactionsRead && len(s.CardRewards.Monthly) > 0, s.CardRewards.AverageMonthlyAccumulationKRW),
		},
		Net: netJSON{
			MembershipNetValue: s.NetValue,
			Benefit:            accountValue(c.BenefitUsageObserved, s.NetValue.ObservedBenefitKRW),
			ConfirmedFee:       accountValue(s.MembershipCosts.ObservedPaymentCount > 0, s.NetValue.ConfirmedMembershipFeeKRW),
			EstimatedFee:       accountValue(estimated, s.NetValue.EstimatedMembershipFeeKRW),
			EstimatedNet:       accountValue(estimated, s.NetValue.EstimatedNetValueKRW),
		},
	})
}

func (c AccountBenefitsCoverage) MarshalJSON() ([]byte, error) {
	type plain AccountBenefitsCoverage
	if c.CashTransactionStatus == "" && c.CashTransactionPagesRead == 0 {
		c.CashTransactionStatus = "not_read"
	}
	return json.Marshal(struct {
		plain
		HasMore *bool `json:"cash_transaction_has_more,omitempty"`
	}{plain(c), accountValue(c.CashTransactionPagesRead > 0, c.CashTransactionHasMore)})
}
