package infrastructure

import (
	"context"
	"encoding/json"
	"fmt"

	mcptool "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/kitex/client"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"

	"agnet-project-demo/third/kitex_gen/third"
	"agnet-project-demo/third/kitex_gen/third/thirdbusinessservice"
)

type thirdMCPServer struct {
	client thirdbusinessservice.Client
}

func NewThirdMCPTools(ctx context.Context, cfg Config) ([]tool.BaseTool, error) {
	thirdClient, err := thirdbusinessservice.NewClient(
		cfg.Third.ServiceName,
		client.WithHostPorts(cfg.Third.TargetAddr),
	)
	if err != nil {
		return nil, fmt.Errorf("new third rpc client: %w", err)
	}

	svr := newThirdMCPServer(thirdClient)
	mcpClient, err := mcpclient.NewInProcessClient(svr)
	if err != nil {
		return nil, fmt.Errorf("new in-process mcp client: %w", err)
	}
	if err := mcpClient.Start(ctx); err != nil {
		return nil, fmt.Errorf("start in-process mcp client: %w", err)
	}

	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "agnet-project-demo-backend",
		Version: "1.0.0",
	}
	if _, err := mcpClient.Initialize(ctx, initRequest); err != nil {
		return nil, fmt.Errorf("initialize in-process mcp client: %w", err)
	}

	tools, err := mcptool.GetTools(ctx, &mcptool.Config{Cli: mcpClient})
	if err != nil {
		return nil, fmt.Errorf("get third mcp tools: %w", err)
	}
	return tools, nil
}

func newThirdMCPServer(cli thirdbusinessservice.Client) *mcpserver.MCPServer {
	wrapper := &thirdMCPServer{client: cli}
	svr := mcpserver.NewMCPServer("third-business-mcp", mcp.LATEST_PROTOCOL_VERSION)

	svr.AddTool(mcp.NewTool("third_list_im_conversations",
		mcp.WithDescription("List IM conversation IDs for a user."),
		mcp.WithString("user_id", mcp.Required(), mcp.Description("User ID.")),
	), wrapper.listIMConversations)

	svr.AddTool(mcp.NewTool("third_get_im_conversation_messages",
		mcp.WithDescription("Get messages from a user's IM conversation."),
		mcp.WithString("user_id", mcp.Required(), mcp.Description("User ID.")),
		mcp.WithString("conversation_id", mcp.Required(), mcp.Description("Conversation ID.")),
	), wrapper.getIMConversationMessages)

	svr.AddTool(mcp.NewTool("third_send_im_message",
		mcp.WithDescription("Send a text message to a user's IM conversation."),
		mcp.WithString("user_id", mcp.Required(), mcp.Description("User ID.")),
		mcp.WithString("conversation_id", mcp.Required(), mcp.Description("Conversation ID.")),
		mcp.WithString("content", mcp.Required(), mcp.Description("Message text.")),
	), wrapper.sendIMMessage)

	svr.AddTool(mcp.NewTool("third_recommend_products",
		mcp.WithDescription("Recommend products for a user."),
		mcp.WithString("user_id", mcp.Required(), mcp.Description("User ID.")),
	), wrapper.recommendProducts)

	svr.AddTool(mcp.NewTool("third_check_free_sample",
		mcp.WithDescription("Check whether a user can apply for a free sample of a product."),
		mcp.WithString("user_id", mcp.Required(), mcp.Description("User ID.")),
		mcp.WithString("product_id", mcp.Required(), mcp.Description("Product ID.")),
	), wrapper.checkFreeSample)

	svr.AddTool(mcp.NewTool("third_add_selection_cart",
		mcp.WithDescription("Add products to a user's selection cart."),
		mcp.WithString("user_id", mcp.Required(), mcp.Description("User ID.")),
		mcp.WithArray("product_ids", mcp.Required(), mcp.Description("Product IDs."), mcp.WithStringItems()),
	), wrapper.addSelectionCart)

	svr.AddTool(mcp.NewTool("third_get_selection_cart",
		mcp.WithDescription("Get product IDs already added to a user's selection cart."),
		mcp.WithString("user_id", mcp.Required(), mcp.Description("User ID.")),
	), wrapper.getSelectionCart)

	svr.AddTool(mcp.NewTool("third_apply_free_sample",
		mcp.WithDescription("Apply for free samples for selected products."),
		mcp.WithString("user_id", mcp.Required(), mcp.Description("User ID.")),
		mcp.WithArray("product_ids", mcp.Required(), mcp.Description("Product IDs."), mcp.WithStringItems()),
	), wrapper.applyFreeSample)

	svr.AddTool(mcp.NewTool("third_buy_sample",
		mcp.WithDescription("Buy paid samples for selected products."),
		mcp.WithString("user_id", mcp.Required(), mcp.Description("User ID.")),
		mcp.WithArray("product_ids", mcp.Required(), mcp.Description("Product IDs."), mcp.WithStringItems()),
	), wrapper.buySample)

	svr.AddTool(mcp.NewTool("third_get_fulfillment_status",
		mcp.WithDescription("Get fulfillment status for a user's related products, or for specified product IDs."),
		mcp.WithString("user_id", mcp.Required(), mcp.Description("User ID.")),
		mcp.WithArray("product_ids", mcp.Description("Optional product IDs. Empty means all user-related products."), mcp.WithStringItems()),
	), wrapper.getFulfillmentStatus)

	return svr
}

