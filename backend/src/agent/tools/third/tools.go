package third

import (
	"context"
	"encoding/json"

	infra "agnet-project-demo/backend/src/infrastructure"
	inframcp "agnet-project-demo/backend/src/infrastructure/mcp"
	infrarpc "agnet-project-demo/backend/src/infrastructure/rpc"
	thirdrpc "agnet-project-demo/third/kitex_gen/third"
	"agnet-project-demo/third/kitex_gen/third/thirdbusinessservice"

	"github.com/cloudwego/eino/components/tool"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

type mcpToolSet struct {
	client thirdbusinessservice.Client
}

// NewTools 创建第三方业务 MCP 工具集合，并转换为 Eino 工具。
func NewTools(ctx context.Context, cfg infra.Config) ([]tool.BaseTool, error) {
	// 业务工具先封装成 MCP server，再转换成 Eino tool，便于后续替换为远端 MCP。
	thirdClient, err := infrarpc.NewThirdClient(cfg)
	if err != nil {
		return nil, err
	}

	return NewToolsWithClient(ctx, thirdClient)
}

// NewToolsWithClient 把已存在的第三方业务 RPC client 暴露成 Eino 工具集合。
func NewToolsWithClient(ctx context.Context, thirdClient thirdbusinessservice.Client) ([]tool.BaseTool, error) {
	return inframcp.ToolsFromInProcessServer(ctx, newMCPServer(thirdClient))
}

// newMCPServer 注册所有第三方业务工具的 MCP schema 和处理函数。
func newMCPServer(cli thirdbusinessservice.Client) *mcpserver.MCPServer {
	tools := &mcpToolSet{client: cli}
	svr := mcpserver.NewMCPServer("third-business-mcp", mcp.LATEST_PROTOCOL_VERSION)

	// 工具 schema 明确约束参数，降低模型直接拼 RPC 请求时漏传或错传字段的概率。
	svr.AddTool(mcp.NewTool("third_list_im_conversations",
		mcp.WithDescription("List IM conversation IDs for a user."),
		mcp.WithString("user_id", mcp.Required(), mcp.Description("User ID.")),
	), tools.listIMConversations)

	svr.AddTool(mcp.NewTool("third_get_im_conversation_messages",
		mcp.WithDescription("Get messages from a user's IM conversation."),
		mcp.WithString("user_id", mcp.Required(), mcp.Description("User ID.")),
		mcp.WithString("conversation_id", mcp.Required(), mcp.Description("Conversation ID.")),
	), tools.getIMConversationMessages)

	svr.AddTool(mcp.NewTool("third_send_im_message",
		mcp.WithDescription("Send a text message to a user's IM conversation."),
		mcp.WithString("user_id", mcp.Required(), mcp.Description("User ID.")),
		mcp.WithString("conversation_id", mcp.Required(), mcp.Description("Conversation ID.")),
		mcp.WithString("content", mcp.Required(), mcp.Description("Message text.")),
	), tools.sendIMMessage)

	svr.AddTool(mcp.NewTool("third_recommend_products",
		mcp.WithDescription("Recommend products for a user."),
		mcp.WithString("user_id", mcp.Required(), mcp.Description("User ID.")),
	), tools.recommendProducts)

	svr.AddTool(mcp.NewTool("third_check_free_sample",
		mcp.WithDescription("Check whether a user can apply for a free sample of a product."),
		mcp.WithString("user_id", mcp.Required(), mcp.Description("User ID.")),
		mcp.WithString("product_id", mcp.Required(), mcp.Description("Product ID.")),
	), tools.checkFreeSample)

	svr.AddTool(mcp.NewTool("third_add_selection_cart",
		mcp.WithDescription("Add products to a user's selection cart."),
		mcp.WithString("user_id", mcp.Required(), mcp.Description("User ID.")),
		mcp.WithArray("product_ids", mcp.Required(), mcp.Description("Product IDs."), mcp.WithStringItems()),
	), tools.addSelectionCart)

	svr.AddTool(mcp.NewTool("third_get_selection_cart",
		mcp.WithDescription("Get product IDs already added to a user's selection cart."),
		mcp.WithString("user_id", mcp.Required(), mcp.Description("User ID.")),
	), tools.getSelectionCart)

	svr.AddTool(mcp.NewTool("third_apply_free_sample",
		mcp.WithDescription("Apply for free samples for selected products."),
		mcp.WithString("user_id", mcp.Required(), mcp.Description("User ID.")),
		mcp.WithArray("product_ids", mcp.Required(), mcp.Description("Product IDs."), mcp.WithStringItems()),
	), tools.applyFreeSample)

	svr.AddTool(mcp.NewTool("third_buy_sample",
		mcp.WithDescription("Buy paid samples for selected products."),
		mcp.WithString("user_id", mcp.Required(), mcp.Description("User ID.")),
		mcp.WithArray("product_ids", mcp.Required(), mcp.Description("Product IDs."), mcp.WithStringItems()),
	), tools.buySample)

	svr.AddTool(mcp.NewTool("third_get_fulfillment_status",
		mcp.WithDescription("Get fulfillment status for a user's related products, or for specified product IDs."),
		mcp.WithString("user_id", mcp.Required(), mcp.Description("User ID.")),
		mcp.WithArray("product_ids", mcp.Description("Optional product IDs. Empty means all user-related products."), mcp.WithStringItems()),
	), tools.getFulfillmentStatus)

	return svr
}

// listIMConversations 调用第三方 RPC 获取指定用户的 IM 会话 ID 列表。
func (s *mcpToolSet) listIMConversations(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// MCP 参数已按 schema 约束，这里只负责映射到 thrift 请求结构。
	resp, err := s.client.ListIMConversations(ctx, &thirdrpc.ListIMConversationsRequest{
		UserId: request.GetString("user_id", ""),
	})
	return toolResult(resp, err)
}

// getIMConversationMessages 调用第三方 RPC 获取某个 IM 会话的消息列表。
func (s *mcpToolSet) getIMConversationMessages(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// user_id 和 conversation_id 都来自模型工具调用参数，保持字段名与 schema 一致。
	resp, err := s.client.GetIMConversationMessages(ctx, &thirdrpc.GetIMConversationMessagesRequest{
		UserId:         request.GetString("user_id", ""),
		ConversationId: request.GetString("conversation_id", ""),
	})
	return toolResult(resp, err)
}

// sendIMMessage 调用第三方 RPC 向指定 IM 会话发送文本消息。
func (s *mcpToolSet) sendIMMessage(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// content 只传文本，结构化卡片等能力由第三方服务自行扩展。
	resp, err := s.client.SendIMMessage(ctx, &thirdrpc.SendIMMessageRequest{
		UserId:         request.GetString("user_id", ""),
		ConversationId: request.GetString("conversation_id", ""),
		Content:        request.GetString("content", ""),
	})
	return toolResult(resp, err)
}

// recommendProducts 调用第三方 RPC 为用户获取商品推荐。
func (s *mcpToolSet) recommendProducts(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resp, err := s.client.RecommendProducts(ctx, &thirdrpc.RecommendProductsRequest{
		UserId: request.GetString("user_id", ""),
	})
	return toolResult(resp, err)
}

// checkFreeSample 调用第三方 RPC 检查用户是否可申请指定商品免费样品。
func (s *mcpToolSet) checkFreeSample(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resp, err := s.client.CheckFreeSample(ctx, &thirdrpc.CheckFreeSampleRequest{
		UserId:    request.GetString("user_id", ""),
		ProductId: request.GetString("product_id", ""),
	})
	return toolResult(resp, err)
}

// addSelectionCart 调用第三方 RPC 把商品加入用户选品车。
func (s *mcpToolSet) addSelectionCart(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// product_ids 使用 MCP 的数组参数读取，避免模型把多个商品拼成逗号字符串。
	resp, err := s.client.AddSelectionCart(ctx, &thirdrpc.AddSelectionCartRequest{
		UserId:     request.GetString("user_id", ""),
		ProductIds: request.GetStringSlice("product_ids", nil),
	})
	return toolResult(resp, err)
}

// getSelectionCart 调用第三方 RPC 获取用户当前选品车内容。
func (s *mcpToolSet) getSelectionCart(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resp, err := s.client.GetSelectionCart(ctx, &thirdrpc.GetSelectionCartRequest{
		UserId: request.GetString("user_id", ""),
	})
	return toolResult(resp, err)
}

// applyFreeSample 调用第三方 RPC 为用户申请选中商品的免费样品。
func (s *mcpToolSet) applyFreeSample(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resp, err := s.client.ApplyFreeSample(ctx, &thirdrpc.ApplyFreeSampleRequest{
		UserId:     request.GetString("user_id", ""),
		ProductIds: request.GetStringSlice("product_ids", nil),
	})
	return toolResult(resp, err)
}

// buySample 调用第三方 RPC 为用户购买选中商品的付费样品。
func (s *mcpToolSet) buySample(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resp, err := s.client.BuySample(ctx, &thirdrpc.BuySampleRequest{
		UserId:     request.GetString("user_id", ""),
		ProductIds: request.GetStringSlice("product_ids", nil),
	})
	return toolResult(resp, err)
}

// getFulfillmentStatus 调用第三方 RPC 查询用户相关商品或指定商品的履约状态。
func (s *mcpToolSet) getFulfillmentStatus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// product_ids 为空时遵循第三方接口语义，查询该用户相关的全部商品状态。
	resp, err := s.client.GetFulfillmentStatus(ctx, &thirdrpc.GetFulfillmentStatusRequest{
		UserId:     request.GetString("user_id", ""),
		ProductIds: request.GetStringSlice("product_ids", nil),
	})
	return toolResult(resp, err)
}

// toolResult 把第三方 RPC 响应包装成 MCP 文本结果。
func toolResult(resp any, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return nil, err
	}
	// MCP 工具统一返回 JSON 文本，让模型基于真实 RPC 响应做后续判断。
	data, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}
	return mcp.NewToolResultText(string(data)), nil
}
