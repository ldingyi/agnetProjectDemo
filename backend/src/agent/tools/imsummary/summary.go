package imsummary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"agnet-project-demo/backend/src/infrastructure"
	thirdrpc "agnet-project-demo/third/kitex_gen/third"
	"agnet-project-demo/third/kitex_gen/third/thirdbusinessservice"

	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const imSummaryToolName = "im_chat_summary"

var (
	errMissingUserID = errors.New("missing user_id")
	errFormatFailed  = errors.New("summary json format failed")
)

type Request struct {
	UserID string `json:"user_id" jsonschema:"required" jsonschema_description:"Current login user ID."`
}

type Summary struct {
	NewOffers             []SummaryCard             `json:"new_offers"`
	ConversationSummaries ConversationSummaryGroups `json:"conversation_summaries"`
	UpdatedAt             string                    `json:"updated_at"`
}

type ConversationSummaryGroups struct {
	Agreed       []SummaryCard `json:"agreed"`
	Rejected     []SummaryCard `json:"rejected"`
	NeedFollowUp []SummaryCard `json:"need_follow_up"`
}

type SummaryCard struct {
	ConversationID string   `json:"conversation_id"`
	Title          string   `json:"title"`
	Summary        string   `json:"summary"`
	LatestTime     string   `json:"latest_time"`
	ProductIDs     []string `json:"product_ids"`
	ProductNames   []string `json:"product_names"`
	Evidence       []string `json:"evidence"`
	AnswerStatus   string   `json:"answer_status"`
	NextAction     string   `json:"next_action"`
}

type toolOutput struct {
	Success bool     `json:"success"`
	Error   string   `json:"error,omitempty"`
	Summary *Summary `json:"summary,omitempty"`
}

type conversationBundle struct {
	UserID        string              `json:"user_id"`
	Conversations []conversationBrief `json:"conversations"`
}

type conversationBrief struct {
	ConversationID string         `json:"conversation_id"`
	Messages       []messageBrief `json:"messages"`
}

type messageBrief struct {
	MessageID      string               `json:"message_id"`
	Sender         string               `json:"sender"`
	MessageType    string               `json:"message_type"`
	Content        string               `json:"content"`
	SendTime       string               `json:"send_time"`
	InvitationCard *invitationCardBrief `json:"invitation_card,omitempty"`
	ContactCard    *contactCardBrief    `json:"contact_card,omitempty"`
}

type invitationCardBrief struct {
	Intro    string             `json:"intro"`
	Products []productInfoBrief `json:"products"`
}

type productInfoBrief struct {
	ProductID  string  `json:"product_id"`
	Name       string  `json:"name"`
	Commission float64 `json:"commission"`
}

type contactCardBrief struct {
	Name   string `json:"name"`
	Phone  string `json:"phone"`
	Wechat string `json:"wechat"`
}

type llmOutput struct {
	Bundle *conversationBundle
	Raw    string
}

// NewTool 构建 IM 总结 Graph，并包装成 Agent 可调用的 tool。
func NewTool(ctx context.Context, cfg infrastructure.Config, thirdClient thirdbusinessservice.Client) (tool.BaseTool, error) {
	if thirdClient == nil {
		return nil, fmt.Errorf("third client is required")
	}
	if strings.TrimSpace(cfg.DeepSeek.APIKey) == "" {
		return nil, fmt.Errorf("DEEPSEEK_API_KEY is required")
	}

	summaryModel, err := deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
		APIKey:             cfg.DeepSeek.APIKey,
		BaseURL:            cfg.DeepSeek.BaseURL,
		Model:              cfg.DeepSeek.Model,
		Temperature:        0,
		ResponseFormatType: deepseek.ResponseFormatTypeJSONObject,
	})
	if err != nil {
		return nil, err
	}

	// 总结模型要求 JSON object 输出，并用低温度降低同一输入下的结构波动。
	summaryGraph, err := buildGraph(ctx, thirdClient, summaryModel)
	if err != nil {
		return nil, err
	}
	return wrapGraphAsTool(summaryGraph)
}

