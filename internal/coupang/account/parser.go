package account

import (
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
)

var ErrAccountBenefitsDataMissing = core.WithErrorCode("structured_account_benefits_data_missing", errors.New("structured account benefits data missing"))

type snapshotDocument struct {
	Membership           json.RawMessage   `json:"membership"`
	CashSummary          json.RawMessage   `json:"cash_summary"`
	CashTransactionPages []json.RawMessage `json:"cash_transaction_pages"`
}

type membershipDocument struct {
	Data                membershipData `json:"data"`
	BenefitWindowMonths int            `json:"benefit_window_months"`
	Props               struct {
		PageProps struct {
			Data membershipData `json:"data"`
		} `json:"pageProps"`
	} `json:"props"`
	Query struct {
		Data membershipData `json:"data"`
	} `json:"query"`
}

type membershipData struct {
	LoyaltyMemberInfo    loyaltyMemberInfo `json:"loyaltyMemberInfo"`
	LoyaltyFeeChangeDate int64             `json:"loyaltyFeeChangeDate"`
	PaymentMethod        paymentMethod     `json:"paymentMethod"`
	PaymentMethods       []paymentMethod   `json:"paymentMethods"`
	WowBenefitUsage      benefitUsage      `json:"wowBenefitUsage"`
}

type loyaltyMemberInfo struct {
	FirstJoinDt      int64  `json:"firstJoinDt"`
	MembershipStatus string `json:"membershipStatus"`
	SubscriptionPlan string `json:"subscriptionPlan"`
	MembershipInfoVO struct {
		MembershipStartDt int64 `json:"membershipStartDt"`
		MembershipEndDt   int64 `json:"membershipEndDt"`
	} `json:"membershipInfoVO"`
	PaymentProperty struct {
		UnitAmount    *int64 `json:"unitAmount"`
		NextPaymentDt int64  `json:"nextPaymentDt"`
	} `json:"paymentProperty"`
	MembershipOnHold *bool  `json:"membershipOnHold"`
	PaidMember       *bool  `json:"paidMember"`
	TrialMember      *bool  `json:"trialMember"`
	NotMember        *bool  `json:"notMember"`
	CurrentFee       *int64 `json:"currentFee"`
	NextPaymentDate  int64  `json:"nextPaymentDate"`
}

type paymentMethod struct {
	PaymentMethodDTO struct {
		PayMethodType    string `json:"payMethodType"`
		PayMethodName    string `json:"payMethodName"`
		PayMethodAccount string `json:"payMethodAccount"`
		PayMethodIssuer  string `json:"payMethodIssuer"`
	} `json:"paymentMethodDTO"`
	RecurringPayRegistered *bool `json:"recurringPayRegistered"`
}

type benefitUsage struct {
	MembershipDays                *int   `json:"membershipDays"`
	TotalAmount                   *int64 `json:"totalAmount"`
	RocketFreeDeliveryAmount      *int64 `json:"rocketFreeDeliveryAmount"`
	DawnAndSamedayDeliveryAmount  *int64 `json:"dawnAndSamedayDeliveryAmount"`
	FreshDeliveryAmount           *int64 `json:"freshDeliveryAmount"`
	FreeDeliveryTotalAmount       *int64 `json:"freeDeliveryTotalAmount"`
	WowOnlyDiscountAmount         *int64 `json:"wowOnlyDiscountAmount"`
	FreeReturnAmount              *int64 `json:"freeReturnAmount"`
	RocketJikguFreeDeliveryAmount *int64 `json:"rocketJikguFreeDeliveryAmount"`
	EatsDiscountAmount            *int64 `json:"eatsDiscountAmount"`
	CoupangDiscountAmount         *int64 `json:"coupangDiscountAmount"`
	AdditionalCashbackAmount      *int64 `json:"additionalCashbackAmount"`
	RetentionCashback             *int64 `json:"retentionCashback"`
	RetailFreeShippingCount       *int   `json:"retailFreeShippingCount"`
	OrdersDawnAndSamedayCount     *int   `json:"ordersDawnAndSamedayCount"`
	OrdersRocketFreshCount        *int   `json:"ordersRocketFreshCount"`
	FreeReturnCount               *int   `json:"freeReturnCount"`
	JikguFreeShippingCount        *int   `json:"jikguFreeShippingCount"`
	Improved                      struct {
		NumbersOrderEats *int `json:"numbersOrderEats"`
	} `json:"wowBenefitUsageImprovedDtoV2"`
}

