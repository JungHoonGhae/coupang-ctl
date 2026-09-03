package account_test

import (
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	coupangaccount "github.com/JungHoonGhae/coupang-ctl/internal/coupang/account"
)

func TestParseSnapshotDocumentNormalizesMembershipBenefitsAndCardRewards(t *testing.T) {
	document := []byte(`{
  "membership":{"benefit_window_months":3,"props":{"pageProps":{"data":{
    "loyaltyMemberInfo":{
      "firstJoinDt":1754006400000,"membershipStatus":"ACTIVE","subscriptionPlan":"SYNTHETIC_MONTHLY",
      "membershipInfoVO":{"membershipStartDt":1788192000000,"membershipEndDt":1790784000000},
      "paymentProperty":{"unitAmount":7890,"nextPaymentDt":1790784000000},
      "membershipOnHold":false,"paidMember":true,"trialMember":false,"notMember":false,
      "currentFee":7890,"nextPaymentDate":1790784000000
    },
    "paymentMethod":{"paymentMethodDTO":{"payMethodType":"CARD","payMethodName":"Synthetic card","payMethodAccount":"0000-PII-DISCARDED","payMethodIssuer":"Synthetic issuer"},"recurringPayRegistered":true},
    "paymentMethods":[
      {"paymentMethodDTO":{"payMethodType":"CARD","payMethodName":"Synthetic card","payMethodAccount":"0000-PII-DISCARDED","payMethodIssuer":"Synthetic issuer"},"recurringPayRegistered":true},
      {"paymentMethodDTO":{"payMethodType":"BANK","payMethodName":"Synthetic bank","payMethodAccount":"0000-PII-DISCARDED"},"recurringPayRegistered":false}
    ],
    "wowBenefitUsage":{
      "membershipDays":398,"totalAmount":210000,"rocketFreeDeliveryAmount":30000,
      "dawnAndSamedayDeliveryAmount":40000,"freshDeliveryAmount":20000,"freeDeliveryTotalAmount":90000,
      "wowOnlyDiscountAmount":50000,"freeReturnAmount":10000,"rocketJikguFreeDeliveryAmount":5000,
      "eatsDiscountAmount":15000,"coupangDiscountAmount":25000,"additionalCashbackAmount":9000,
      "retentionCashback":6000,"retailFreeShippingCount":10,"ordersDawnAndSamedayCount":8,
      "ordersRocketFreshCount":4,"freeReturnCount":2,"jikguFreeShippingCount":1,
      "wowBenefitUsageImprovedDtoV2":{"numbersOrderEats":3}
    }
  }}}},
  "cash_summary":{"content":{
    "expectedWowCardAccumulationAmount":{"currency":"KRW","amount":12000},
    "expectedWowCardAccumulationAmountThisMonth":{"earningDate":"2026-09-15","amount":{"currency":"KRW","amount":7000}},
    "expectedWowCardAccumulationAmountNextMonth":{"earningDate":"2026-10-15","amount":{"currency":"KRW","amount":5000}}
  }},
  "cash_transaction_pages":[{"content":{"currentPageNumber":1,"nextPageExist":false,"list":[
    {"actionType":"ACCUMULATE","cashableAmount":{"currencyCode":"KRW","amount":4000},"nonCashableAmount":{"currencyCode":"KRW","amount":0},"displayMessage":"Synthetic Wow card reward","description":"와우카드 쿠팡캐시 적립","createdAt":"2026-08-15T01:00:00Z"},
    {"actionType":"ACCUMULATE","cashableAmount":{"currencyCode":"KRW","amount":3000},"nonCashableAmount":{"currencyCode":"KRW","amount":0},"displayMessage":"와우카드 적립","description":"Synthetic service reward","createdAt":"2026-09-15T01:00:00Z"},
    {"actionType":"USE","cashableAmount":{"currencyCode":"KRW","amount":-1000},"displayMessage":"Synthetic cash use","description":"Synthetic product","createdAt":"2026-09-16T01:00:00Z"}
  ]}}]
}`)

	got, err := coupangaccount.ParseSnapshotDocument(document)
	if err != nil {
		t.Fatal(err)
	}
	if got.Membership.Status != "ACTIVE" || !got.Membership.IsMember || !got.Membership.IsPaidMember || got.Membership.CurrentMonthlyFeeKRW != 7890 || got.Membership.NextPaymentDate != "2026-10-01" {
		t.Fatalf("unexpected membership: %#v", got.Membership)
	}
	if got.Membership.BillingMethod.Name != "Synthetic card" || got.Membership.BillingMethod.Type != "CARD" || !got.Membership.BillingMethod.RecurringRegistered || len(got.PaymentMethods) != 2 {
		t.Fatalf("unexpected payment methods: billing=%#v all=%#v", got.Membership.BillingMethod, got.PaymentMethods)
	}
	if got.BenefitUsage.TotalObservedSavingsKRW != 210000 || got.BenefitUsage.FreeDeliveryTotalKRW != 90000 || got.BenefitUsage.EatsOrderCount != 3 {
		t.Fatalf("unexpected benefit usage: %#v", got.BenefitUsage)
	}
	if got.BenefitUsage.WindowStatus != "observed" || got.BenefitUsage.WindowKind != "rolling_recent_months" || got.BenefitUsage.WindowMonths != 3 {
		t.Fatalf("unexpected benefit window: %#v", got.BenefitUsage)
	}
	if !got.CardRewards.UsageObserved || got.CardRewards.ExpectedAccumulationKRW != 12000 || got.CardRewards.ExpectedThisMonthKRW != 7000 || got.CardRewards.ObservedTransactionCount != 2 || got.CardRewards.ObservedAccumulationKRW != 7000 || len(got.CardRewards.Monthly) != 2 {
		t.Fatalf("unexpected card rewards: %#v", got.CardRewards)
	}
	if got.Coverage.CashTransactionPagesRead != 1 || got.Coverage.CashTransactionHasMore || got.Coverage.MembershipPaymentsObserved || got.NetValue.Status != "not_computed_unaligned_windows" {
		t.Fatalf("unexpected evidence coverage: coverage=%#v net=%#v", got.Coverage, got.NetValue)
	}
	if got.OrderPayments.Status != "unavailable" || got.OrderPayments.Source != "credit_card_sales_slip" || got.OrderPayments.ObservedPaymentCount != 0 || len(got.OrderPayments.Limitations) != 2 {
		t.Fatalf("unobserved order payments were invented: %#v", got.OrderPayments)
	}
}

func TestParseSnapshotDocumentRejectsMissingMembershipState(t *testing.T) {
	if _, err := coupangaccount.ParseSnapshotDocument([]byte(`{"membership":{}}`)); err == nil {
		t.Fatal("expected missing membership state to be rejected")
	}
}

func TestClassifyCommerceKindUsesExplicitMembershipMetadata(t *testing.T) {
	item := core.OrderItem{ProductType: "MEMBERSHIP"}
	if got := core.ClassifyCommerceKind(item); got != core.CommerceKindMembershipFee {
		t.Fatalf("commerce kind = %q", got)
	}
}
