package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/auth"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type authenticationEvidenceBrowser struct {
	failure error
	logins  int
}

func (*authenticationEvidenceBrowser) Inspect(context.Context) (auth.BrowserStatus, error) {
	return auth.BrowserStatus{Name: "Synthetic", ProfilePresent: true}, nil
}
func (b *authenticationEvidenceBrowser) Verify(context.Context) error { return b.failure }
func (b *authenticationEvidenceBrowser) Login(context.Context, core.LoginRequest) error {
	b.logins++
	return nil
}

func TestMCPPreservesAuthenticationScopeAndUnknownRecovery(t *testing.T) {
	ctx := context.Background()
	b := &authenticationEvidenceBrowser{}
	service := auth.NewService(b)
	server := NewWithProviders(Providers{Auth: service, AuthRecovery: service}, "test")
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "synthetic-auth", Version: "test"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "auth_status", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatal("verified source rejected")
	}
	data, _ := json.Marshal(result.StructuredContent)
	var status core.AuthStatus
	if json.Unmarshal(data, &status) != nil || status.State != core.AuthVerified || status.VerificationScope != core.AuthVerificationSession {
		t.Fatal("MCP lost authentication scope")
	}
	b.failure = errors.Join(core.ErrAuthenticationStatusUnavailable, errors.New("synthetic-private-source-value"))
	for _, name := range []string{"auth_status", "auth_login_if_needed"} {
		args := map[string]any{}
		if name == "auth_login_if_needed" {
			args["confirmed"] = true
		}
		result, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || !result.IsError || b.logins != 0 {
			t.Fatal("unknown authentication started login or claimed success")
		}
		data, _ = json.Marshal(result)
		if !strings.Contains(string(data), "authentication_status_unavailable") || strings.Contains(string(data), "synthetic-private") {
			t.Fatal("MCP lost typed error or exposed private details")
		}
	}
}
