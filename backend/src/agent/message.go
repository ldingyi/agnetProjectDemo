package agent

import (
	"agnet-project-demo/backend/kitex_gen/chat"

	"github.com/cloudwego/eino/schema"
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
	content := message.GetContent()
	switch message.GetRole() {
	case "system":
		return schema.SystemMessage(content)
	case "assistant":
		return schema.AssistantMessage(content, nil)
	default:
		return schema.UserMessage(content)
	}
}
