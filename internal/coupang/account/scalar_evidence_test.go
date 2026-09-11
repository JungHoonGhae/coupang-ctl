package account_test

import (
	"encoding/json"
	"strings"
	"testing"

	coupangaccount "github.com/JungHoonGhae/coupang-ctl/internal/coupang/account"
)

// Every case crosses the production parser and JSON response boundary.
// A present zero/false is evidence; the same field absent or null is not.
func TestAccountScalarPresence(t *testing.T) {
	for _, tc := range []struct{ source, output, value, want string }{
		{"loyaltyMemberInfo.notMember", "membership.is_member", "false", "true"},
		{"loyaltyMemberInfo.paidMember", "membership.is_paid_member", "false", "false"},
		{"loyaltyMemberInfo.trialMember", "membership.is_trial_member", "false", "false"},
		{"loyaltyMemberInfo.membershipOnHold", "membership.is_on_hold", "false", "false"},
		{"wowBenefitUsage.membershipDays", "membership.membership_days", "0", "0"},
		{"paymentMethod.recurringPayRegistered", "membership.billing_method.recurring_registered", "false", "false"},
		{"wowBenefitUsage.rocketFreeDeliveryAmount", "benefit_usage.rocket_free_delivery_krw", "0", "0"},
		{"wowBenefitUsage.dawnAndSamedayDeliveryAmount", "benefit_usage.dawn_and_same_day_delivery_krw", "0", "0"},
		{"wowBenefitUsage.freshDeliveryAmount", "benefit_usage.fresh_delivery_krw", "0", "0"},
		{"wowBenefitUsage.freeDeliveryTotalAmount", "benefit_usage.free_delivery_total_krw", "0", "0"},
		{"wowBenefitUsage.wowOnlyDiscountAmount", "benefit_usage.wow_only_discount_krw", "0", "0"},
		{"wowBenefitUsage.freeReturnAmount", "benefit_usage.free_return_krw", "0", "0"},
		{"wowBenefitUsage.rocketJikguFreeDeliveryAmount", "benefit_usage.rocket_jikgu_free_delivery_krw", "0", "0"},
		{"wowBenefitUsage.eatsDiscountAmount", "benefit_usage.eats_discount_krw", "0", "0"},
		{"wowBenefitUsage.coupangDiscountAmount", "benefit_usage.coupang_discount_krw", "0", "0"},
		{"wowBenefitUsage.additionalCashbackAmount", "benefit_usage.additional_cashback_krw", "0", "0"},
		{"wowBenefitUsage.retentionCashback", "benefit_usage.retention_cashback_krw", "0", "0"},
		{"wowBenefitUsage.retailFreeShippingCount", "benefit_usage.retail_free_shipping_count", "0", "0"},
		{"wowBenefitUsage.ordersDawnAndSamedayCount", "benefit_usage.dawn_and_same_day_order_count", "0", "0"},
		{"wowBenefitUsage.ordersRocketFreshCount", "benefit_usage.rocket_fresh_order_count", "0", "0"},
		{"wowBenefitUsage.freeReturnCount", "benefit_usage.free_return_count", "0", "0"},
		{"wowBenefitUsage.jikguFreeShippingCount", "benefit_usage.jikgu_free_shipping_count", "0", "0"},
		{"wowBenefitUsage.wowBenefitUsageImprovedDtoV2.numbersOrderEats", "benefit_usage.eats_order_count", "0", "0"},
	} {
		t.Run(tc.output, func(t *testing.T) {
			for _, state := range []string{"absent", "null", "known"} {
				t.Run(state, func(t *testing.T) {
					data := map[string]any{"loyaltyMemberInfo": map[string]any{"membershipStatus": "ACTIVE"}}
					if state != "absent" {
						value := json.RawMessage("null")
						if state == "known" {
							value = json.RawMessage(tc.value)
						}
						setAccountFixtureField(data, tc.source, value)
					}
					encoded, _ := json.Marshal(map[string]any{"membership": map[string]any{"data": data}})
					got, err := coupangaccount.ParseSnapshotDocument(encoded)
					if err != nil {
						t.Fatal(err)
					}
					encoded, err = json.Marshal(got)
					if err != nil {
						t.Fatal(err)
					}
					value := accountWireField(t, encoded, tc.output)
					if state == "known" {
						if string(value) != tc.want {
							t.Errorf("observed value = %s, want %s", value, tc.want)
						}
					} else if value != nil {
						t.Errorf("unobserved field became %s", value)
					}
				})
			}
			if tc.value == "0" {
				for _, invalid := range []string{"-1", "0.5", `"0"`, "true"} {
					data := map[string]any{"loyaltyMemberInfo": map[string]any{"membershipStatus": "ACTIVE"}}
					setAccountFixtureField(data, tc.source, json.RawMessage(invalid))
					encoded, _ := json.Marshal(map[string]any{"membership": map[string]any{"data": data}})
					if _, err := coupangaccount.ParseSnapshotDocument(encoded); err == nil {
						t.Errorf("invalid %s amount/count accepted", invalid)
					}
				}
			}
		})
	}
}

func setAccountFixtureField(data map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	for _, part := range parts[:len(parts)-1] {
		child, ok := data[part].(map[string]any)
		if !ok {
			child = map[string]any{}
			data[part] = child
		}
		data = child
	}
	data[parts[len(parts)-1]] = value
}

func accountWireField(t *testing.T, data []byte, path string) json.RawMessage {
	t.Helper()
	for _, part := range strings.Split(path, ".") {
		var object map[string]json.RawMessage
		if err := json.Unmarshal(data, &object); err != nil {
			t.Fatal(err)
		}
		var present bool
		data, present = object[part]
		if !present {
			return nil
		}
	}
	return data
}

func TestPaymentSummaryDoesNotChooseOneRecurringFlagFromCollapsedMethods(t *testing.T) {
	for _, tc := range []struct{ flags, want string }{
		{"true,true", "true"}, {"false,false", "false"}, {"true,false", ""},
		{"false,true", ""}, {"null,true", ""}, {"true,null", ""}, {"null,null", ""},
	} {
		t.Run(tc.flags, func(t *testing.T) {
			var methods []any
			for _, flag := range strings.Split(tc.flags, ",") {
				methods = append(methods, map[string]any{
					"paymentMethodDTO":       map[string]any{"payMethodType": "CARD", "payMethodName": "Synthetic same brand"},
					"recurringPayRegistered": json.RawMessage(flag),
				})
			}
			encoded, _ := json.Marshal(map[string]any{"membership": map[string]any{"data": map[string]any{
				"loyaltyMemberInfo": map[string]any{"membershipStatus": "ACTIVE"}, "paymentMethods": methods,
			}}})
			got, err := coupangaccount.ParseSnapshotDocument(encoded)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.PaymentMethods) != 1 {
				t.Fatal("same-brand private identifiers must not become public method identities")
			}
			encoded, err = json.Marshal(got.PaymentMethods[0])
			if err != nil {
				t.Fatal(err)
			}
			if value := string(accountWireField(t, encoded, "recurring_registered")); value != tc.want {
				t.Errorf("collapsed recurring flag = %q, want %q", value, tc.want)
			}
		})
	}
}
