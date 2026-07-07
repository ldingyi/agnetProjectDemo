package application

import (
	"encoding/json"

	"agnet-project-demo/backend/src/agent"
	"agnet-project-demo/backend/src/agent/agents"
	agentsession "agnet-project-demo/backend/src/agent/session"
	"agnet-project-demo/backend/src/agent/tools/imsummary"
	"agnet-project-demo/third/kitex_gen/third"
	"agnet-project-demo/third/kitex_gen/third/thirdbusinessservice"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"agnet-project-demo/backend/kitex_gen/chat"
	"agnet-project-demo/backend/src/infrastructure"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/cloudwego/kitex/client"
)

type AgentChatService struct {
	agent            *agents.ChatModelAgent
	sessionDir       string
	cfg              infrastructure.Config
	thirdClient      thirdbusinessservice.Client
	imSummaryService *imsummary.Service
}

func NewAgentChatService(ctx context.Context, cfg infrastructure.Config) (*AgentChatService, error) {
	thirdClient, err := thirdbusinessservice.NewClient(
		cfg.Third.ServiceName,
		client.WithHostPorts(cfg.Third.TargetAddr),
	)
	if err != nil {
		return nil, fmt.Errorf("new third rpc client: %w", err)
	}
	imSummaryService, err := imsummary.NewService(ctx, cfg, thirdClient)
	if err != nil {
		return nil, fmt.Errorf("new im summary service: %w", err)
	}
	chatAgent, err := agents.NewChatModelAgentWithTools(ctx, cfg, []tool.BaseTool{imSummaryService.Tool()})
	if err != nil {
		return nil, fmt.Errorf("new chat model agents: %w", err)
	}

	return &AgentChatService{
		agent:            chatAgent,
		sessionDir:       cfg.Session.Dir,
		cfg:              cfg,
		thirdClient:      thirdClient,
		imSummaryService: imSummaryService,
	}, nil
}

func (s *AgentChatService) Login(ctx context.Context, req *chat.LoginRequest) (*chat.LoginResponse, error) {
	userID := strings.TrimSpace(req.GetUserId())
	resp, err := s.thirdClient.GetUserByID(ctx, &third.GetUserByIDRequest{UserId: userID})
	if err != nil {
		return nil, err
	}
	if resp == nil || !resp.GetFound() || resp.GetUser() == nil {
		return &chat.LoginResponse{
			Success: false,
			UserId:  userID,
			Message: "user not found",
		}, nil
	}

	user := resp.GetUser()
	return &chat.LoginResponse{
		Success:  true,
		UserId:   user.GetUserId(),
		Username: user.GetName(),
		Message:  "ok",
	}, nil
}

func (s *AgentChatService) Chat(ctx context.Context, req *chat.ChatRequest) (*chat.ChatResponse, error) {
	session, userMsg, err := s.prepareSession(req)
	if err != nil {
		return nil, err
	}
	if shouldRunIMSummary(req, userMsg) {
		assistantMsg, err := s.buildIMSummaryAssistantMessage(ctx, req.GetUserId())
		if err != nil {
			return nil, err
		}
		if err := session.Append(userMsg); err != nil {
			return nil, err
		}
		if err := session.Append(assistantMsg); err != nil {
			return nil, err
		}
		return &chat.ChatResponse{
			ConversationId: session.ID(),
			Message:        assistantMsg.GetContent(),
			ContentType:    assistantMsg.GetContentType(),
			Payload:        assistantMsg.GetPayload(),
		}, nil
	}

	messages := withUserContext(req.GetUserId(), agent.BuildEinoMessages(append(session.Messages(), userMsg)))
	content, err := s.agent.Generate(ctx, messages)
	if err != nil {
		return nil, err
	}
	if err := session.Append(userMsg); err != nil {
		return nil, err
	}
	if err := session.Append(agent.TextMessage("assistant", content)); err != nil {
		return nil, err
	}

	return &chat.ChatResponse{
		ConversationId: session.ID(),
		Message:        content,
		ContentType:    agent.ContentTypeText,
	}, nil
}

