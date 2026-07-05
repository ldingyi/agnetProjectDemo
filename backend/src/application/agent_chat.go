package application

import (
	"agnet-project-demo/backend/src/agent"
	"agnet-project-demo/backend/src/agent/agents"
	agentsession "agnet-project-demo/backend/src/agent/session"
	"agnet-project-demo/third/kitex_gen/third"
	"agnet-project-demo/third/kitex_gen/third/thirdbusinessservice"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"agnet-project-demo/backend/kitex_gen/chat"
	"agnet-project-demo/backend/src/infrastructure"

	"github.com/cloudwego/eino/schema"
	"github.com/cloudwego/kitex/client"
)

type AgentChatService struct {
	agent       *agents.ChatModelAgent
	sessionDir  string
	thirdClient thirdbusinessservice.Client
}

func NewAgentChatService(ctx context.Context, cfg infrastructure.Config) (*AgentChatService, error) {
	chatAgent, err := agents.NewChatModelAgent(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("new chat model agents: %w", err)
	}
	thirdClient, err := thirdbusinessservice.NewClient(
		cfg.Third.ServiceName,
		client.WithHostPorts(cfg.Third.TargetAddr),
	)
	if err != nil {
		return nil, fmt.Errorf("new third rpc client: %w", err)
	}

	return &AgentChatService{
		agent:       chatAgent,
		sessionDir:  cfg.Session.Dir,
		thirdClient: thirdClient,
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

	messages := withUserContext(req.GetUserId(), append(session.Messages(), userMsg))
	content, err := s.agent.Generate(ctx, messages)
	if err != nil {
		return nil, err
	}
	if err := session.Append(userMsg); err != nil {
		return nil, err
	}
	if err := session.Append(schema.AssistantMessage(content, nil)); err != nil {
		return nil, err
	}

	return &chat.ChatResponse{
		ConversationId: session.ID(),
		Message:        content,
	}, nil
}

func (s *AgentChatService) ChatStream(ctx context.Context, req *chat.ChatRequest, stream chat.AgentChatService_ChatStreamServer) error {
	session, userMsg, err := s.prepareSession(req)
	if err != nil {
		return err
	}

	messages := withUserContext(req.GetUserId(), append(session.Messages(), userMsg))
	sender := agent.NewStreamDeltaSender(ctx, session.ID(), stream)
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
	if err := session.Append(schema.AssistantMessage(assistantText.String(), nil)); err != nil {
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

func (s *AgentChatService) prepareSession(req *chat.ChatRequest) (*agentsession.Session, *schema.Message, error) {
	store, err := s.sessionStore(req.GetUserId())
	if err != nil {
		return nil, nil, err
	}
	session, err := store.GetOrCreate(req.GetConversationId())
	if err != nil {
		return nil, nil, err
	}

	inputMessages := agent.BuildEinoMessages(req.GetMessages())
	userMsg := lastUserMessage(inputMessages)
	if userMsg == nil {
		userMsg = schema.UserMessage("")
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

func lastUserMessage(messages []*schema.Message) *schema.Message {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i] != nil && messages[i].Role == schema.User {
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

func toChatMessages(messages []*schema.Message) []*chat.ChatMessage {
	result := make([]*chat.ChatMessage, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		result = append(result, &chat.ChatMessage{
			Role:    string(message.Role),
			Content: message.Content,
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
