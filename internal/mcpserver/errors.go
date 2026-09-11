package mcpserver

import (
	"context"
	"errors"

	"github.com/JungHoonGhae/coupang-ctl/internal/core"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type toolExecutionKey struct{}
type toolExecution struct{ entered bool }

// SDK input-schema failures happen before the typed handler. Normalize only
// that path here; unknown tools and JSON-RPC protocol errors remain protocol
// errors. No schema diagnostic containing user input is returned as tool text.
func errorContractMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
		if method != "tools/call" {
			return next(ctx, method, request)
		}
		state := &toolExecution{}
		result, err := next(context.WithValue(ctx, toolExecutionKey{}, state), method, request)
		if err == nil && !state.entered {
			if failure, ok := result.(*mcp.CallToolResult); ok && failure.IsError {
				operation := "unknown"
				if params, ok := request.GetParams().(*mcp.CallToolParamsRaw); ok {
					operation = params.Name
				}
				response := core.PublicError(operation, core.NewError("invalid_request"))
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: response.ErrorJSON()}}}, nil
			}
		}
		return result, err
	}
}

// Preserve the SDK's typed success/output-schema validation. Returning an
// ordinary error makes the SDK emit IsError + JSON TextContent, not a fabricated
// zero success or StructuredContent that the SDK would overwrite.
func addTool[In, Out any](server *mcp.Server, tool *mcp.Tool, handler func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error)) {
	mcp.AddTool(server, tool, func(ctx context.Context, request *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		if state, ok := ctx.Value(toolExecutionKey{}).(*toolExecution); ok {
			state.entered = true
		}
		var zero Out
		failure := func(err error) (*mcp.CallToolResult, Out, error) {
			return nil, zero, errors.New(core.PublicError(tool.Name, err).ErrorJSON())
		}
		if err := ctx.Err(); err != nil {
			return failure(err)
		}
		if validator, ok := any(input).(interface{ Validate() error }); ok {
			if err := core.ValidateRequest(validator); err != nil {
				return failure(err)
			}
		}
		result, output, err := handler(ctx, request, input)
		if err != nil {
			return failure(err)
		}
		return result, output, nil
	})
}