func (s *AgentChatService) ChatStream(ctx context.Context, req *chat.ChatRequest, stream chat.AgentChatService_ChatStreamServer) error {
	session, userMsg, err := s.prepareSession(req)
	if err != nil {
		return err
	}

	sender := agent.NewStreamDeltaSender(ctx, session.ID(), stream)
	if shouldRunIMSummary(req, userMsg) {
		assistantMsg, err := s.buildIMSummaryAssistantMessage(ctx, req.GetUserId())
		if err != nil {
			return err
		}
		if err := session.Append(userMsg); err != nil {
			return err
		}
		if err := session.Append(assistantMsg); err != nil {
			return err
		}
		if err := sender.SendStructured(assistantMsg.GetContentType(), assistantMsg.GetPayload(), assistantMsg.GetContent()); err != nil {
			return err
		}
		return sender.SendDone()
	}

	messages := withUserContext(req.GetUserId(), agent.BuildEinoMessages(append(session.Messages(), userMsg)))
	var assistantText strings.Builder
	err = s.agent.Stream(ctx, messages, func(delta string) error {
		assistantText.WriteString(delta)
		return sender.SendDelta(delta)
	})
	if err != nil {
		return err
	}
	if err := session.Append(userMsg); err != nil {
		return err
	}
	if err := session.Append(agent.TextMessage("assistant", assistantText.String())); err != nil {
		return err
	}

	return sender.SendDone()
}

func (s *AgentChatService) CreateSession(ctx context.Context, req *chat.CreateSessionRequest) (*chat.CreateSessionResponse, error) {
	store, err := s.sessionStore(req.GetUserId())
	if err != nil {
		return nil, err
	}
	session, err := store.Create()
	if err != nil {
		return nil, err
	}
	return &chat.CreateSessionResponse{Session: toSessionInfo(session.Info())}, nil
}

func (s *AgentChatService) ListSessions(ctx context.Context, req *chat.ListSessionsRequest) (*chat.ListSessionsResponse, error) {
	store, err := s.sessionStore(req.GetUserId())
	if err != nil {
		return nil, err
	}
	infos, err := store.List()
	if err != nil {
		return nil, err
	}

	sessions := make([]*chat.SessionInfo, 0, len(infos))
	for _, info := range infos {
		sessions = append(sessions, toSessionInfo(info))
	}
	return &chat.ListSessionsResponse{Sessions: sessions}, nil
}

func (s *AgentChatService) GetSession(ctx context.Context, req *chat.GetSessionRequest) (*chat.GetSessionResponse, error) {
	store, err := s.sessionStore(req.GetUserId())
	if err != nil {
		return nil, err
	}
	session, err := store.Get(req.GetId())
	if err != nil {
		return nil, err
	}

	return &chat.GetSessionResponse{
		Session:  toSessionInfo(session.Info()),
		Messages: toChatMessages(session.Messages()),
	}, nil
}

func (s *AgentChatService) GetIMChatSummary(ctx context.Context, req *chat.IMChatSummaryRequest) (*chat.IMChatSummaryResponse, error) {
	summary, err := s.summarizeIMWithTrace(ctx, req.GetUserId())
	if err != nil {
		return &chat.IMChatSummaryResponse{
			Success: false,
			Error:   imsummary.UserFacingError(err),
			ConversationSummaries: &chat.IMConversationSummaryGroups{
				Agreed:       []*chat.IMSummaryCard{},
				Rejected:     []*chat.IMSummaryCard{},
				NeedFollowUp: []*chat.IMSummaryCard{},
			},
			NewOffers_: []*chat.IMSummaryCard{},
			UpdatedAt:  time.Now().Format(time.RFC3339),
		}, nil
	}

	return &chat.IMChatSummaryResponse{
		Success:    true,
		NewOffers_: toIMSummaryCards(summary.NewOffers),
		ConversationSummaries: &chat.IMConversationSummaryGroups{
			Agreed:       toIMSummaryCards(summary.ConversationSummaries.Agreed),
			Rejected:     toIMSummaryCards(summary.ConversationSummaries.Rejected),
			NeedFollowUp: toIMSummaryCards(summary.ConversationSummaries.NeedFollowUp),
		},
		UpdatedAt: summary.UpdatedAt,
	}, nil
}

func (s *AgentChatService) prepareSession(req *chat.ChatRequest) (*agentsession.Session, *chat.ChatMessage, error) {
	store, err := s.sessionStore(req.GetUserId())
	if err != nil {
		return nil, nil, err
	}
	session, err := store.GetOrCreate(req.GetConversationId())
	if err != nil {
		return nil, nil, err
	}

	userMsg := lastUserMessage(req.GetMessages())
	if userMsg == nil {
		userMsg = agent.TextMessage("user", "")
	}
	return session, userMsg, nil
}

func (s *AgentChatService) sessionStore(userID string) (*agentsession.Store, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("user_id is required")
	}
	return agentsession.NewStore(filepath.Join(s.sessionDir, userID))
}

func lastUserMessage(messages []*chat.ChatMessage) *chat.ChatMessage {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i] != nil && messages[i].GetRole() == "user" {
			return messages[i]
		}
	}
	if len(messages) > 0 {
		return messages[len(messages)-1]
	}
	return nil
}

