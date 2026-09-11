package core

import (
	"errors"
	"time"
)

const AccountBenefitsSchemaVersion = 6

type AccountBenefitsRequest struct {
	MaxCashTransactionPages int `json:"max_cash_transaction_pages,omitempty" jsonschema:"Maximum Coupang Cash transaction pages to inspect,from 1 through 100"`
}

func (r AccountBenefitsRequest) Validate() error {
	if r.MaxCashTransactionPages < 0 || r.MaxCashTransactionPages > 100 {
		return errors.New("max_cash_transaction_pages must be between 1 and 100")
	}
	return nil
}

type AccountBenefitsSnapshot struct {
	SchemaVersion   int                       `json:"schema_version"`
	Visibility      string                    `json:"visibility"`
	FetchedAt       time.Time                 `json:"fetched_at"`
	Membership      WowMembership             `json:"membership"`
	MembershipCosts MembershipCostEvidence    `json:"membership_costs"`
	BenefitUsage    WowBenefitUsage           `json:"benefit_usage"`
	CardRewards     WowCardRewardSummary      `json:"wow_card_rewards"`
	CardProgram     WowCardProgramTerms       `json:"wow_card_program"`
	PaymentMethods  []PaymentMethodSummary    `json:"payment_methods"`
	OrderPayments   OrderPaymentStatistics    `json:"order_payments"`
	NetValue        MembershipNetValue        `json:"net_value"`
	Coverage        AccountBenefitsCoverage   `json:"coverage"`
	Warnings        []string                  `json:"warnings"`
	Definitions     AccountBenefitDefinitions `json:"definitions"`
}

type WowCardProgramTerms struct {
	ProductName                           string  `json:"product_name"`
	PublishedAnnualFeeKRW                 int64   `json:"published_annual_fee_krw"`
	CoupangBaseRewardRate                 float64 `json:"coupang_base_reward_rate"`
	CoupangPromotionRewardRate            float64 `json:"coupang_promotion_reward_rate"`
	CoupangMaximumRewardRate              float64 `json:"coupang_maximum_reward_rate"`
	CoupangBaseMonthlyCapKRW              int64   `json:"coupang_base_monthly_cap_krw"`
	CoupangPromotionMonthlyCapKRW         int64   `json:"coupang_promotion_monthly_cap_krw"`
	PromotionFrom                         string  `json:"promotion_from"`
	PromotionTo                           string  `json:"promotion_to"`
	PromotionStatus                       string  `json:"promotion_status"`
	InterestFreeInstallmentRewardEligible bool    `json:"interest_free_installment_reward_eligible"`
	SourceURL                             string  `json:"source_url"`
	LastVerified                          string  `json:"last_verified"`
}

// Optional source scalars use nil for unobserved values, including booleans.
// Status alone does not establish the separate member/payment flags.
type WowMembership struct {
	Status               string               `json:"status"`
	IsMember             *bool                `json:"is_member,omitempty"`
	IsPaidMember         *bool                `json:"is_paid_member,omitempty"`
	IsTrialMember        *bool                `json:"is_trial_member,omitempty"`
	IsOnHold             *bool                `json:"is_on_hold,omitempty"`
	SubscriptionPlan     string               `json:"subscription_plan,omitempty"`
	CurrentMonthlyFeeKRW int64                `json:"current_monthly_fee_krw,omitempty"`
	SourceFeeChangeDate  string               `json:"source_fee_change_date,omitempty"`
	FirstJoinDate        string               `json:"first_join_date,omitempty"`
	CurrentPeriodStart   string               `json:"current_period_start,omitempty"`
	CurrentPeriodEnd     string               `json:"current_period_end,omitempty"`
	NextPaymentDate      string               `json:"next_payment_date,omitempty"`
	MembershipDays       *int                 `json:"membership_days,omitempty"`
	BillingMethod        PaymentMethodSummary `json:"billing_method"`
}

type PaymentMethodSummary struct {
	Type                string `json:"type,omitempty"`
	Name                string `json:"name,omitempty"`
	Issuer              string `json:"issuer,omitempty"`
	RecurringRegistered *bool  `json:"recurring_registered,omitempty"`
}

