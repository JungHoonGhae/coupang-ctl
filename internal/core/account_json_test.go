package core_test

import (
	"encoding/json"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

func TestAccountJSONDoesNotInventHeadlineMoneyOrPagination(t *testing.T) {
	for _, known := range []bool{false, true} {
		snapshot := core.AccountBenefitsSnapshot{
			Coverage: core.AccountBenefitsCoverage{
				CurrentMembershipFeeObserved: known, BenefitUsageObserved: known,
				CardRewardSummaryObserved: known, CardRewardThisMonthObserved: known,
				CardRewardNextMonthObserved: known,
			},
		}
		if known {
			snapshot.Coverage.CashTransactionPagesRead = 1
			snapshot.Coverage.CashTransactionStatus = "complete_returned_pages"
		}
		encoded, err := json.Marshal(snapshot)
		if err != nil {
			t.Fatal(err)
		}
		var wire map[string]map[string]json.RawMessage
		// Select object fields only; the envelope also contains scalar fields.
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &envelope); err != nil {
			t.Fatal(err)
		}
		wire = make(map[string]map[string]json.RawMessage)
		for _, key := range []string{"membership", "benefit_usage", "wow_card_rewards", "net_value", "coverage"} {
			var block map[string]json.RawMessage
			if err := json.Unmarshal(envelope[key], &block); err != nil {
				t.Fatal(err)
			}
			wire[key] = block
		}
		for block, keys := range map[string][]string{
			"membership":       {"current_monthly_fee_krw"},
			"benefit_usage":    {"total_observed_savings_krw"},
			"wow_card_rewards": {"expected_accumulation_krw", "expected_this_month_krw", "expected_next_month_krw", "observed_transaction_count", "observed_accumulation_krw"},
			"net_value":        {"observed_benefit_krw"},
			"coverage":         {"cash_transaction_has_more"},
		} {
			for _, key := range keys {
				value, exists := wire[block][key]
				if exists != known {
					t.Fatalf("%s.%s presence=%v, want %v", block, key, exists, known)
				}
				if known && string(value) != "0" && string(value) != "false" {
					t.Fatalf("known zero/false lost for %s", key)
				}
			}
		}
		for _, key := range []string{"confirmed_net_value_krw", "confirmed_card_annual_fee_krw", "estimated_membership_fee_krw", "estimated_net_value_krw"} {
			if _, exists := wire["net_value"][key]; exists {
				t.Fatalf("uncomputed %s emitted", key)
			}
		}
		if _, exists := wire["wow_card_rewards"]["average_monthly_accumulation_krw"]; exists {
			t.Fatal("average without observed months emitted")
		}
	}
}
