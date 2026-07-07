package agent

import (
	"agnet-project-demo/backend/kitex_gen/chat"
	agentsession "agnet-project-demo/backend/src/agent/session"
	"strings"

	"github.com/cloudwego/eino/schema"
)

const (
	ContentTypeText          = "text"
	ContentTypeIMChatSummary = "im_chat_summary"
)

// BuildEinoMessages 把 RPC 聊天消息列表转换成 Eino 模型消息列表。
func BuildEinoMessages(messages []*chat.ChatMessage) []*schema.Message {
	if len(messages) == 0 {
		// 模型调用至少需要一条消息，空会话用空 user message 占位。
		return []*schema.Message{schema.UserMessage("")}
	}

	result := make([]*schema.Message, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		// 单条消息转换集中在 BuildEinoMessage，保持 role 和 content 处理一致。
		result = append(result, BuildEinoMessage(message))
	}

	if len(result) == 0 {
		return []*schema.Message{schema.UserMessage("")}
	}
	return result
}

// BuildEinoContextMessages 把 agent 上下文消息转换成 Eino 模型消息列表。
func BuildEinoContextMessages(messages []*agentsession.ContextMessage) []*schema.Message {
	if len(messages) == 0 {
		return []*schema.Message{schema.UserMessage("")}
	}

	result := make([]*schema.Message, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		result = append(result, BuildEinoContextMessage(message))
	}

	if len(result) == 0 {
		return []*schema.Message{schema.UserMessage("")}
	}
	return result
}

// BuildEinoContextMessage 根据 agent 上下文角色构造对应的 Eino schema.Message。
func BuildEinoContextMessage(message *agentsession.ContextMessage) *schema.Message {
	if message == nil {
		return schema.UserMessage("")
	}
	content := strings.TrimSpace(message.Content)
	switch message.Role {
	case "system":
		return schema.SystemMessage(content)
	case "assistant":
		return schema.AssistantMessage(content, nil)
	default:
		return schema.UserMessage(content)
	}
}

// BuildEinoMessage 根据聊天消息角色构造对应的 Eino schema.Message。
func BuildEinoMessage(message *chat.ChatMessage) *schema.Message {
	content := EinoContent(message)
	switch message.GetRole() {
	case "system":
		return schema.SystemMessage(content)
	case "assistant":
		return schema.AssistantMessage(content, nil)
	default:
		return schema.UserMessage(content)
	}
}

// EinoContent 提取模型可见文本，并把结构化消息 payload 回灌到上下文中。
func EinoContent(message *chat.ChatMessage) string {
	if message == nil {
		return ""
	}
	content := strings.TrimSpace(message.GetContent())
	if message.GetContentType() != ContentTypeIMChatSummary {
		return content
	}

	// IM 总结展示给前端的是结构化 payload；继续对话时也要把它放回模型上下文。
	var b strings.Builder
	if content != "" {
		b.WriteString(content)
	}
	payload := strings.TrimSpace(message.GetPayload())
	if payload != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("以下是上一轮 IM 聊天总结的结构化数据，后续用户可能会引用其中的会话、商品或分类：\n")
		b.WriteString(payload)
	}
	return b.String()
}

// TextMessage 构造普通文本聊天消息。
func TextMessage(role string, content string) *chat.ChatMessage {
	return &chat.ChatMessage{
		Role:        role,
		Content:     content,
		ContentType: ContentTypeText,
	}
}

// StructuredMessage 构造带 contentType 和 payload 的结构化聊天消息。
func StructuredMessage(role string, contentType string, content string, payload string) *chat.ChatMessage {
	return &chat.ChatMessage{
		Role:        role,
		Content:     content,
		ContentType: contentType,
		Payload:     payload,
	}
}

// ContextMessageFromChatMessage 把用户可见聊天消息转换成 agent 上下文消息。
func ContextMessageFromChatMessage(message *chat.ChatMessage) *agentsession.ContextMessage {
	if message == nil {
		return nil
	}
	return &agentsession.ContextMessage{
		Role:    message.GetRole(),
		Content: EinoContent(message),
	}
}

// ContextTextMessage 构造只给 agent 上下文使用的文本消息。
func ContextTextMessage(role string, content string) *agentsession.ContextMessage {
	return &agentsession.ContextMessage{
		Role:    role,
		Content: strings.TrimSpace(content),
	}
}