func toSessionInfo(info agentsession.Info) *chat.SessionInfo {
	return &chat.SessionInfo{
		Id:        info.ID,
		Title:     info.Title,
		CreatedAt: formatTime(info.CreatedAt),
		UpdatedAt: formatTime(info.UpdatedAt),
	}
}

func toChatMessages(messages []*chat.ChatMessage) []*chat.ChatMessage {
	result := make([]*chat.ChatMessage, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		result = append(result, message)
	}
	return result
}

func (s *AgentChatService) buildIMSummaryAssistantMessage(ctx context.Context, userID string) (*chat.ChatMessage, error) {
	summary, err := s.summarizeIMWithTrace(ctx, userID)
	if err != nil {
		return agent.TextMessage("assistant", imsummary.UserFacingError(err)), nil
	}
	response := buildIMChatSummaryResponse(summary)
	payload, err := json.Marshal(response)
	if err != nil {
		return nil, err
	}
	return agent.StructuredMessage("assistant", agent.ContentTypeIMChatSummary, imSummaryAssistantText(summary), string(payload)), nil
}

func (s *AgentChatService) summarizeIMWithTrace(ctx context.Context, userID string) (*imsummary.Summary, error) {
	trace := infrastructure.NewRequestTrace(s.cfg)
	defer trace.Finish()
	return s.imSummaryService.SummarizeWithCallbacks(ctx, userID, trace.Handler())
}

func buildIMChatSummaryResponse(summary *imsummary.Summary) *chat.IMChatSummaryResponse {
	if summary == nil {
		return &chat.IMChatSummaryResponse{
			Success:    false,
			Error:      "IM 聊天总结暂时不可用，请稍后再试。",
			NewOffers_: []*chat.IMSummaryCard{},
			ConversationSummaries: &chat.IMConversationSummaryGroups{
				Agreed:       []*chat.IMSummaryCard{},
				Rejected:     []*chat.IMSummaryCard{},
				NeedFollowUp: []*chat.IMSummaryCard{},
			},
			UpdatedAt: time.Now().Format(time.RFC3339),
		}
	}
	return &chat.IMChatSummaryResponse{
		Success:    true,
		NewOffers_: toIMSummaryCards(summary.NewOffers),
		ConversationSummaries: &chat.IMConversationSummaryGroups{
			Agreed:       toIMSummaryCards(summary.ConversationSummaries.Agreed),
			Rejected:     toIMSummaryCards(summary.ConversationSummaries.Rejected),
			NeedFollowUp: toIMSummaryCards(summary.ConversationSummaries.NeedFollowUp),
		},
		UpdatedAt: summary.UpdatedAt,
	}
}

func imSummaryAssistantText(summary *imsummary.Summary) string {
	if summary == nil {
		return "IM 聊天总结暂时不可用，请稍后再试。"
	}
	return fmt.Sprintf(
		"已完成 IM 聊天总结：新邀约 %d 个，已同意 %d 个，已拒绝 %d 个，需继续沟通 %d 个。你可以继续基于这些会话让我草拟回复、申请样品或处理选品。",
		len(summary.NewOffers),
		len(summary.ConversationSummaries.Agreed),
		len(summary.ConversationSummaries.Rejected),
		len(summary.ConversationSummaries.NeedFollowUp),
	)
}

func shouldRunIMSummary(req *chat.ChatRequest, userMsg *chat.ChatMessage) bool {
	content := strings.TrimSpace(userMsg.GetContent())
	if content == "" {
		return false
	}
	return strings.Contains(content, "总结") &&
		(strings.Contains(strings.ToLower(content), "im") || strings.Contains(content, "会话") || strings.Contains(content, "聊天"))
}

func toIMSummaryCards(cards []imsummary.SummaryCard) []*chat.IMSummaryCard {
	result := make([]*chat.IMSummaryCard, 0, len(cards))
	for _, card := range cards {
		result = append(result, &chat.IMSummaryCard{
			ConversationId: card.ConversationID,
			Title:          card.Title,
			Summary:        card.Summary,
			LatestTime:     card.LatestTime,
			ProductIds:     card.ProductIDs,
			ProductNames:   card.ProductNames,
			Evidence:       card.Evidence,
			AnswerStatus:   card.AnswerStatus,
			NextAction:     card.NextAction,
		})
	}
	return result
}

func withUserContext(userID string, messages []*schema.Message) []*schema.Message {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return messages
	}
	result := make([]*schema.Message, 0, len(messages)+1)
	result = append(result, schema.SystemMessage("当前登录用户ID是 "+userID+"。调用第三方工具时，user_id 参数必须使用这个用户ID。"))
	result = append(result, messages...)
	return result
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
