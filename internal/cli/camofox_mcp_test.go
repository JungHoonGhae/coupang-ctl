package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/browser"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/mcpserver"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Exercise the real CLI bootstrap and stdio transport, not just a server with
// injected providers. Every child uses a synthetic, isolated state directory.
func TestCamofoxMCPProcess(t *testing.T) {
	if os.Getenv("COUPANGCTL_TEST_MCP_PROCESS") != "1" {
		return
	}
	if err := Run(context.Background(), []string{"mcp"}, os.Stdout, os.Stderr, "test"); err != nil {
		WriteCommandError(os.Stderr, []string{"mcp"}, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func startCamofoxMCPTest(t *testing.T, state string) (context.Context, *mcp.ClientSession) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCamofoxMCPProcess$")
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "COUPANGCTL_STATE_DIR=") && !strings.HasPrefix(env, "COUPANGCTL_TEST_MCP_PROCESS=") {
			cmd.Env = append(cmd.Env, env)
		}
	}
	cmd.Env = append(cmd.Env, "COUPANGCTL_STATE_DIR="+state, "COUPANGCTL_TEST_MCP_PROCESS=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "synthetic-client", Version: "test"}, nil)
	// A race-instrumented helper deliberately pauses before process exit.
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd, TerminateDuration: 5 * time.Second}, nil)
	if err != nil {
		t.Fatalf("MCP startup failed before any tool call: %v", err)
	}
	t.Cleanup(func() {
		if err := session.Close(); err != nil {
			t.Errorf("MCP shutdown: %v", err)
		}
		if stderr.Len() != 0 {
			t.Error("MCP emitted unexpected diagnostics")
		}
	})
	return ctx, session
}

func callCamofoxMCPTest(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args any) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s protocol failure: %v", name, err)
	}
	return result
}

func TestCamofoxMCPDiscoveryAndReportNeedNoSetup(t *testing.T) {
	for _, state := range []string{filepath.Join(t.TempDir(), "absent"), "invalid-relative-state"} {
		t.Run(filepath.Base(state), func(t *testing.T) {
			ctx, session := startCamofoxMCPTest(t, state)
			list, err := session.ListTools(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			names := make(map[string]bool)
			for _, tool := range list.Tools {
				names[tool.Name] = true
			}
			for _, name := range []string{"products_report_render", "products_search", "products_recommend", "orders_stats", "orders_preview", "auth_status", "auth_login_if_needed"} {
				if !names[name] {
					t.Fatalf("missing tool %s before setup", name)
				}
			}
			if names["receipts_status"] {
				t.Fatal("unsupported source capability was advertised")
			}
			report := core.ProductRecommendationReportRenderRequest{Report: core.ProductRecommendationReport{Title: "Synthetic offline report", Recommendation: core.ProductRecommendationResult{SchemaVersion: core.ProductRecommendationSchemaVersion, Status: core.ProductRecommendationIncomplete, Warnings: []string{"Synthetic source unavailable"}}}}
			result := callCamofoxMCPTest(t, ctx, session, "products_report_render", report)
			data, _ := json.Marshal(result.StructuredContent)
			if result.IsError || !bytes.Contains(data, []byte("Synthetic offline report")) {
				t.Fatal("offline report blocked by runtime setup")
			}
			if _, err := os.Stat(state); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("discovery or rendering created runtime state")
			}
		})
	}
}

func TestCamofoxMCPLocalReadsSurviveMissingBrowserSetup(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	ctx, session := startCamofoxMCPTest(t, state)
	for _, tc := range []struct {
		name string
		args any
	}{
		{"auth_status", struct{}{}},
		{"products_search", core.ProductSearchRequest{Query: "synthetic"}},
		{"products_recommend", core.ProductRecommendationRequest{Query: "synthetic"}},
		{"product_inspect", core.ProductInspectRequest{ProductID: "101"}},
		{"product_watch_refresh", core.ProductWatchRefreshRequest{}},
		{"orders_sync", core.SyncRequest{MaxPages: 1}},
		{"orders_preview", struct{}{}},
		{"orders_enrich_categories", core.CategoryEnrichmentRequest{}},
		{"account_benefits", core.AccountBenefitsRequest{}},
	} {
		blocked := callCamofoxMCPTest(t, ctx, session, tc.name, tc.args)
		data, _ := json.Marshal(blocked.Content)
		if !blocked.IsError || !bytes.Contains(data, []byte("browser_setup_required")) || bytes.Contains(data, []byte(state)) {
			t.Fatalf("%s did not return a bounded actionable setup error", tc.name)
		}
	}
	if _, err := os.Stat(state); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed source initialization created runtime state")
	}
	for _, name := range []string{"orders_sync_status", "orders_stats", "product_watchlist"} {
		result := callCamofoxMCPTest(t, ctx, session, name, struct{}{})
		if result.IsError {
			t.Fatalf("local tool %s requires a configured browser", name)
		}
	}
	if _, err := os.Stat(filepath.Join(state, "camofox-coupangctl.sqlite3")); err != nil {
		t.Fatal("local tools did not use the dedicated ledger")
	}
	for _, name := range []string{"camofox", "browser-profile", "coupangctl.sqlite3", "camofox.json"} {
		if _, err := os.Stat(filepath.Join(state, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("local read created unrelated state %s", name)
		}
	}
}