func (s *thirdMCPServer) listIMConversations(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resp, err := s.client.ListIMConversations(ctx, &third.ListIMConversationsRequest{
		UserId: request.GetString("user_id", ""),
	})
	return toolResult(resp, err)
}

func (s *thirdMCPServer) getIMConversationMessages(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resp, err := s.client.GetIMConversationMessages(ctx, &third.GetIMConversationMessagesRequest{
		UserId:         request.GetString("user_id", ""),
		ConversationId: request.GetString("conversation_id", ""),
	})
	return toolResult(resp, err)
}

func (s *thirdMCPServer) sendIMMessage(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resp, err := s.client.SendIMMessage(ctx, &third.SendIMMessageRequest{
		UserId:         request.GetString("user_id", ""),
		ConversationId: request.GetString("conversation_id", ""),
		Content:        request.GetString("content", ""),
	})
	return toolResult(resp, err)
}

func (s *thirdMCPServer) recommendProducts(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resp, err := s.client.RecommendProducts(ctx, &third.RecommendProductsRequest{
		UserId: request.GetString("user_id", ""),
	})
	return toolResult(resp, err)
}

func (s *thirdMCPServer) checkFreeSample(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resp, err := s.client.CheckFreeSample(ctx, &third.CheckFreeSampleRequest{
		UserId:    request.GetString("user_id", ""),
		ProductId: request.GetString("product_id", ""),
	})
	return toolResult(resp, err)
}

func (s *thirdMCPServer) addSelectionCart(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resp, err := s.client.AddSelectionCart(ctx, &third.AddSelectionCartRequest{
		UserId:     request.GetString("user_id", ""),
		ProductIds: request.GetStringSlice("product_ids", nil),
	})
	return toolResult(resp, err)
}

func (s *thirdMCPServer) getSelectionCart(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resp, err := s.client.GetSelectionCart(ctx, &third.GetSelectionCartRequest{
		UserId: request.GetString("user_id", ""),
	})
	return toolResult(resp, err)
}

func (s *thirdMCPServer) applyFreeSample(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resp, err := s.client.ApplyFreeSample(ctx, &third.ApplyFreeSampleRequest{
		UserId:     request.GetString("user_id", ""),
		ProductIds: request.GetStringSlice("product_ids", nil),
	})
	return toolResult(resp, err)
}

func (s *thirdMCPServer) buySample(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resp, err := s.client.BuySample(ctx, &third.BuySampleRequest{
		UserId:     request.GetString("user_id", ""),
		ProductIds: request.GetStringSlice("product_ids", nil),
	})
	return toolResult(resp, err)
}

func (s *thirdMCPServer) getFulfillmentStatus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	resp, err := s.client.GetFulfillmentStatus(ctx, &third.GetFulfillmentStatusRequest{
		UserId:     request.GetString("user_id", ""),
		ProductIds: request.GetStringSlice("product_ids", nil),
	})
	return toolResult(resp, err)
}

func toolResult(resp any, err error) (*mcp.CallToolResult, error) {
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}
	return mcp.NewToolResultText(string(data)), nil
}
