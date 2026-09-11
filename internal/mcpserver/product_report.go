package mcpserver

import (
	"context"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/JungHoonGhae/coupang-ctl/internal/recommendationreport"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func addProductReportTool(server *mcp.Server) {
	addTool(server, &mcp.Tool{
		Name:        "products_report_render",
		Description: "Render existing recommendation v5 evidence as a private-local report v2 HTML string. Preserves query/category scope, verified filters, inspected exclusions, missing values, detail-read stop reasons and next actions. Reassesses required conditions from bound inspection evidence, not supplied success flags. Optional purchase aggregates preserve product versus vendor-option scope, exclusions, month precision and separate latest-attempt/cumulative-scan evidence; overlapping aggregates are never summed and do not establish preference or complete account history. Keeps distinct vendor options and rejects duplicate exact references within either list. Does not research, read a ledger, open a browser, fetch images, execute next actions, or save a file. The HTML may reference external images when explicitly opened later; reports containing purchase context must not be publicly shared. Input is limited to 4 MiB and HTML output to 8 MiB; 200 total distinct inspected/displayed options, 1000 discovery rows and 32 comparison axes are rendering resource bounds, not optimal recommendation counts.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: boolPointer(false), IdempotentHint: true, OpenWorldHint: boolPointer(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input core.ProductRecommendationReportRenderRequest) (*mcp.CallToolResult, core.ProductRecommendationReportRenderResult, error) {
		result, err := recommendationreport.RenderResult(ctx, input.Report)
		return nil, result, err
	})
}