type money struct {
	CurrencyCode string `json:"currencyCode"`
	Currency     string `json:"currency"`
	Amount       *int64 `json:"amount"`
}

type expectedReward struct {
	EarningDate string `json:"earningDate"`
	Amount      money  `json:"amount"`
}

type cashSummaryDocument struct {
	Content struct {
		ExpectedWowCardAccumulationAmount          money           `json:"expectedWowCardAccumulationAmount"`
		ExpectedWowCardAccumulationAmountThisMonth *expectedReward `json:"expectedWowCardAccumulationAmountThisMonth"`
		ExpectedWowCardAccumulationAmountNextMonth *expectedReward `json:"expectedWowCardAccumulationAmountNextMonth"`
	} `json:"content"`
}

type cashTransactionPage struct {
	Content struct {
		CurrentPageNumber *int              `json:"currentPageNumber"`
		NextPageExist     *bool             `json:"nextPageExist"`
		List              []cashTransaction `json:"list"`
	} `json:"content"`
}

type cashTransaction struct {
	SourceWowCardLabel *bool  `json:"source_wow_card_label"`
	ActionType         string `json:"actionType"`
	CashableAmount     money  `json:"cashableAmount"`
	NonCashableAmount  money  `json:"nonCashableAmount"`
	DisplayMessage     string `json:"displayMessage"`
	Description        string `json:"description"`
	CreatedAt          string `json:"createdAt"`
}

