package account_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	coupangaccount "github.com/JungHoonGhae/coupang-ctl/internal/coupang/account"
)

const membershipEvidence = `"membership":{"data":{"loyaltyMemberInfo":{"membershipStatus":"ACTIVE"}}}`

func TestCashSummaryNeedsObservedKRWAmount(t *testing.T) {
	for _, tc := range []struct {
		name, summary string
		observed      bool
	}{
		{"empty", `{}`, false},
		{"null", `null`, false},
		{"missing_amount", `{"content":{"expectedWowCardAccumulationAmount":{"currency":"KRW"}}}`, false},
		{"null_amount", `{"content":{"expectedWowCardAccumulationAmount":{"currency":"KRW","amount":null}}}`, false},
		{"missing_currency", `{"content":{"expectedWowCardAccumulationAmount":{"amount":100}}}`, false},
		{"other_currency", `{"content":{"expectedWowCardAccumulationAmount":{"currency":"USD","amount":100}}}`, false},
		{"conflicting_currency", `{"content":{"expectedWowCardAccumulationAmount":{"currencyCode":"KRW","currency":"USD","amount":100}}}`, false},
		{"zero", `{"content":{"expectedWowCardAccumulationAmount":{"currency":"KRW","amount":0}}}`, true},
		{"positive", `{"content":{"expectedWowCardAccumulationAmount":{"currencyCode":"KRW","amount":100}}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := coupangaccount.ParseSnapshotDocument([]byte(`{` + membershipEvidence + `,"cash_summary":` + tc.summary + `}`))
			if err != nil {
				t.Fatal(err)
			}
			if got.Coverage.CardRewardSummaryObserved != tc.observed {
				t.Errorf("summary observed = %v, want %v", got.Coverage.CardRewardSummaryObserved, tc.observed)
			}
			if !tc.observed && got.CardRewards.ExpectedAccumulationKRW != 0 {
				t.Fatal("unknown money included in KRW total")
			}
		})
	}
}

func TestCashCoverageSeparatesUnreadPartialAndExhausted(t *testing.T) {
	for _, tc := range []struct {
		pages, status string
		count         int
		more          bool
	}{
		{`[]`, "not_read", 0, false},
		{`[{"content":{"currentPageNumber":1,"nextPageExist":true,"list":[]}}]`, "partial", 1, true},
		{`[{"content":{"currentPageNumber":1,"nextPageExist":false,"list":[]}}]`, "complete_returned_pages", 1, false},
		{`[{"content":{"currentPageNumber":1,"nextPageExist":true,"list":[]}},{"content":{"currentPageNumber":2,"nextPageExist":false,"list":[]}}]`, "complete_returned_pages", 2, false},
	} {
		got, err := coupangaccount.ParseSnapshotDocument([]byte(`{` + membershipEvidence + `,"cash_transaction_pages":` + tc.pages + `}`))
		if err != nil {
			t.Fatal(err)
		}
		if got.Coverage.CashTransactionStatus != tc.status || got.Coverage.CashTransactionPagesRead != tc.count || got.Coverage.CashTransactionHasMore != tc.more {
			t.Fatal("cash coverage conflates unread, bounded and exhausted reads")
		}
	}
}

func TestCashTransactionsRequireExactMoneyAndDates(t *testing.T) {
	const transaction = `{"cashableAmount":{"currency":"KRW","amount":100},"nonCashableAmount":{"currency":"KRW","amount":0},"displayMessage":"Synthetic Wow card reward","description":"synthetic-private-description","createdAt":"2026-09-01T00:00:00Z"}`
	for _, tc := range []struct {
		name, rows string
		valid      bool
	}{
		{"valid", transaction, true},
		{"missing_currency", strings.Replace(transaction, `"currency":"KRW",`, "", 1), false},
		{"missing_component", strings.Replace(transaction, `"nonCashableAmount":{"currency":"KRW","amount":0},`, "", 1), false},
		{"null_component", strings.Replace(transaction, `"amount":100`, `"amount":null`, 1), false},
		{"other_currency", strings.Replace(transaction, `"KRW"`, `"USD"`, 1), false},
		{"invalid_date", strings.Replace(transaction, `2026-09-01T00:00:00Z`, `synthetic-not-a-date`, 1), false},
		{"component_overflow", strings.Replace(strings.Replace(transaction, `"amount":100`, `"amount":9223372036854775807`, 1), `"amount":0`, `"amount":1`, 1), false},
		{"aggregate_overflow", strings.Replace(transaction, `"amount":100`, `"amount":9223372036854775807`, 1) + `,` + transaction, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := coupangaccount.ParseSnapshotDocument([]byte(`{` + membershipEvidence + `,"cash_transaction_pages":[{"content":{"currentPageNumber":1,"nextPageExist":false,"list":[` + tc.rows + `]}}]}`))
			if (err == nil) != tc.valid {
				t.Fatal("cash amount/date validation mismatch")
			}
			if tc.valid {
				if got.CardRewards.ObservedAccumulationKRW != 100 || got.CardRewards.ObservedTransactionCount != 1 {
					t.Fatal("verified reward lost")
				}
				wire, err := json.Marshal(got)
				if err != nil || strings.Contains(string(wire), "synthetic-private-description") {
					t.Fatal("raw cash description escaped normalization")
				}
			}
		})
	}
}

func TestCashSummaryKeepsIndependentMonthEvidence(t *testing.T) {
	got, err := coupangaccount.ParseSnapshotDocument([]byte(`{` + membershipEvidence + `,"cash_summary":{"content":{"expectedWowCardAccumulationAmountThisMonth":{"amount":{"currency":"KRW","amount":0}}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got.Coverage.CardRewardSummaryObserved || !got.Coverage.CardRewardThisMonthObserved || got.Coverage.CardRewardNextMonthObserved {
		t.Fatal("one month became total or other-month evidence")
	}
	wire, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wire), `"expected_this_month_krw":0`) || strings.Contains(string(wire), `"expected_accumulation_krw"`) || strings.Contains(string(wire), `"expected_next_month_krw"`) {
		t.Fatal("independent monetary availability lost on wire")
	}
}

func TestCashPagesRejectUnverifiableSequence(t *testing.T) {
	page := func(number int, more bool) string {
		return fmt.Sprintf(`{"content":{"currentPageNumber":%d,"nextPageExist":%t,"list":[]}}`, number, more)
	}
	for _, tc := range []struct{ name, pages string }{
		{"empty_object", `{}`},
		{"null", `null`},
		{"missing_more", `{"content":{"currentPageNumber":1,"list":[]}}`},
		{"null_more", `{"content":{"currentPageNumber":1,"nextPageExist":null,"list":[]}}`},
		{"missing_list", `{"content":{"currentPageNumber":1,"nextPageExist":false}}`},
		{"non_initial", page(2, false)},
		{"duplicate", page(1, true) + `,` + page(1, false)},
		{"gap", page(1, true) + `,` + page(3, false)},
		{"after_terminal", page(1, false) + `,` + page(2, false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := coupangaccount.ParseSnapshotDocument([]byte(`{` + membershipEvidence + `,"cash_transaction_pages":[` + tc.pages + `]}`)); err == nil {
				t.Fatal("unverified cash pagination accepted as complete evidence")
			}
		})
	}
}