// wrapGraphAsTool 把已编译的 Graph Runnable 暴露成 ChatModelAgent 的普通工具。
func wrapGraphAsTool(summaryGraph compose.Runnable[Request, *Summary]) (tool.InvokableTool, error) {
	if summaryGraph == nil {
		return nil, fmt.Errorf("summary graph is required")
	}
	return utils.InferTool(imSummaryToolName,
		"Run the IM chat summary graph for the current user and return structured JSON cards.",
		func(ctx context.Context, input Request) (*toolOutput, error) {
			userID := strings.TrimSpace(input.UserID)
			if userID == "" {
				return &toolOutput{
					Success: false,
					Error:   userFacingError(errMissingUserID),
				}, nil
			}
			summary, err := summaryGraph.Invoke(ctx, Request{UserID: userID})
			if err != nil {
				return &toolOutput{
					Success: false,
					Error:   userFacingError(err),
				}, nil
			}
			return &toolOutput{Success: true, Summary: summary}, nil
		})
}

// userFacingError 把内部错误归类成可以直接展示给模型和用户的文案。
func userFacingError(err error) string {
	switch {
	case errors.Is(err, errMissingUserID):
		return "缺少用户信息，无法生成 IM 提醒。"
	case errors.Is(err, errFormatFailed):
		return "总结结果格式化失败，已尝试修复但仍无法生成可展示内容。"
	default:
		return "IM 聊天总结暂时不可用，请稍后再试。"
	}
}