// OrderPaymentStatistics is deliberately separate from registered payment
// methods. A saved card does not prove it funded an order, and an order amount
// does not prove whether the charge was lump-sum or installment.
type OrderPaymentStatistics struct {
	Status               string               `json:"status"`
	Source               string               `json:"source"`
	ObservedPaymentCount int                  `json:"observed_payment_count"`
	ObservedAmountKRW    int64                `json:"observed_amount_krw"`
	LumpSum              PaymentPlanBucket    `json:"lump_sum"`
	Installment          PaymentPlanBucket    `json:"installment"`
	Unknown              PaymentPlanBucket    `json:"unknown"`
	ByMethod             []PaymentMethodUsage `json:"by_method"`
	Limitations          []string             `json:"limitations"`
}

type PaymentPlanBucket struct {
	PaymentCount int     `json:"payment_count"`
	AmountKRW    int64   `json:"amount_krw"`
	AmountShare  float64 `json:"amount_share"`
}

type PaymentMethodUsage struct {
	Method           string `json:"method"`
	Issuer           string `json:"issuer,omitempty"`
	PaymentCount     int    `json:"payment_count"`
	AmountKRW        int64  `json:"amount_krw"`
	InstallmentCount int    `json:"installment_count"`
}

// Individual components are independent observations, not a decomposition
// computed from TotalObservedSavingsKRW. Nil is not zero savings or zero uses.
type WowBenefitUsage struct {
	Source                     string `json:"source"`
	WindowStatus               string `json:"window_status"`
	WindowKind                 string `json:"window_kind,omitempty"`
	WindowMonths               int    `json:"window_months"`
	TotalObservedSavingsKRW    int64  `json:"total_observed_savings_krw,omitempty"`
	RocketFreeDeliveryKRW      *int64 `json:"rocket_free_delivery_krw,omitempty"`
	DawnAndSameDayDeliveryKRW  *int64 `json:"dawn_and_same_day_delivery_krw,omitempty"`
	FreshDeliveryKRW           *int64 `json:"fresh_delivery_krw,omitempty"`
	FreeDeliveryTotalKRW       *int64 `json:"free_delivery_total_krw,omitempty"`
	WowOnlyDiscountKRW         *int64 `json:"wow_only_discount_krw,omitempty"`
	FreeReturnKRW              *int64 `json:"free_return_krw,omitempty"`
	RocketJikguFreeDeliveryKRW *int64 `json:"rocket_jikgu_free_delivery_krw,omitempty"`
	EatsDiscountKRW            *int64 `json:"eats_discount_krw,omitempty"`
	CoupangDiscountKRW         *int64 `json:"coupang_discount_krw,omitempty"`
	AdditionalCashbackKRW      *int64 `json:"additional_cashback_krw,omitempty"`
	RetentionCashbackKRW       *int64 `json:"retention_cashback_krw,omitempty"`
	RetailFreeShippingCount    *int   `json:"retail_free_shipping_count,omitempty"`
	DawnAndSameDayOrderCount   *int   `json:"dawn_and_same_day_order_count,omitempty"`
	RocketFreshOrderCount      *int   `json:"rocket_fresh_order_count,omitempty"`
	FreeReturnCount            *int   `json:"free_return_count,omitempty"`
	JikguFreeShippingCount     *int   `json:"jikgu_free_shipping_count,omitempty"`
	EatsOrderCount             *int   `json:"eats_order_count,omitempty"`
}

type WowCardRewardSummary struct {
	UsageObserved                 bool                `json:"usage_observed"`
	ExpectedAccumulationKRW       int64               `json:"expected_accumulation_krw,omitempty"`
	ExpectedThisMonthKRW          int64               `json:"expected_this_month_krw,omitempty"`
	ExpectedThisMonthEarningDate  string              `json:"expected_this_month_earning_date,omitempty"`
	ExpectedNextMonthKRW          int64               `json:"expected_next_month_krw,omitempty"`
	ObservedTransactionCount      int                 `json:"observed_transaction_count,omitempty"`
	ObservedAccumulationKRW       int64               `json:"observed_accumulation_krw,omitempty"`
	FirstObservedRewardDate       string              `json:"first_observed_reward_date,omitempty"`
	LastObservedRewardDate        string              `json:"last_observed_reward_date,omitempty"`
	AverageMonthlyAccumulationKRW int64               `json:"average_monthly_accumulation_krw,omitempty"`
	Monthly                       []MonthlyCardReward `json:"monthly"`
}

