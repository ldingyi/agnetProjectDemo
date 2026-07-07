package tools

import (
	"context"
	"fmt"

	"agnet-project-demo/backend/src/agent/tools/imsummary"
	thirdtools "agnet-project-demo/backend/src/agent/tools/third"
	"agnet-project-demo/backend/src/infrastructure"
	infrarpc "agnet-project-demo/backend/src/infrastructure/rpc"

	"github.com/cloudwego/eino/components/tool"
)

// NewTools 创建 ChatModelAgent 可调用的完整工具集合。
func NewTools(ctx context.Context, cfg infrastructure.Config) ([]tool.BaseTool, error) {
	thirdClient, err := infrarpc.NewThirdClient(cfg)
	if err != nil {
		return nil, err
	}

	tools, err := thirdtools.NewToolsWithClient(ctx, thirdClient)
	if err != nil {
		return nil, err
	}

	summaryTool, err := imsummary.NewTool(ctx, cfg, thirdClient)
	if err != nil {
		return nil, fmt.Errorf("new im summary tool: %w", err)
	}
	return append(tools, summaryTool), nil
}
