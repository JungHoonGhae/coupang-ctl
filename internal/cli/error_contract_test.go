package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Only the external executable is synthetic. Both paths still use the real
// application, document parser and service; MCP also crosses the stdio SDK.
func TestSourceFailureCLIAndMCPShareActionableJSON(t *testing.T) {
	for _, tc := range []struct{ status, reason, code string }{
		{"access_denied", "access_denied", "camofox_access_denied"},
		{"authentication_required", "authentication_required", "camofox_authentication_required"},
	} {
		t.Run(tc.status, func(t *testing.T) {
			state := t.TempDir()
			syntheticCamofoxSearchRuntime(t, state, `{"result":{"status":"`+tc.status+`"}}`)
			t.Setenv("COUPANGCTL_STATE_DIR", state)
			args := []string{"products", "search", "--query", "synthetic", "--no-affiliate"}
			var stdout, stderr, failure bytes.Buffer
			err := Run(context.Background(), args, &stdout, &stderr, "test")
			if err == nil || stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatal("failed CLI read did not return a clean error")
			}
			WriteCommandError(&failure, args, err)
			ctx, session := startCamofoxMCPTest(t, state)
			result := callCamofoxMCPTest(t, ctx, session, "products_search", map[string]any{"query": "synthetic", "disable_affiliate": true})
			if !result.IsError || result.StructuredContent != nil || len(result.Content) != 1 {
				t.Fatal("MCP source failure became protocol failure or zero-value success")
			}
			content, ok := result.Content[0].(*mcp.TextContent)
			if !ok || !json.Valid([]byte(content.Text)) {
				t.Fatal("MCP discarded actionable error JSON")
			}
			var cliJSON, mcpJSON bytes.Buffer
			if json.Compact(&cliJSON, failure.Bytes()) != nil || json.Compact(&mcpJSON, []byte(content.Text)) != nil || !bytes.Equal(cliJSON.Bytes(), mcpJSON.Bytes()) {
				t.Fatal("CLI and MCP disagree on the same source failure")
			}
			var body struct {
				Error map[string]any `json:"error"`
			}
			if json.Unmarshal(mcpJSON.Bytes(), &body) != nil || body.Error["code"] != tc.code || body.Error["reason"] != tc.reason || body.Error["operation"] != "products_search" || body.Error["retryable"] != false || body.Error["next_action"] == nil || body.Error["next_action"] == "" {
				t.Fatal("missing failure classification or recovery action")
			}
			if bytes.Contains(mcpJSON.Bytes(), []byte(state)) {
				t.Fatal("failure exposed a profile path")
			}
			if _, err := os.Stat(filepath.Join(state, "camofox-coupangctl.sqlite3")); !os.IsNotExist(err) {
				t.Fatal("failed source read acquired purchase storage")
			}
		})
	}
}