func ParseSnapshotDocument(document []byte) (core.AccountBenefitsSnapshot, error) {
	var raw snapshotDocument
	if len(document) == 0 || len(document) > 4<<20 || !json.Valid(document) || json.Unmarshal(document, &raw) != nil || len(raw.Membership) == 0 {
		return core.AccountBenefitsSnapshot{}, ErrAccountBenefitsDataMissing
	}
	var membership membershipDocument
	if json.Unmarshal(raw.Membership, &membership) != nil {
		return core.AccountBenefitsSnapshot{}, ErrAccountBenefitsDataMissing
	}
	data := membership.Data
	if data.LoyaltyMemberInfo.MembershipStatus == "" {
		data = membership.Props.PageProps.Data
	}
	if data.LoyaltyMemberInfo.MembershipStatus == "" {
		data = membership.Query.Data
	}
	if data.LoyaltyMemberInfo.MembershipStatus == "" {
		return core.AccountBenefitsSnapshot{}, ErrAccountBenefitsDataMissing
	}
	result := core.AccountBenefitsSnapshot{
		SchemaVersion: core.AccountBenefitsSchemaVersion,
		Visibility:    "private_local",
		Warnings:      []string{},
		Definitions: core.AccountBenefitDefinitions{
			BenefitSource:      "coupang_reported_wow_benefit_usage",
			CardRewardEvidence: "structured_expected_reward_plus_cash_transactions_classified_by_source_label",
			MembershipFee:      "current_monthly_fee_and_source_fee_change_date_are_observed_schedule_metadata_not_historical_charges",
			PaymentPrivacy:     "payment_account_identifiers_are_discarded",
			NetValue:           "recent-benefit comparison using current monthly fee times the source-observed month window; actual historical charges, card costs, and card rewards stay separate",
		},
	}
	info := data.LoyaltyMemberInfo
	fee := info.CurrentFee
	if fee == nil {
		fee = info.PaymentProperty.UnitAmount
	}
	if !nonNegative(fee) || !data.WowBenefitUsage.valid() {
		return core.AccountBenefitsSnapshot{}, ErrAccountBenefitsDataMissing
	}
	nextPayment := info.NextPaymentDate
	if nextPayment == 0 {
		nextPayment = info.PaymentProperty.NextPaymentDt
	}
	result.Membership = core.WowMembership{
		Status:       info.MembershipStatus,
		IsPaidMember: info.PaidMember, IsTrialMember: info.TrialMember, IsOnHold: info.MembershipOnHold,
		SubscriptionPlan:    info.SubscriptionPlan,
		SourceFeeChangeDate: dateFromMillis(data.LoyaltyFeeChangeDate),
		FirstJoinDate:       dateFromMillis(info.FirstJoinDt), CurrentPeriodStart: dateFromMillis(info.MembershipInfoVO.MembershipStartDt),
		CurrentPeriodEnd: dateFromMillis(info.MembershipInfoVO.MembershipEndDt), NextPaymentDate: dateFromMillis(nextPayment),
		MembershipDays: data.WowBenefitUsage.MembershipDays, BillingMethod: normalizePaymentMethod(data.PaymentMethod),
	}
	if info.NotMember != nil {
		isMember := !*info.NotMember
		result.Membership.IsMember = &isMember
	}
	if fee != nil {
		result.Membership.CurrentMonthlyFeeKRW = *fee
		result.Coverage.CurrentMembershipFeeObserved = true
	}
	result.PaymentMethods = normalizePaymentMethods(data.PaymentMethods)
	result.OrderPayments = core.OrderPaymentStatistics{
		Status: "unavailable", Source: "credit_card_sales_slip",
		ByMethod: []core.PaymentMethodUsage{},
		Limitations: []string{
			"registered payment methods do not prove which method funded each product order",
			"the adopted order and account-benefit sources do not expose installment months",
		},
	}
	benefit := data.WowBenefitUsage
	result.BenefitUsage = core.WowBenefitUsage{
		Source: "coupang_wow_management", WindowStatus: "unavailable",
		RocketFreeDeliveryKRW: benefit.RocketFreeDeliveryAmount, DawnAndSameDayDeliveryKRW: benefit.DawnAndSamedayDeliveryAmount,
		FreshDeliveryKRW: benefit.FreshDeliveryAmount, FreeDeliveryTotalKRW: benefit.FreeDeliveryTotalAmount,
		WowOnlyDiscountKRW: benefit.WowOnlyDiscountAmount, FreeReturnKRW: benefit.FreeReturnAmount,
		RocketJikguFreeDeliveryKRW: benefit.RocketJikguFreeDeliveryAmount, EatsDiscountKRW: benefit.EatsDiscountAmount,
		CoupangDiscountKRW: benefit.CoupangDiscountAmount, AdditionalCashbackKRW: benefit.AdditionalCashbackAmount,
		RetentionCashbackKRW: benefit.RetentionCashback, RetailFreeShippingCount: benefit.RetailFreeShippingCount,
		DawnAndSameDayOrderCount: benefit.OrdersDawnAndSamedayCount, RocketFreshOrderCount: benefit.OrdersRocketFreshCount,
		FreeReturnCount: benefit.FreeReturnCount, JikguFreeShippingCount: benefit.JikguFreeShippingCount,
		EatsOrderCount: benefit.Improved.NumbersOrderEats,
	}
	if membership.BenefitWindowMonths == 3 {
		result.BenefitUsage.WindowStatus = "observed"
		result.BenefitUsage.WindowKind = "rolling_recent_months"
		result.BenefitUsage.WindowMonths = 3
	}
	result.Coverage.MembershipStateObserved = true
	if benefit.TotalAmount != nil {
		result.BenefitUsage.TotalObservedSavingsKRW = *benefit.TotalAmount
		result.Coverage.BenefitUsageObserved = true
	}
	result.Coverage.PaymentMethodsObserved = len(result.PaymentMethods) > 0 || result.Membership.BillingMethod.Type != ""
	parseCashSummary(raw.CashSummary, &result)
	if err := parseCashTransactions(raw.CashTransactionPages, &result); err != nil {
		return core.AccountBenefitsSnapshot{}, err
	}
	result.NetValue = core.MembershipNetValue{
		ObservedBenefitKRW: result.BenefitUsage.TotalObservedSavingsKRW,
		Status:             "not_computed_unaligned_windows",
		Provenance:         "not_computed",
		WindowBasis:        "benefit and actual membership-payment windows are not both available",
		MissingEvidence:    []string{"benefit_window", "actual_membership_payments_for_benefit_window"},
		Limitations:        []string{"card rewards and card fees are excluded from membership-only value"},
	}
	result.Warnings = append(result.Warnings,
		"the account source does not expose historical membership payments; only separate explicit membership orders from the private local ledger can supply that cost evidence",
		"order-level payment method and lump-sum versus installment statistics remain unavailable until card sales-slip evidence is adopted",
	)
	return result, nil
}

