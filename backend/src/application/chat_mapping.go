package application

import (
	"time"

	"agnet-project-demo/backend/kitex_gen/chat"
	"agnet-project-demo/backend/src/agent"
	"agnet-project-demo/backend/src/domain/chatthread"
)

// lastUserMessage 从请求消息中优先选择最后一条用户消息作为本轮输入。
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

// toSessionInfo 把用户可见聊天会话元信息转换成 RPC 响应结构。
func toSessionInfo(info chatthread.Info) *chat.SessionInfo {
	return &chat.SessionInfo{
		Id:        info.ID,
		Title:     info.Title,
		CreatedAt: formatTime(info.CreatedAt),
		UpdatedAt: formatTime(info.UpdatedAt),
	}
}

// toChatMessages 把用户可见聊天消息转换成 RPC 响应结构。
func toChatMessages(messages []*chatthread.Message) []*chat.ChatMessage {
	result := make([]*chat.ChatMessage, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		result = append(result, &chat.ChatMessage{
			Role:        message.Role,
			Content:     message.Content,
			ContentType: message.ContentType,
			Payload:     message.Payload,
		})
	}
	return result
}

func toThreadMessage(message *chat.ChatMessage) *chatthread.Message {
	if message == nil {
		return nil
	}
	return &chatthread.Message{
		Role:        message.GetRole(),
		Content:     message.GetContent(),
		ContentType: message.GetContentType(),
		Payload:     message.GetPayload(),
	}
}

func emptyUserMessage() *chat.ChatMessage {
	return agent.TextMessage("user", "")
}

// formatTime 把内部 time.Time 统一转换成 RPC 使用的 UTC RFC3339Nano 字符串。
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
