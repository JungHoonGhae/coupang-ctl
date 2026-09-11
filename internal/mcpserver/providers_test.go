package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func connectFactoryTest(t *testing.T, factories ProviderFactories) (context.Context, *mcp.ClientSession) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	ct, st := mcp.NewInMemoryTransports()
	ss, err := NewWithFactories(factories, "test").Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "synthetic", Version: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return ctx, cs
}

func TestProviderFactoriesAreLazyValidatedAndRetryable(t *testing.T) {
	var calls atomic.Int32
	ctx, cs := connectFactoryTest(t, ProviderFactories{Products: func(_ context.Context, tool string) (ProductProvider, error) {
		if tool != "products_search" {
			t.Error("wrong tool passed to factory")
		}
		if calls.Add(1) == 1 {
			return nil, errors.New("synthetic-private-dependency-details")
		}
		return fixedProductProvider{}, nil
	}})
	if _, err := cs.ListTools(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if err := cs.Ping(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("discovery initialized dependencies")
	}
	invalid, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "products_search", Arguments: map[string]any{"query": []string{"wrong-type"}}})
	if err != nil || !invalid.IsError || calls.Load() != 0 {
		t.Fatal("invalid schema initialized dependencies")
	}
	if _, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "synthetic_unknown_tool"}); err == nil || calls.Load() != 0 {
		t.Fatal("unknown tool initialized dependencies")
	}
	args := &mcp.CallToolParams{Name: "products_search", Arguments: core.ProductSearchRequest{Query: "synthetic"}}
	failed, err := cs.CallTool(ctx, args)
	if err != nil || !failed.IsError {
		t.Fatal("initialization error was not a tool error")
	}
	data, _ := json.Marshal(failed.Content)
	if !strings.Contains(string(data), "provider_unavailable") || strings.Contains(string(data), "synthetic-private") {
		t.Fatal("factory error leaked")
	}
	succeeded, err := cs.CallTool(ctx, args)
	if err != nil || succeeded.IsError || calls.Load() != 2 {
		t.Fatal("factory failure poisoned future calls")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// The SDK adds request metadata; concurrent calls own their params.
			result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "products_search", Arguments: core.ProductSearchRequest{Query: "synthetic"}})
			if err != nil || result.IsError {
				t.Error("concurrent resolution failed")
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 10 {
		t.Fatal("resolution did not remain per invocation")
	}
}

func TestMissingAuthProviderReturnsErrorInsteadOfPanicking(t *testing.T) {
	ctx, cs := connectFactoryTest(t, ProviderFactories{})
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "auth_status", Arguments: struct{}{}})
	if err != nil || !result.IsError {
		t.Fatal("missing provider not reported")
	}
}

func TestProviderInitializationErrorsAreBounded(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{
		{errors.Join(ErrBrowserSetupRequired, errors.New("synthetic-private")), "browser_setup_required"},
		{errors.Join(ErrLocalDataUnavailable, errors.New("synthetic-private")), "local_data_unavailable"},
		{context.Canceled, "operation_cancelled"},
		{context.DeadlineExceeded, "operation_timed_out"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			ctx, cs := connectFactoryTest(t, ProviderFactories{Auth: func(context.Context, string) (StatusProvider, error) { return nil, tc.err }})
			result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "auth_status", Arguments: struct{}{}})
			if err != nil || !result.IsError {
				t.Fatal("not a tool error")
			}
			data, _ := json.Marshal(result.Content)
			if !strings.Contains(string(data), tc.code) || strings.Contains(string(data), "synthetic-private") {
				t.Fatal("unbounded initialization error")
			}
		})
	}
}