// buildGraph 构造 IM 总结 Graph，串联会话加载、模型总结和 JSON 校验修复。
func buildGraph(ctx context.Context, thirdClient thirdbusinessservice.Client, summaryModel model.BaseChatModel) (compose.Runnable[Request, *Summary], error) {
	g := compose.NewGraph[Request, *Summary]()

	// Graph 节点保持单一职责：先取原始会话，再让模型总结，最后做 JSON 校验和修复。
	if err := g.AddLambdaNode("load_im_conversations", compose.InvokableLambda(func(ctx context.Context, req Request) (*conversationBundle, error) {
		return loadIMConversations(ctx, thirdClient, req.UserID)
	})); err != nil {
		return nil, err
	}

	if err := g.AddLambdaNode("summarize_with_llm", compose.InvokableLambda(func(ctx context.Context, bundle *conversationBundle) (*llmOutput, error) {
		return summarizeWithLLM(ctx, summaryModel, bundle)
	})); err != nil {
		return nil, err
	}

	if err := g.AddLambdaNode("validate_or_repair_json", compose.InvokableLambda(func(ctx context.Context, output *llmOutput) (*Summary, error) {
		return validateOrRepairJSON(ctx, summaryModel, output)
	})); err != nil {
		return nil, err
	}

	if err := g.AddEdge(compose.START, "load_im_conversations"); err != nil {
		return nil, err
	}
	// 线性边明确表达流程顺序：先拿数据，再生成，再校验修复。
	if err := g.AddEdge("load_im_conversations", "summarize_with_llm"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("summarize_with_llm", "validate_or_repair_json"); err != nil {
		return nil, err
	}
	if err := g.AddEdge("validate_or_repair_json", compose.END); err != nil {
		return nil, err
	}

	return g.Compile(ctx,
		compose.WithGraphName("im_chat_summary_graph"),
		compose.WithNodeTriggerMode(compose.AllPredecessor),
	)
}

// loadIMConversations 从第三方业务服务读取用户所有 IM 会话和消息。
func loadIMConversations(ctx context.Context, cli thirdbusinessservice.Client, userID string) (*conversationBundle, error) {
	resp, err := cli.ListIMConversations(ctx, &thirdrpc.ListIMConversationsRequest{UserId: userID})
	if err != nil {
		return nil, fmt.Errorf("list im conversations: %w", err)
	}

	ids := resp.GetConversationIds()
	sort.Strings(ids)

	// 排序后逐个拉取消息，保证同样输入下 prompt 顺序稳定，便于调试和回放。
	bundle := &conversationBundle{
		UserID:        userID,
		Conversations: make([]conversationBrief, 0, len(ids)),
	}
	for _, id := range ids {
		msgResp, err := cli.GetIMConversationMessages(ctx, &thirdrpc.GetIMConversationMessagesRequest{
			UserId:         userID,
			ConversationId: id,
		})
		if err != nil {
			return nil, fmt.Errorf("get im conversation messages: %w", err)
		}

		conversation := conversationBrief{
			ConversationID: id,
			Messages:       make([]messageBrief, 0, len(msgResp.GetMessages())),
		}
		for _, msg := range msgResp.GetMessages() {
			conversation.Messages = append(conversation.Messages, toMessageBrief(msg))
		}
		bundle.Conversations = append(bundle.Conversations, conversation)
	}

	return bundle, nil
}

// summarizeWithLLM 把会话 bundle 输入模型，生成原始 JSON 总结文本。
func summarizeWithLLM(ctx context.Context, cm model.BaseChatModel, bundle *conversationBundle) (*llmOutput, error) {
	if bundle == nil {
		return nil, fmt.Errorf("conversation bundle is nil")
	}
	if len(bundle.Conversations) == 0 {
		// 没有会话时直接返回空结构，避免无意义调用模型。
		raw, _ := json.Marshal(emptySummary())
		return &llmOutput{Bundle: bundle, Raw: string(raw)}, nil
	}

	source, err := json.MarshalIndent(bundle.Conversations, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal conversations: %w", err)
	}

	// prompt 中同时给出当前用户和完整会话 JSON，让模型只基于可见数据分类。
	msg, err := cm.Generate(ctx, []*schema.Message{
		schema.SystemMessage(imSummarySystemPrompt()),
		schema.UserMessage(fmt.Sprintf("当前用户 ID：%s\n\nIM 会话原始数据 JSON：\n%s", bundle.UserID, string(source))),
	})
	if err != nil {
		return nil, fmt.Errorf("generate im summary: %w", err)
	}

	return &llmOutput{Bundle: bundle, Raw: msg.Content}, nil
}

// validateOrRepairJSON 解析模型输出；失败时再调用模型做一次 JSON 修复。
func validateOrRepairJSON(ctx context.Context, cm model.BaseChatModel, output *llmOutput) (*Summary, error) {
	if output == nil {
		return nil, fmt.Errorf("%w: empty llm output", errFormatFailed)
	}

	summary, err := parseSummary(output.Raw)
	if err == nil {
		return summary, nil
	}

	// LLM 偶尔会输出 Markdown 或缺字段；用原始会话和解析错误再修复一次。
	repaired, repairErr := repairSummaryJSON(ctx, cm, output, err)
	if repairErr != nil {
		return nil, fmt.Errorf("%w: %v; repair failed: %v", errFormatFailed, err, repairErr)
	}

	summary, err = parseSummary(repaired)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errFormatFailed, err)
	}
	return summary, nil
}

// repairSummaryJSON 根据原始会话数据、上次输出和解析错误重新生成合法 JSON。
func repairSummaryJSON(ctx context.Context, cm model.BaseChatModel, output *llmOutput, parseErr error) (string, error) {
	source, _ := json.MarshalIndent(output.Bundle.Conversations, "", "  ")
	// 修复 prompt 不要求延续上次格式，只要求重新输出符合展示协议的 JSON。
	msg, err := cm.Generate(ctx, []*schema.Message{
		schema.SystemMessage(imSummaryRepairPrompt()),
		schema.UserMessage(fmt.Sprintf("上一次输出不是合法的展示 JSON。\n错误原因：%v\n\n请基于原始会话数据重新输出合法 JSON。\n\n原始会话数据：\n%s\n\n上一次输出：\n%s", parseErr, string(source), output.Raw)),
	})
	if err != nil {
		return "", fmt.Errorf("repair im summary json: %w", err)
	}
	return msg.Content, nil
}

