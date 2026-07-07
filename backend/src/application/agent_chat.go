package application

import (
	"agnet-project-demo/backend/src/agent"
	"agnet-project-demo/backend/src/agent/agents"
	agentsession "agnet-project-demo/backend/src/agent/session"
	"agnet-project-demo/backend/src/domain/chatthread"
	"agnet-project-demo/third/kitex_gen/third"
	"agnet-project-demo/third/kitex_gen/third/thirdbusinessservice"
	"context"
	"fmt"
	"strings"

	"agnet-project-demo/backend/kitex_gen/chat"
	"agnet-project-demo/backend/src/infrastructure"

	"github.com/cloudwego/eino/schema"
	"github.com/cloudwego/kitex/client"
)

type AgentChatService struct {
	agent         *agents.ChatModelAgent
	cfg           infrastructure.Config
	thirdClient   thirdbusinessservice.Client
	chatThreads   *chatthread.Manager
	agentSessions *agentsession.Manager
}

// NewAgentChatService 组装聊天服务依赖，包括登录 RPC client、Agent 和会话存储。
func NewAgentChatService(ctx context.Context, cfg infrastructure.Config) (*AgentChatService, error) {
	// application 层只保留登录流程需要的第三方 RPC client；Agent 工具由 Agent 内部统一组装。
	thirdClient, err := thirdbusinessservice.NewClient(
		cfg.Third.ServiceName,
		client.WithHostPorts(cfg.Third.TargetAddr),
	)
	if err != nil {
		return nil, fmt.Errorf("new third rpc client: %w", err)
	}
	chatAgent, err := agents.NewChatModelAgent(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("new chat model agents: %w", err)
	}

	return &AgentChatService{
		agent:         chatAgent,
		cfg:           cfg,
		thirdClient:   thirdClient,
		chatThreads:   chatthread.NewManager(cfg.Session.Dir),
		agentSessions: agentsession.NewManager(cfg.Session.Dir),
	}, nil
}

// Login 根据用户 ID 查询第三方用户信息，并返回前端登录态需要的基础字段。
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

	// 只把前端需要的字段透出，避免 RPC 用户对象结构变化直接影响聊天接口。
	user := resp.GetUser()
	return &chat.LoginResponse{
		Success:  true,
		UserId:   user.GetUserId(),
		Username: user.GetName(),
		Message:  "ok",
	}, nil
}

// Chat 处理非流式聊天请求，负责会话读写和 Agent 调用。
func (s *AgentChatService) Chat(ctx context.Context, req *chat.ChatRequest) (*chat.ChatResponse, error) {
	thread, agentSession, userMsg, err := s.prepareSession(req)
	if err != nil {
		return nil, err
	}

	// 所有聊天消息都把历史消息和当前用户 ID 一起交给 Agent，由模型决策是否调用工具。
	contextMessages := append(agentSession.Messages(), agent.ContextMessageFromChatMessage(userMsg))
	messages := withUserContext(req.GetUserId(), agent.BuildEinoContextMessages(contextMessages))
	content, err := s.agent.Generate(ctx, messages)
	if err != nil {
		return nil, err
	}
	assistantMsg := agent.TextMessage("assistant", content)
	// Agent 成功生成后再落盘，避免失败请求污染会话历史。
	if err := thread.Append(toThreadMessage(userMsg), toThreadMessage(assistantMsg)); err != nil {
		return nil, err
	}
	if err := agentSession.Append(
		agent.ContextMessageFromChatMessage(userMsg),
		agent.ContextMessageFromChatMessage(assistantMsg),
	); err != nil {
		return nil, err
	}

	return &chat.ChatResponse{
		ConversationId: thread.ID(),
		Message:        content,
		ContentType:    agent.ContentTypeText,
	}, nil
}