func TestCamofoxMCPSeesSetupRepairWithoutReconnect(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	ctx, session := startCamofoxMCPTest(t, state)
	if !callCamofoxMCPTest(t, ctx, session, "auth_status", struct{}{}).IsError {
		t.Fatal("missing setup accepted")
	}
	// Inert installation-shaped files are enough for the local installation
	// check. No profile exists, so auth_status must not execute the fake node.
	installation := t.TempDir()
	cfg := browser.CamofoxConfig{SchemaVersion: 1, Node: filepath.Join(installation, "node"), Server: filepath.Join(installation, "server.js"), EngineDir: filepath.Join(installation, "engine"), UserID: "synthetic"}
	for _, path := range []string{cfg.Node, cfg.Server, filepath.Join(cfg.EngineDir, "version.json"), filepath.Join(installation, "plugins", "persistence", "index.js"), filepath.Join(installation, "mcp", "lib", "cookies.mjs")} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("synthetic-inert-fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := browser.SaveCamofoxConfig(state, cfg); err != nil {
		t.Fatal(err)
	}
	result := callCamofoxMCPTest(t, ctx, session, "auth_status", struct{}{})
	data, _ := json.Marshal(result.StructuredContent)
	var status core.AuthStatus
	if json.Unmarshal(data, &status) != nil || result.IsError || status.State != core.AuthNotConfigured || status.ProfilePresent {
		t.Fatal("same MCP session did not distinguish configured runtime from absent authentication")
	}
	for _, name := range []string{"camofox", "camofox-coupangctl.sqlite3"} {
		if _, err := os.Stat(filepath.Join(state, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("local auth inspection initialized a browser or ledger")
		}
	}
}

func TestCamofoxMCPLedgerInitializationCanRecover(t *testing.T) {
	state := t.TempDir()
	path := filepath.Join(state, "camofox-coupangctl.sqlite3")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, session := startCamofoxMCPTest(t, state)
	failed := callCamofoxMCPTest(t, ctx, session, "orders_stats", struct{}{})
	data, _ := json.Marshal(failed.Content)
	if !failed.IsError || !bytes.Contains(data, []byte("local_data_unavailable")) || bytes.Contains(data, []byte(state)) {
		t.Fatal("ledger failure not safely classified")
	}
	if err := session.Ping(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	} // Only our empty synthetic blocker.
	result := callCamofoxMCPTest(t, ctx, session, "orders_stats", struct{}{})
	if result.IsError {
		t.Fatal("failed initialization was cached")
	}
}

func TestCamofoxMCPLedgerConcurrentInitializationAndClose(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	t.Setenv("COUPANGCTL_STATE_DIR", state)
	r := &camofoxMCPRuntime{}
	t.Cleanup(r.close)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.localLedger(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled initialization ran")
	}
	if _, err := os.Stat(state); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancelled initialization wrote state")
	}
	var wg sync.WaitGroup
	results := make(chan *store.SQLite, 12)
	for i := 0; i < cap(results); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ledger, err := r.localLedger(context.Background())
			if err != nil {
				t.Error("concurrent initialization failed")
			}
			results <- ledger
		}()
	}
	wg.Wait()
	close(results)
	var shared *store.SQLite
	for ledger := range results {
		if shared == nil {
			shared = ledger
		}
		if ledger == nil || ledger != shared {
			t.Fatal("opened multiple ledgers")
		}
	}
	r.close()
	if _, err := r.localLedger(context.Background()); !errors.Is(err, mcpserver.ErrLocalDataUnavailable) {
		t.Fatal("closed runtime reopened its ledger")
	}
}