// parseSummary 从模型原始输出中提取 JSON 对象，并反序列化为 Summary。
func parseSummary(raw string) (*Summary, error) {
	raw = extractJSONObject(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty json output")
	}

	var summary Summary
	if err := json.Unmarshal([]byte(raw), &summary); err != nil {
		return nil, err
	}
	// 解析后统一做默认值和必填字段校验，避免前端收到半结构化数据。
	if err := normalizeSummary(&summary); err != nil {
		return nil, err
	}
	return &summary, nil
}

// normalizeSummary 补齐 Summary 默认值，并校验每张卡片的关键字段。
func normalizeSummary(summary *Summary) error {
	if summary == nil {
		return fmt.Errorf("summary is nil")
	}
	if summary.UpdatedAt == "" {
		// 模型缺少更新时间时使用服务端当前时间，保证 payload 有稳定展示字段。
		summary.UpdatedAt = time.Now().Format(time.RFC3339)
	}

	for i := range summary.NewOffers {
		normalizeCard(&summary.NewOffers[i], "new_offer")
		if summary.NewOffers[i].ConversationID == "" {
			return fmt.Errorf("new_offers[%d].conversation_id is required", i)
		}
	}
	if err := normalizeCards(summary.ConversationSummaries.Agreed, "agreed", "conversation_summaries.agreed"); err != nil {
		return err
	}
	if err := normalizeCards(summary.ConversationSummaries.Rejected, "rejected", "conversation_summaries.rejected"); err != nil {
		return err
	}
	if err := normalizeCards(summary.ConversationSummaries.NeedFollowUp, "need_follow_up", "conversation_summaries.need_follow_up"); err != nil {
		return err
	}
	return nil
}

// normalizeCards 归一化同一分类下的卡片，并带上路径信息返回校验错误。
func normalizeCards(cards []SummaryCard, status string, path string) error {
	for i := range cards {
		normalizeCard(&cards[i], status)
		if cards[i].ConversationID == "" {
			return fmt.Errorf("%s[%d].conversation_id is required", path, i)
		}
	}
	return nil
}

// normalizeCard 清理单张总结卡片的文本字段和数组字段。
func normalizeCard(card *SummaryCard, status string) {
	// 归一化阶段只补展示所需的默认值，不新增模型没有依据的业务事实。
	card.ConversationID = strings.TrimSpace(card.ConversationID)
	card.Title = strings.TrimSpace(card.Title)
	card.Summary = strings.TrimSpace(card.Summary)
	card.LatestTime = strings.TrimSpace(card.LatestTime)
	card.AnswerStatus = strings.TrimSpace(card.AnswerStatus)
	card.NextAction = strings.TrimSpace(card.NextAction)
	if card.Title == "" && card.ConversationID != "" {
		card.Title = "会话 " + card.ConversationID
	}
	if card.AnswerStatus == "" {
		card.AnswerStatus = status
	}
	card.ProductIDs = compactStrings(card.ProductIDs)
	card.ProductNames = compactStrings(card.ProductNames)
	card.Evidence = compactStrings(card.Evidence)
}

// compactStrings 清理字符串数组中的空值和重复值。
func compactStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		// 用 map 保留首次出现顺序，同时去掉后续重复项。
		seen[value] = true
		result = append(result, value)
	}
	return result
}

// extractJSONObject 从模型输出中剥离代码块标记并截取最外层 JSON 对象。
func extractJSONObject(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end < start {
		return ""
	}
	// 只取第一个左花括号到最后一个右花括号，兼容模型额外输出的说明文字。
	return raw[start : end+1]
}

// emptySummary 返回没有会话数据时的标准空总结。
func emptySummary() *Summary {
	return &Summary{
		NewOffers: []SummaryCard{},
		ConversationSummaries: ConversationSummaryGroups{
			Agreed:       []SummaryCard{},
			Rejected:     []SummaryCard{},
			NeedFollowUp: []SummaryCard{},
		},
		UpdatedAt: time.Now().Format(time.RFC3339),
	}
}

