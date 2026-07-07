package mcp

import (
	"context"
	"fmt"

	mcptool "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
	mcpclient "github.com/mark3labs/mcp-go/client"
	mcpschema "github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// ToolsFromInProcessServer 把进程内 MCP Server 暴露的工具转换成 Eino 可调用工具。
func ToolsFromInProcessServer(ctx context.Context, svr *mcpserver.MCPServer) ([]tool.BaseTool, error) {
	// In-process MCP 避免额外进程和网络开销，但仍复用 MCP 的工具 schema 和协议适配。
	mcpClient, err := mcpclient.NewInProcessClient(svr)
	if err != nil {
		return nil, fmt.Errorf("new in-process mcp client: %w", err)
	}
	if err := mcpClient.Start(ctx); err != nil {
		return nil, fmt.Errorf("start in-process mcp client: %w", err)
	}

	// 先完成 MCP 初始化握手，确保后续工具发现使用一致的协议版本和客户端信息。
	initRequest := mcpschema.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcpschema.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcpschema.Implementation{
		Name:    "agnet-project-demo-backend",
		Version: "1.0.0",
	}
	if _, err := mcpClient.Initialize(ctx, initRequest); err != nil {
		return nil, fmt.Errorf("initialize in-process mcp client: %w", err)
	}

	// eino-ext 会读取 MCP server 暴露的工具定义并转换成 BaseTool。
	tools, err := mcptool.GetTools(ctx, &mcptool.Config{Cli: mcpClient})
	if err != nil {
		return nil, fmt.Errorf("get mcp tools: %w", err)
	}
	return tools, nil
}