type MonthlyCardReward struct {
	Month            string `json:"month"`
	TransactionCount int    `json:"transaction_count"`
	AccumulationKRW  int64  `json:"accumulation_krw"`
}

type MembershipNetValue struct {
	ObservedBenefitKRW        int64    `json:"observed_benefit_krw,omitempty"`
	ConfirmedMembershipFeeKRW int64    `json:"confirmed_membership_fee_krw,omitempty"`
	EstimatedMembershipFeeKRW int64    `json:"estimated_membership_fee_krw,omitempty"`
	ConfirmedCardAnnualFeeKRW int64    `json:"confirmed_card_annual_fee_krw,omitempty"`
	ConfirmedNetValueKRW      int64    `json:"confirmed_net_value_krw,omitempty"`
	EstimatedNetValueKRW      int64    `json:"estimated_net_value_krw,omitempty"`
	ComparisonFrom            string   `json:"comparison_from,omitempty"`
	ComparisonTo              string   `json:"comparison_to,omitempty"`
	Provenance                string   `json:"provenance"`
	WindowBasis               string   `json:"window_basis"`
	Status                    string   `json:"status"`
	MissingEvidence           []string `json:"missing_evidence"`
	Limitations               []string `json:"limitations"`
}

// MembershipCostEvidence contains only orders whose normalized source
// metadata explicitly classifies every item as a membership charge. Product
// names and amount-pattern guesses are deliberately excluded.
type MembershipCostEvidence struct {
	Status                          string   `json:"status"`
	Source                          string   `json:"source"`
	Provenance                      string   `json:"provenance"`
	ObservedPaymentCount            int      `json:"observed_payment_count"`
	ObservedGrossAmountKRW          int64    `json:"observed_gross_amount_krw"`
	ObservedNonCanceledPaymentCount int      `json:"observed_non_canceled_payment_count"`
	ObservedPaidAmountKRW           int64    `json:"observed_paid_amount_krw"`
	FirstObservedPaymentDate        string   `json:"first_observed_payment_date,omitempty"`
	LastObservedPaymentDate         string   `json:"last_observed_payment_date,omitempty"`
	FirstObservedOrderDate          string   `json:"first_observed_order_date,omitempty"`
	LastObservedOrderDate           string   `json:"last_observed_order_date,omitempty"`
	LastCompleteHistorySyncAt       string   `json:"last_complete_history_sync_at,omitempty"`
	CompleteHistorySync             bool     `json:"complete_history_sync"`
	Limitations                     []string `json:"limitations"`
}

type AccountBenefitsCoverage struct {
	CurrentMembershipFeeObserved bool   `json:"current_membership_fee_observed"`
	MembershipStateObserved      bool   `json:"membership_state_observed"`
	BenefitUsageObserved         bool   `json:"benefit_usage_observed"`
	CardRewardSummaryObserved    bool   `json:"card_reward_summary_observed"`
	CardRewardThisMonthObserved  bool   `json:"card_reward_this_month_observed"`
	CardRewardNextMonthObserved  bool   `json:"card_reward_next_month_observed"`
	CashTransactionStatus        string `json:"cash_transaction_status"`
	CashTransactionPagesRead     int    `json:"cash_transaction_pages_read"`
	CashTransactionHasMore       bool   `json:"cash_transaction_has_more,omitempty"`
	PaymentMethodsObserved       bool   `json:"payment_methods_observed"`
	MembershipPaymentsObserved   bool   `json:"membership_payments_observed"`
	OrderPaymentDetailsObserved  bool   `json:"order_payment_details_observed"`
}

type AccountBenefitDefinitions struct {
	BenefitSource      string `json:"benefit_source"`
	CardRewardEvidence string `json:"card_reward_evidence"`
	MembershipFee      string `json:"membership_fee"`
	PaymentPrivacy     string `json:"payment_privacy"`
	NetValue           string `json:"net_value"`
}
