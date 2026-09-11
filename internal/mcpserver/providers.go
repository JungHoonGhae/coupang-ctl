package mcpserver

import (
	"context"
	"github.com/JungHoonGhae/coupang-ctl/internal/core"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var ErrBrowserSetupRequired = core.NewError("browser_setup_required")
var ErrLocalDataUnavailable = core.NewError("local_data_unavailable")

// ProviderFactory resolves a dependency for one validated tool invocation.
// Factories may be called concurrently; their owner manages shared resources
// until the server has stopped. Failed initialization is not cached here.
type ProviderFactory[P any] func(context.Context, string) (P, error)

// ProviderFactories advertises capabilities without opening their dependencies.
// A nil factory omits its tools, except auth_status, which returns an unavailable
// error. The report renderer is independent of all providers.
type ProviderFactories struct {
	Auth            ProviderFactory[StatusProvider]
	AuthRecovery    ProviderFactory[AuthRecoveryProvider]
	Orders          ProviderFactory[OrderProvider]
	OrderPreview    ProviderFactory[OrderPreviewProvider]
	Products        ProviderFactory[ProductProvider]
	Recommendations ProviderFactory[ProductRecommendationProvider]
	Account         ProviderFactory[AccountProvider]
	Receipts        ProviderFactory[ReceiptProvider]
}

func fixedProvider[P any](provider P) ProviderFactory[P] {
	return func(context.Context, string) (P, error) { return provider, nil }
}

func addProviderTool[P, In, Out any](server *mcp.Server, factory ProviderFactory[P], tool *mcp.Tool, handler func(context.Context, *mcp.CallToolRequest, In, P) (*mcp.CallToolResult, Out, error)) {
	addTool(server, tool, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		var zero Out
		if err := ctx.Err(); err != nil {
			return nil, zero, err
		}
		if factory == nil {
			return nil, zero, core.NewError("provider_unavailable")
		}
		provider, err := factory(ctx, tool.Name)
		if err != nil {
			return nil, zero, core.WithErrorCode("provider_unavailable", err)
		}
		return handler(ctx, request, input, provider)
	})
}
