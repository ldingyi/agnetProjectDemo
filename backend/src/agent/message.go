package agent

import (
	"agnet-project-demo/backend/kitex_gen/chat"
	"strings"

	"github.com/cloudwego/eino/schema"
)

const (
	ContentTypeText          = "text"
	ContentTypeIMChatSummary = "im_chat_summary"
)

func BuildEinoMessages(messages []*chat.ChatMessage) []*schema.Message {
	if len(messages) == 0 {
		return []*schema.Message{schema.UserMessage("")}
	}

	result := make([]*schema.Message, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		result = append(result, BuildEinoMessage(message))
	}

	if len(result) == 0 {
		return []*schema.Message{schema.UserMessage("")}
	}
	return result
}

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

func EinoContent(message *chat.ChatMessage) string {
	if message == nil {
		return ""
	}
	content := strings.TrimSpace(message.GetContent())
	if message.GetContentType() != ContentTypeIMChatSummary {
		return content
	}

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

func TextMessage(role string, content string) *chat.ChatMessage {
	return &chat.ChatMessage{
		Role:        role,
		Content:     content,
		ContentType: ContentTypeText,
	}
}

func StructuredMessage(role string, contentType string, content string, payload string) *chat.ChatMessage {
	return &chat.ChatMessage{
		Role:        role,
		Content:     content,
		ContentType: contentType,
		Payload:     payload,
	}
}
