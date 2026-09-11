package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/account"
	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	coupangaccount "github.com/JungHoonGhae/coupang-ctl/internal/coupang/account"
)

type accountEvidenceSource string

func (s accountEvidenceSource) FetchAccountBenefits(context.Context, core.AccountBenefitsRequest) ([]byte, error) {
	return []byte(s), nil
}

func TestAccountStructureErrorIsTypedAndRedacted(t *testing.T) {
	var output bytes.Buffer
	err := errors.Join(coupangaccount.ErrAccountBenefitsDataMissing, errors.New("synthetic-private-source-value"))
	WriteError(&output, err)
	var wire struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(output.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Error.Code != "structured_account_benefits_data_missing" || bytes.Contains(output.Bytes(), []byte("synthetic-private-source-value")) {
		t.Fatal("account error lost classification or leaked source data")
	}
}

func TestAccountDefaultRouteRequiresCamofoxButHelpDoesNot(t *testing.T) {
	t.Setenv("COUPANGCTL_STATE_DIR", t.TempDir())
	var stdout, stderr bytes.Buffer
	if err := Run(context.Background(), []string{"account", "benefits", "--help"}, &stdout, &stderr, "test"); err != nil || !bytes.Contains(stdout.Bytes(), []byte("headless Camofox")) {
		t.Fatal("account help required a runtime")
	}
	stdout.Reset()
	if err := Run(context.Background(), []string{"account", "benefits"}, &stdout, &stderr, "test"); !errors.Is(err, browser.ErrCamofoxUnavailable) || errors.Is(err, browser.ErrCamofoxUnsupported) || stdout.Len() != 0 {
		t.Fatal("default account route did not select Camofox")
	}
}

func TestAccountCommandUsesParserCoverageOnWire(t *testing.T) {
	for _, known := range []bool{false, true} {
		usage := `{"membershipDays":30}`
		member := `{"membershipStatus":"ACTIVE"}`
		payment := `{"paymentMethodDTO":{"payMethodName":"Synthetic card"}}`
		if known {
			usage = `{"totalAmount":0,"freeReturnAmount":0,"freeReturnCount":0}`
			member = `{"membershipStatus":"INACTIVE","notMember":true,"paidMember":false}`
			payment = `{"paymentMethodDTO":{"payMethodName":"Synthetic card"},"recurringPayRegistered":false}`
		}
		doc := accountEvidenceSource(`{"membership":{"data":{"loyaltyMemberInfo":` + member + `,"paymentMethods":[` + payment + `],"wowBenefitUsage":` + usage + `}}}`)
		service := account.New(coupangaccount.New(doc))
		var output bytes.Buffer
		if err := runAccount(context.Background(), []string{"benefits", "--cash-pages", "1"}, &output, service); err != nil {
			t.Fatal(err)
		}
		var wire struct {
			Version    int                          `json:"schema_version"`
			Benefits   map[string]json.RawMessage   `json:"benefit_usage"`
			Membership map[string]json.RawMessage   `json:"membership"`
			Methods    []map[string]json.RawMessage `json:"payment_methods"`
			Coverage   core.AccountBenefitsCoverage `json:"coverage"`
		}
		if err := json.Unmarshal(output.Bytes(), &wire); err != nil {
			t.Fatal(err)
		}
		value, present := wire.Benefits["total_observed_savings_krw"]
		if wire.Version != core.AccountBenefitsSchemaVersion || present != known || wire.Coverage.BenefitUsageObserved != known || known && string(value) != "0" || wire.Coverage.CashTransactionStatus != "not_read" {
			t.Fatal("CLI changed observed-zero or missing evidence")
		}
		if len(wire.Methods) != 1 {
			t.Fatal("CLI lost registered-method evidence")
		}
		for _, tc := range []struct {
			fields        map[string]json.RawMessage
			key, expected string
		}{
			{wire.Benefits, "free_return_krw", "0"}, {wire.Benefits, "free_return_count", "0"},
			{wire.Membership, "is_member", "false"}, {wire.Membership, "is_paid_member", "false"},
			{wire.Methods[0], "recurring_registered", "false"},
		} {
			value, present := tc.fields[tc.key]
			if present != known || known && string(value) != tc.expected {
				t.Errorf("CLI changed %s availability or value", tc.key)
			}
		}
	}
}
