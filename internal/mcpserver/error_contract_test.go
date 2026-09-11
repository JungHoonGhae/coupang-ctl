package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type errorContractProducts struct {
	fixedProductProvider
	err   error
	calls *int
}

func (p errorContractProducts) Search(context.Context, core.ProductSearchRequest) (core.ProductSearchResult, error) {
	*p.calls++
	return core.ProductSearchResult{}, p.err
}

func toolErrorJSON(t *testing.T, result *mcp.CallToolResult) core.ErrorResponse {
	t.Helper()
	if !result.IsError || result.StructuredContent != nil || len(result.Content) != 1 {
		t.Fatal("failure was not an unstructured tool error")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	var response core.ErrorResponse
	if !ok || json.Unmarshal([]byte(text.Text), &response) != nil || response.Error.NextAction == "" || strings.Contains(text.Text, "synthetic-private") {
		t.Fatal("missing bounded actionable JSON error")
	}
	return response
}

func TestMCPErrorContractPreservesTypedServiceAndFactoryFailures(t *testing.T) {
	for _, atFactory := range []bool{false, true} {
		for _, err := range []error{core.ErrAuthenticationRequired, core.ErrBrowserAccessDenied, context.Canceled, context.DeadlineExceeded, core.NewError("profile_in_use"), core.NewError("camofox_operation_unsupported"), core.NewError("permission_required"), core.NewError("camofox_document_changed"), core.NewError("document_protocol_error")} {
			calls := 0
			ctx, cs := connectFactoryTest(t, ProviderFactories{Products: func(context.Context, string) (ProductProvider, error) {
				wrapped := errors.Join(core.NewError("product_source_unavailable"), err, errors.New("synthetic-private"))
				if atFactory {
					return nil, wrapped
				}
				return errorContractProducts{err: wrapped, calls: &calls}, nil
			}})
			result, callErr := cs.CallTool(ctx, &mcp.CallToolParams{Name: "products_search", Arguments: core.ProductSearchRequest{Query: "synthetic"}})
			if callErr != nil {
				t.Fatal("business failure escaped as a JSON-RPC error")
			}
			got := toolErrorJSON(t, result)
			if got != core.PublicError("products_search", err) {
				t.Fatalf("MCP changed public error policy: %s", got.Error.Code)
			}
			if (atFactory && calls != 0) || (!atFactory && calls != 1) {
				t.Fatal("failed read was retried")
			}
		}
	}
}

func TestMCPErrorContractValidatesBeforeAcquiringDependencies(t *testing.T) {
	calls := 0
	ctx, cs := connectFactoryTest(t, ProviderFactories{Products: func(context.Context, string) (ProductProvider, error) {
		calls++
		return errorContractProducts{calls: &calls}, nil
	}})
	for _, args := range []any{
		map[string]any{"query": []string{"synthetic-private"}},
		map[string]any{"query": "synthetic-private", "unknown": true},
		core.ProductSearchRequest{Query: "synthetic-private", CategoryID: "123"},
		core.ProductSearchRequest{Query: "synthetic-private", Limit: 21},
	} {
		result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "products_search", Arguments: args})
		if err != nil {
			t.Fatal("input failure escaped as a protocol error")
		}
		got := toolErrorJSON(t, result)
		if got.Error.Code != "invalid_request" || got.Error.Operation != "products_search" || calls != 0 {
			t.Fatal("invalid input acquired a dependency or lost classification")
		}
	}
	if _, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "synthetic_unknown_tool"}); err == nil || calls != 0 {
		t.Fatal("unknown-tool protocol error was swallowed")
	}
}

func TestMCPErrorContractDoesNotTrustProviderErrorText(t *testing.T) {
	calls := 0
	ctx, cs := connectFactoryTest(t, ProviderFactories{Products: fixedProvider[ProductProvider](errorContractProducts{err: errors.New("usage: synthetic-private"), calls: &calls})})
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "products_search", Arguments: core.ProductSearchRequest{Query: "synthetic"}})
	if err != nil {
		t.Fatal(err)
	}
	if toolErrorJSON(t, result).Error.Code != "internal_error" || calls != 1 {
		t.Fatal("untrusted text was promoted to validation failure")
	}
}
