package main

import (
	"agnet-project-demo/backend/src/application"
	"agnet-project-demo/backend/src/infrastructure"
	"context"
	"log"

	chat "agnet-project-demo/backend/kitex_gen/chat"
)

// AgentChatServiceImpl 是 Kitex 生成接口与 application 服务之间的薄适配层。
type AgentChatServiceImpl struct {
	inner *application.AgentChatService
}

func newAgentChatServiceImpl() *AgentChatServiceImpl {
	inner, err := application.NewAgentChatService(context.Background(), infrastructure.LoadConfig())
	if err != nil {
		log.Fatalf("init agents chat service: %v", err)
	}
	return &AgentChatServiceImpl{inner: inner}
}

// Chat implements the AgentChatServiceImpl interface.
func (s *AgentChatServiceImpl) Chat(ctx context.Context, req *chat.ChatRequest) (resp *chat.ChatResponse, err error) {
	return s.inner.Chat(ctx, req)
}

func (s *AgentChatServiceImpl) ChatStream(ctx context.Context, req *chat.ChatRequest, stream chat.AgentChatService_ChatStreamServer) (err error) {
	return s.inner.ChatStream(ctx, req, stream)
}

// CreateSession implements the AgentChatServiceImpl interface.
func (s *AgentChatServiceImpl) CreateSession(ctx context.Context, req *chat.CreateSessionRequest) (resp *chat.CreateSessionResponse, err error) {
	return s.inner.CreateSession(ctx, req)
}

// ListSessions implements the AgentChatServiceImpl interface.
func (s *AgentChatServiceImpl) ListSessions(ctx context.Context, req *chat.ListSessionsRequest) (resp *chat.ListSessionsResponse, err error) {
	return s.inner.ListSessions(ctx, req)
}

// GetSession implements the AgentChatServiceImpl interface.
func (s *AgentChatServiceImpl) GetSession(ctx context.Context, req *chat.GetSessionRequest) (resp *chat.GetSessionResponse, err error) {
	return s.inner.GetSession(ctx, req)
}

// Login implements the AgentChatServiceImpl interface.
func (s *AgentChatServiceImpl) Login(ctx context.Context, req *chat.LoginRequest) (resp *chat.LoginResponse, err error) {
	return s.inner.Login(ctx, req)
}