func nonNegative[T ~int | ~int64](values ...*T) bool {
	for _, value := range values {
		if value != nil && *value < 0 {
			return false
		}
	}
	return true
}

func (v benefitUsage) valid() bool {
	return nonNegative(v.TotalAmount, v.RocketFreeDeliveryAmount, v.DawnAndSamedayDeliveryAmount,
		v.FreshDeliveryAmount, v.FreeDeliveryTotalAmount, v.WowOnlyDiscountAmount, v.FreeReturnAmount,
		v.RocketJikguFreeDeliveryAmount, v.EatsDiscountAmount, v.CoupangDiscountAmount,
		v.AdditionalCashbackAmount, v.RetentionCashback) &&
		nonNegative(v.MembershipDays, v.RetailFreeShippingCount, v.OrdersDawnAndSamedayCount,
			v.OrdersRocketFreshCount, v.FreeReturnCount, v.JikguFreeShippingCount, v.Improved.NumbersOrderEats)
}

func parseCashSummary(document json.RawMessage, result *core.AccountBenefitsSnapshot) {
	if len(document) == 0 {
		return
	}
	var raw cashSummaryDocument
	if json.Unmarshal(document, &raw) != nil {
		return
	}
	if amount, ok := krw(raw.Content.ExpectedWowCardAccumulationAmount); ok && amount >= 0 {
		result.CardRewards.ExpectedAccumulationKRW = amount
		result.Coverage.CardRewardSummaryObserved = true
	}
	if raw.Content.ExpectedWowCardAccumulationAmountThisMonth != nil {
		if amount, ok := krw(raw.Content.ExpectedWowCardAccumulationAmountThisMonth.Amount); ok && amount >= 0 {
			result.CardRewards.ExpectedThisMonthKRW = amount
			result.Coverage.CardRewardThisMonthObserved = true
			if _, err := time.Parse(time.DateOnly, raw.Content.ExpectedWowCardAccumulationAmountThisMonth.EarningDate); err == nil {
				result.CardRewards.ExpectedThisMonthEarningDate = raw.Content.ExpectedWowCardAccumulationAmountThisMonth.EarningDate
			}
		}
	}
	if raw.Content.ExpectedWowCardAccumulationAmountNextMonth != nil {
		if amount, ok := krw(raw.Content.ExpectedWowCardAccumulationAmountNextMonth.Amount); ok && amount >= 0 {
			result.CardRewards.ExpectedNextMonthKRW = amount
			result.Coverage.CardRewardNextMonthObserved = true
		}
	}
}