// ChatStream 处理流式聊天请求，逐段发送 assistant 文本并在结束时写入会话历史。
func (s *AgentChatService) ChatStream(ctx context.Context, req *chat.ChatRequest, stream chat.AgentChatService_ChatStreamServer) error {
	thread, agentSession, userMsg, err := s.prepareSession(req)
	if err != nil {
		return err
	}

	sender := agent.NewStreamDeltaSender(ctx, thread.ID(), stream)
	contextMessages := append(agentSession.Messages(), agent.ContextMessageFromChatMessage(userMsg))
	messages := withUserContext(req.GetUserId(), agent.BuildEinoContextMessages(contextMessages))
	var assistantText strings.Builder
	err = s.agent.Stream(ctx, messages, func(delta string) error {
		// 一边累积完整 assistant 文本用于落盘，一边把增量直接透传给前端。
		assistantText.WriteString(delta)
		return sender.SendDelta(delta)
	})
	if err != nil {
		return err
	}
	assistantMsg := agent.TextMessage("assistant", assistantText.String())
	if err := thread.Append(toThreadMessage(userMsg), toThreadMessage(assistantMsg)); err != nil {
		return err
	}
	if err := agentSession.Append(
		agent.ContextMessageFromChatMessage(userMsg),
		agent.ContextMessageFromChatMessage(assistantMsg),
	); err != nil {
		return err
	}

	return sender.SendDone()
}

// CreateSession 为指定用户创建一个新的本地会话文件。
func (s *AgentChatService) CreateSession(ctx context.Context, req *chat.CreateSessionRequest) (*chat.CreateSessionResponse, error) {
	thread, err := s.chatThreads.Create(req.GetUserId())
	if err != nil {
		return nil, err
	}
	if _, err := s.agentSessions.GetOrCreate(req.GetUserId(), thread.ID()); err != nil {
		return nil, err
	}
	return &chat.CreateSessionResponse{Session: toSessionInfo(thread.Info())}, nil
}

// ListSessions 返回指定用户的会话列表，按最近更新时间倒序排列。
func (s *AgentChatService) ListSessions(ctx context.Context, req *chat.ListSessionsRequest) (*chat.ListSessionsResponse, error) {
	infos, err := s.chatThreads.List(req.GetUserId())
	if err != nil {
		return nil, err
	}

	sessions := make([]*chat.SessionInfo, 0, len(infos))
	for _, info := range infos {
		// application 层负责把 domain/chatthread.Info 转成 thrift 生成的响应结构。
		sessions = append(sessions, toSessionInfo(info))
	}
	return &chat.ListSessionsResponse{Sessions: sessions}, nil
}

// GetSession 读取指定会话的元信息和历史消息。
func (s *AgentChatService) GetSession(ctx context.Context, req *chat.GetSessionRequest) (*chat.GetSessionResponse, error) {
	thread, err := s.chatThreads.Get(req.GetUserId(), req.GetId())
	if err != nil {
		return nil, err
	}

	return &chat.GetSessionResponse{
		Session:  toSessionInfo(thread.Info()),
		Messages: toChatMessages(thread.Messages()),
	}, nil
}

// prepareSession 根据请求确定会话对象，并从请求消息中提取当前用户消息。
func (s *AgentChatService) prepareSession(req *chat.ChatRequest) (*chatthread.ChatThread, *agentsession.Session, *chat.ChatMessage, error) {
	// conversation_id 当前仍是对外兼容字段，内部语义是 chat thread id。
	thread, err := s.chatThreads.GetOrCreate(req.GetUserId(), req.GetConversationId())
	if err != nil {
		return nil, nil, nil, err
	}
	agentSession, err := s.agentSessions.GetOrCreate(req.GetUserId(), thread.ID())
	if err != nil {
		return nil, nil, nil, err
	}

	userMsg := lastUserMessage(req.GetMessages())
	if userMsg == nil {
		// 没有用户消息时补一个空消息，保证 Agent 输入始终满足至少一条 user message。
		userMsg = emptyUserMessage()
	}
	return thread, agentSession, userMsg, nil
}

// withUserContext 在模型消息前注入当前登录用户 ID，约束工具调用使用正确身份。
func withUserContext(userID string, messages []*schema.Message) []*schema.Message {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return messages
	}
	// user_id 不交给模型猜测，作为系统消息注入，确保工具调用使用当前登录用户。
	result := make([]*schema.Message, 0, len(messages)+1)
	result = append(result, schema.SystemMessage("当前登录用户ID是 "+userID+"。调用第三方工具时，user_id 参数必须使用这个用户ID。"))
	result = append(result, messages...)
	return result
}
