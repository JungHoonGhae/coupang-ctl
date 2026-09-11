package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Run explicitly with -run '^$' -bench BenchmarkLocalOrderAggregateProcess
// -benchtime=23x -count=1. The first three samples include new-process startup;
// the next twenty use repeated CLI processes or one persistent MCP process.
// This is synthetic application-cold evidence, not an OS-cache flush or a
// measurement of the user's account. No credentials or real state are loaded.
func BenchmarkLocalOrderAggregateProcess(b *testing.B) {
	const orderCount = 5000
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	stateDir := b.TempDir()
	executable := filepath.Join(b.TempDir(), "coupangctl")
	build := exec.CommandContext(ctx, "go", "build", "-o", executable, "./cmd/coupangctl")
	build.Dir = "../.."
	if output, err := build.CombinedOutput(); err != nil {
		b.Fatalf("build: %v: %s", err, output)
	}
	ledger, err := store.Open(ctx, filepath.Join(stateDir, "coupangctl.sqlite3"))
	if err != nil {
		b.Fatal(err)
	}
	page := core.OrderPage{}
	start := time.Date(2021, 1, 1, 3, 0, 0, 0, time.UTC)
	for i := 0; i < orderCount; i++ {
		at := start.Add(time.Duration(i) * 8 * time.Hour)
		delivered := at.Add(30 * time.Hour)
		order := core.Order{SourceRef: fmt.Sprintf("synthetic-%d", i), PurchasedAt: at.Add(9 * time.Hour).Format(time.DateOnly), PurchasedAtTime: &at, Currency: "KRW", TotalAmount: 2400}
		for j := 0; j < 2; j++ {
			order.Items = append(order.Items, core.OrderItem{ProductID: fmt.Sprint(1000 + (i*2+j)%200), Name: "Synthetic item", BrandName: "Synthetic brand", Quantity: 1, PaidPrice: 1200, DeliveredAt: &delivered, CommerceKind: core.CommerceKindProductPurchase})
		}
		page.Orders = append(page.Orders, order)
	}
	if _, err := ledger.UpsertOrderPage(ctx, page); err != nil {
		ledger.Close()
		b.Fatal(err)
	}
	if err := ledger.Close(); err != nil {
		b.Fatal(err)
	}
	// Deliberately do not inherit COUPANGCTL settings, credentials, browser
	// discovery selectors, or real account paths from the parent environment.
	command := func(args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, executable, args...)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "TMPDIR=" + os.Getenv("TMPDIR"), "SystemRoot=" + os.Getenv("SystemRoot"), "COUPANGCTL_STATE_DIR=" + stateDir, "COUPANGCTL_BROWSER_PATH=" + filepath.Join(stateDir, "no-browser")}
		return cmd
	}
	for _, operation := range []struct{ cli, tool string }{
		{"stats", "orders_stats"}, {"spend", "orders_spend"}, {"insights", "orders_insights"}, {"products", "orders_product_insights"},
	} {
		baseline, err := command("orders", operation.cli).Output()
		if err != nil {
			b.Fatalf("synthetic CLI reference read: %v", err)
		}
		cliEvidence := comparableLatencyAggregate(b, baseline)
		for _, mode := range []string{"cli", "mcp"} {
			b.Run(operation.cli+"/"+mode, func(b *testing.B) {
				var session *mcp.ClientSession
				defer func() {
					if session != nil {
						session.Close()
					}
				}()
				timings := make([]time.Duration, 0, b.N)
				b.ResetTimer()
				for i := 0; i < b.N; i++ {
					before := time.Now()
					var output []byte
					if mode == "cli" {
						output, err = command("orders", operation.cli).Output()
					} else {
						if session == nil {
							client := mcp.NewClient(&mcp.Implementation{Name: "synthetic-latency", Version: "test"}, nil)
							session, err = client.Connect(ctx, &mcp.CommandTransport{Command: command("mcp")}, nil)
						}
						if err == nil {
							var result *mcp.CallToolResult
							result, err = session.CallTool(ctx, &mcp.CallToolParams{Name: operation.tool})
							if err == nil && result.IsError {
								b.Fatal("synthetic MCP aggregate returned tool error")
							}
							if err == nil {
								output, err = json.Marshal(result.StructuredContent)
							}
						}
					}
					timings = append(timings, time.Since(before))
					if err != nil {
						b.Fatalf("synthetic local aggregate: %v", err)
					}
					fields := comparableLatencyAggregate(b, output)
					countKey, wantCount := "order_count", float64(orderCount)
					if operation.cli == "products" {
						countKey, wantCount = "retained_item_line_count", float64(orderCount*2)
					}
					if fields[countKey] != wantCount {
						b.Fatal("synthetic row count mismatch")
					}
					// Only snapshot timestamps differ between these immutable reads.
					got, _ := json.Marshal(fields)
					want, _ := json.Marshal(cliEvidence)
					if string(got) != string(want) {
						b.Fatal("CLI/MCP aggregate payloads disagree")
					}
					if mode == "mcp" && i < 2 {
						session.Close()
						session = nil
					}
				}
				b.StopTimer()
				if len(timings) >= 23 {
					coldMax := max(timings[0], timings[1], timings[2])
					warm := append([]time.Duration(nil), timings[3:]...)
					sort.Slice(warm, func(i, j int) bool { return warm[i] < warm[j] })
					p95 := warm[(95*len(warm)+99)/100-1]
					b.ReportMetric(float64(coldMax)/float64(time.Millisecond), "cold3_max_ms")
					b.ReportMetric(float64(p95)/float64(time.Millisecond), "repeat_p95_ms")
					if coldMax > time.Second || p95 > time.Second {
						b.Errorf("synthetic latency exceeds 1s gate: cold max %v, repeated p95 %v", coldMax, p95)
					}
				}
				b.ReportMetric(orderCount, "synthetic_orders")
				if _, err := os.Stat(filepath.Join(stateDir, "browser-profile")); !os.IsNotExist(err) {
					b.Fatal("local aggregate created a browser profile")
				}
			})
		}
	}
}

func comparableLatencyAggregate(b *testing.B, output []byte) map[string]any {
	b.Helper()
	var fields map[string]any
	if err := json.Unmarshal(output, &fields); err != nil {
		b.Fatal(err)
	}
	evidence, ok := fields["evidence"].(map[string]any)
	if !ok || evidence["visibility"] != "private_local" || evidence["dataset"] != "retained_local_history" {
		b.Fatal("synthetic aggregate lost local evidence boundary")
	}
	captured, ok := evidence["snapshot_captured_at"].(string)
	if !ok {
		b.Fatal("synthetic aggregate lost read timestamp")
	}
	if _, err := time.Parse(time.RFC3339Nano, captured); err != nil {
		b.Fatal("synthetic aggregate read timestamp invalid")
	}
	delete(evidence, "snapshot_captured_at")
	return fields
}