func parseCashTransactions(documents []json.RawMessage, result *core.AccountBenefitsSnapshot) error {
	result.Coverage.CashTransactionStatus = "not_read"
	if len(documents) > 100 {
		return ErrAccountBenefitsDataMissing
	}
	monthly := map[string]*core.MonthlyCardReward{}
	for i, document := range documents {
		var page cashTransactionPage
		if json.Unmarshal(document, &page) != nil || page.Content.CurrentPageNumber == nil || *page.Content.CurrentPageNumber != i+1 || page.Content.NextPageExist == nil || page.Content.List == nil || len(page.Content.List) > 500 || i > 0 && !result.Coverage.CashTransactionHasMore {
			return ErrAccountBenefitsDataMissing
		}
		result.Coverage.CashTransactionPagesRead++
		result.Coverage.CashTransactionHasMore = *page.Content.NextPageExist
		result.Coverage.CashTransactionStatus = "complete_returned_pages"
		if *page.Content.NextPageExist {
			result.Coverage.CashTransactionStatus = "partial"
		}
		for _, transaction := range page.Content.List {
			if !wowCardTransaction(transaction) {
				continue
			}
			cashable, cashableOK := krw(transaction.CashableAmount)
			nonCashable, nonCashableOK := krw(transaction.NonCashableAmount)
			if !cashableOK || !nonCashableOK || cashable > 0 && nonCashable > math.MaxInt64-cashable || cashable < 0 && nonCashable < math.MinInt64-cashable {
				return ErrAccountBenefitsDataMissing
			}
			amount := cashable + nonCashable
			if amount <= 0 {
				continue
			}
			date, month := transactionDate(transaction.CreatedAt)
			if month == "" || result.CardRewards.ObservedAccumulationKRW > math.MaxInt64-amount {
				return ErrAccountBenefitsDataMissing
			}
			entry := monthly[month]
			if entry == nil {
				entry = &core.MonthlyCardReward{Month: month}
				monthly[month] = entry
			}
			entry.TransactionCount++
			entry.AccumulationKRW += amount
			result.CardRewards.ObservedTransactionCount++
			result.CardRewards.ObservedAccumulationKRW += amount
			if result.CardRewards.FirstObservedRewardDate == "" || date < result.CardRewards.FirstObservedRewardDate {
				result.CardRewards.FirstObservedRewardDate = date
			}
			if date > result.CardRewards.LastObservedRewardDate {
				result.CardRewards.LastObservedRewardDate = date
			}
		}
	}
	months := make([]string, 0, len(monthly))
	for month := range monthly {
		months = append(months, month)
	}
	sort.Strings(months)
	for _, month := range months {
		result.CardRewards.Monthly = append(result.CardRewards.Monthly, *monthly[month])
	}
	if result.CardRewards.Monthly == nil {
		result.CardRewards.Monthly = []core.MonthlyCardReward{}
	}
	if len(months) > 0 {
		total, count := result.CardRewards.ObservedAccumulationKRW, int64(len(months))
		result.CardRewards.AverageMonthlyAccumulationKRW = total / count
		if total%count >= (count+1)/2 {
			result.CardRewards.AverageMonthlyAccumulationKRW++
		}
	}
	result.CardRewards.UsageObserved = result.CardRewards.ObservedTransactionCount > 0 || result.CardRewards.ExpectedAccumulationKRW > 0
	return nil
}

func normalizePaymentMethods(values []paymentMethod) []core.PaymentMethodSummary {
	result := make([]core.PaymentMethodSummary, 0, len(values))
	seen := map[string]int{}
	for _, value := range values {
		normalized := normalizePaymentMethod(value)
		if normalized.Type == "" && normalized.Name == "" {
			continue
		}
		key := normalized.Type + "\x00" + normalized.Name + "\x00" + normalized.Issuer
		if index, exists := seen[key]; exists {
			// Brand/type/issuer are a summary, not a distinct card identity.
			// Preserve a flag only if every collapsed observation agrees.
			previous := result[index].RecurringRegistered
			if previous == nil || normalized.RecurringRegistered == nil || *previous != *normalized.RecurringRegistered {
				result[index].RecurringRegistered = nil
			}
			continue
		}
		seen[key] = len(result)
		result = append(result, normalized)
	}
	return result
}

func normalizePaymentMethod(value paymentMethod) core.PaymentMethodSummary {
	// payMethodAccount is deliberately discarded even for private-local output.
	return core.PaymentMethodSummary{
		Type: value.PaymentMethodDTO.PayMethodType, Name: value.PaymentMethodDTO.PayMethodName,
		Issuer: value.PaymentMethodDTO.PayMethodIssuer, RecurringRegistered: value.RecurringPayRegistered,
	}
}

func wowCardTransaction(value cashTransaction) bool {
	if value.SourceWowCardLabel != nil {
		return *value.SourceWowCardLabel
	}
	text := strings.ToLower(value.DisplayMessage + " " + value.Description)
	return strings.Contains(text, "와우카드") || strings.Contains(text, "wow card")
}

func transactionDate(value string) (string, string) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		parsed, err = time.Parse("2006-01-02 15:04:05", value)
	}
	if err != nil {
		return "", ""
	}
	return parsed.Format(time.DateOnly), parsed.Format("2006-01")
}

func dateFromMillis(value int64) string {
	if value <= 0 {
		return ""
	}
	return time.UnixMilli(value).In(time.FixedZone("KST", 9*60*60)).Format(time.DateOnly)
}

func krw(value money) (int64, bool) {
	if value.Amount == nil || value.CurrencyCode != "" && value.Currency != "" && value.CurrencyCode != value.Currency {
		return 0, false
	}
	currency := value.CurrencyCode
	if currency == "" {
		currency = value.Currency
	}
	if currency != "KRW" {
		return 0, false
	}
	return *value.Amount, true
}
