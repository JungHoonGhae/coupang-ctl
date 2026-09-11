package mcpserver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/account"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	coupangaccount "github.com/JungHoonGhae/coupang-ctl/internal/coupang/account"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type accountEvidenceSource string

func (s accountEvidenceSource) FetchAccountBenefits(context.Context, core.AccountBenefitsRequest) ([]byte, error) {
	return []byte(s), nil
}

func TestAccountMCPUsesParserCoverageOnWire(t *testing.T) {
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
		ctx := context.Background()
		server := NewWithProviders(Providers{Account: service}, "test")
		ct, st := mcp.NewInMemoryTransports()
		ss, err := server.Connect(ctx, st, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer ss.Close()
		client := mcp.NewClient(&mcp.Implementation{Name: "evidence-test", Version: "test"}, nil)
		cs, err := client.Connect(ctx, ct, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer cs.Close()
		response, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "account_benefits", Arguments: map[string]any{"max_cash_transaction_pages": 1}})
		if err != nil || response.IsError {
			t.Fatal("MCP rejected account evidence", err)
		}
		encoded, err := json.Marshal(response.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		var wire struct {
			Version    int                          `json:"schema_version"`
			Benefits   map[string]json.RawMessage   `json:"benefit_usage"`
			Membership map[string]json.RawMessage   `json:"membership"`
			Methods    []map[string]json.RawMessage `json:"payment_methods"`
			Coverage   core.AccountBenefitsCoverage `json:"coverage"`
		}
		if err := json.Unmarshal(encoded, &wire); err != nil {
			t.Fatal(err)
		}
		value, present := wire.Benefits["total_observed_savings_krw"]
		if wire.Version != core.AccountBenefitsSchemaVersion || present != known || wire.Coverage.BenefitUsageObserved != known || known && string(value) != "0" || wire.Coverage.CashTransactionStatus != "not_read" {
			t.Fatal("MCP changed observed-zero or missing evidence")
		}
		if len(wire.Methods) != 1 {
			t.Fatal("MCP lost registered-method evidence")
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
				t.Errorf("MCP changed %s availability or value", tc.key)
			}
		}
	}
}
