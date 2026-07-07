package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"agnet-project-demo/api/internal/rpc"
	"agnet-project-demo/backend/kitex_gen/chat"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol/consts"
	"github.com/cloudwego/hertz/pkg/protocol/sse"
)

type ChatHandler struct {
	rpc *rpc.Client
}

type chatStreamEvent struct {
	ConversationID string `json:"conversation_id"`
	Delta          string `json:"delta"`
	Done           bool   `json:"done"`
	ContentType    string `json:"content_type"`
	Payload        string `json:"payload"`
}

func NewChatHandler(rpcClient *rpc.Client) *ChatHandler {
	return &ChatHandler{rpc: rpcClient}
}

func (h *ChatHandler) Login(ctx context.Context, c *app.RequestContext) {
	var req chat.LoginRequest
	if err := c.BindAndValidate(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	resp, err := h.rpc.Chat().Login(ctx, &req)
	if err != nil {
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !resp.GetSuccess() {
		c.JSON(consts.StatusUnauthorized, resp)
		return
	}
	c.JSON(consts.StatusOK, resp)
}

func (h *ChatHandler) Chat(ctx context.Context, c *app.RequestContext) {
	var req chat.ChatRequest
	if err := c.BindAndValidate(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	resp, err := h.rpc.Chat().Chat(ctx, &req)
	if err != nil {
		c.JSON(consts.StatusNotImplemented, map[string]string{"error": err.Error()})
		return
	}

	c.JSON(consts.StatusOK, resp)
}

func (h *ChatHandler) ChatStream(ctx context.Context, c *app.RequestContext) {
	var req chat.ChatRequest
	if err := c.BindAndValidate(&req); err != nil {
		c.JSON(consts.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	stream, err := h.rpc.ChatStream().ChatStream(ctx, &req)
	if err != nil {
		c.JSON(consts.StatusNotImplemented, map[string]string{"error": err.Error()})
		return
	}

	c.SetStatusCode(consts.StatusOK)
	c.Response.Header.Set("Cache-Control", "no-cache")
	c.Response.Header.Set("Connection", "keep-alive")
	c.Response.Header.SetContentType("text/event-stream; charset=utf-8")
	c.Response.ImmediateHeaderFlush = true
	writer := sse.NewWriter(c)
	defer writer.Close()

	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			_ = writer.WriteEvent("", "error", []byte(err.Error()))
			return
		}
		payload, err := json.Marshal(chatStreamEvent{
			ConversationID: chunk.GetConversationId(),
			Delta:          chunk.GetDelta(),
			Done:           chunk.GetDone(),
			ContentType:    chunk.GetContentType(),
			Payload:        chunk.GetPayload(),
		})
		if err != nil {
			_ = writer.WriteEvent("", "error", []byte("stream encode failed"))
			return
		}
		if err := writer.WriteEvent("", "message", payload); err != nil {
			return
		}
		if chunk.GetDone() {
			return
		}
	}
}

func (h *ChatHandler) CreateSession(ctx context.Context, c *app.RequestContext) {
	resp, err := h.rpc.Chat().CreateSession(ctx, &chat.CreateSessionRequest{UserId: string(c.Query("user_id"))})
	if err != nil {
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	c.JSON(consts.StatusOK, resp)
}

func (h *ChatHandler) ListSessions(ctx context.Context, c *app.RequestContext) {
	resp, err := h.rpc.Chat().ListSessions(ctx, &chat.ListSessionsRequest{UserId: string(c.Query("user_id"))})
	if err != nil {
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	c.JSON(consts.StatusOK, resp)
}

func (h *ChatHandler) GetSession(ctx context.Context, c *app.RequestContext) {
	id := c.Param("id")
	resp, err := h.rpc.Chat().GetSession(ctx, &chat.GetSessionRequest{Id: id, UserId: string(c.Query("user_id"))})
	if err != nil {
		c.JSON(consts.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	c.JSON(consts.StatusOK, resp)
}

func RegisterRoutes(h *server.Hertz, chatHandler *ChatHandler) {
	api := h.Group("/api")
	api.POST("/login", chatHandler.Login)
	api.POST("/chat", chatHandler.Chat)
	api.POST("/chat/stream", chatHandler.ChatStream)
	api.POST("/sessions", chatHandler.CreateSession)
	api.GET("/sessions", chatHandler.ListSessions)
	api.GET("/sessions/:id", chatHandler.GetSession)
}

func Health(ctx context.Context, c *app.RequestContext) {
	c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
