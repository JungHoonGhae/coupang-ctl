package mcpserver

import (
	"context"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type OrderPreviewProvider interface {
	Preview(context.Context) (core.OrderPreview, error)
}

func addOrderPreviewTool(server *mcp.Server, factory ProviderFactory[OrderPreviewProvider]) {
	addProviderTool(server, factory, &mcp.Tool{
		Name:        "orders_preview",
		Description: "Read one current normalized order entry page in the dedicated headless session without opening a ledger, using its cursor, persisting orders or combining retained account history. Use when the user asks to check current purchases before deciding whether to synchronize. Private-local fields retain exact dates and product evidence; share only what the user requested. Empty results and has_next_page describe this one source response, not whole-account history. Stable account identity remains unverified. No automatic login window, retry, checkout or payment. Browser-owned session persistence is separate from order storage.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: boolPointer(false), IdempotentHint: true, OpenWorldHint: boolPointer(true)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}, provider OrderPreviewProvider) (*mcp.CallToolResult, core.OrderPreview, error) {
		result, err := provider.Preview(ctx)
		if err != nil {
			return nil, core.OrderPreview{}, err
		}
		return nil, result, nil
	})
}