// toMessageBrief 把第三方 IM 消息转换成模型 prompt 使用的精简消息结构。
func toMessageBrief(msg *thirdrpc.IMMessage) messageBrief {
	if msg == nil {
		return messageBrief{}
	}
	return messageBrief{
		MessageID:      msg.GetMessageId(),
		Sender:         msg.GetSender().String(),
		MessageType:    msg.GetMessageType().String(),
		Content:        msg.GetContent(),
		SendTime:       msg.GetSendTime(),
		InvitationCard: toInvitationCardBrief(msg.GetInvitationCard()),
		ContactCard:    toContactCardBrief(msg.GetContactCard()),
	}
}

// toInvitationCardBrief 把邀约卡转换成只包含总结所需字段的结构。
func toInvitationCardBrief(card *thirdrpc.IMInvitationCard) *invitationCardBrief {
	if card == nil {
		return nil
	}
	products := make([]productInfoBrief, 0, len(card.GetProducts()))
	for _, product := range card.GetProducts() {
		if product == nil {
			continue
		}
		// prompt 只需要商品 ID、名称和佣金，避免把无关字段增加给模型的噪音。
		products = append(products, productInfoBrief{
			ProductID:  product.GetProductId(),
			Name:       product.GetName(),
			Commission: product.GetCommission(),
		})
	}
	return &invitationCardBrief{
		Intro:    card.GetIntro(),
		Products: products,
	}
}

// toContactCardBrief 把联系方式卡片转换成精简结构。
func toContactCardBrief(card *thirdrpc.IMContactInfo) *contactCardBrief {
	if card == nil {
		return nil
	}
	return &contactCardBrief{
		Name:   card.GetName(),
		Phone:  card.GetPhone(),
		Wechat: card.GetWechat(),
	}
}

// imSummarySystemPrompt 返回首次总结使用的系统提示词。
func imSummarySystemPrompt() string {
	return `你是联盟撮合 IM 会话分析器。你只输出 JSON，不输出 Markdown 或解释文字。

请把输入的 IM 会话整理成前端可展示的系统提示卡片，输出必须是合法 JSON 对象，结构如下：
{
  "new_offers": [],
  "conversation_summaries": {
    "agreed": [],
    "rejected": [],
    "need_follow_up": []
  },
  "updated_at": "RFC3339 时间"
}

每张卡片结构：
{
  "conversation_id": "会话 ID，必填",
  "title": "短标题，优先使用商品或会话主题，不要编造商家名",
  "summary": "一句话业务总结",
  "latest_time": "相关最新消息时间",
  "product_ids": ["商品 ID"],
  "product_names": ["商品名"],
  "evidence": ["关键证据，最多 3 条，来自消息内容"],
  "answer_status": "new_offer|agreed|rejected|need_follow_up",
  "next_action": "建议用户下一步做什么"
}

分类规则：
1. new_offers：商家发出邀约或邀约卡后，用户还没有在该邀约之后回复。
2. agreed：用户询问高佣、免费申样、寄样、内容要求等，对方已明确同意或给出可执行支持。
3. rejected：对方明确拒绝，或明确表示不满足条件、名额已满、暂不支持。
4. need_follow_up：对方没有明确答复，或提出条件、需要继续沟通、需要用户补充排期/信息。

约束：
- 只能基于输入消息判断，不要编造商家名、商品、承诺、审核状态或发货状态。
- 如果商品来自邀约卡，优先使用 invitation_card.products 中的 product_id、name、commission。
- 如果一个会话同时满足多个分类，选择对用户当前最需要处理的分类。
- 每个会话最多输出一张卡片。
- 数组不存在内容时输出空数组。`
}

// imSummaryRepairPrompt 返回 JSON 修复阶段使用的系统提示词。
func imSummaryRepairPrompt() string {
	return `你是 JSON 修复器。你只输出合法 JSON 对象，不输出 Markdown 或解释文字。

请根据原始 IM 会话数据重新生成可展示的 IM 总结 JSON。必须符合以下结构：
{
  "new_offers": [],
  "conversation_summaries": {
    "agreed": [],
    "rejected": [],
    "need_follow_up": []
  },
  "updated_at": "RFC3339 时间"
}

每张卡片必须包含 conversation_id、title、summary、latest_time、product_ids、product_names、evidence、answer_status、next_action。`
}
