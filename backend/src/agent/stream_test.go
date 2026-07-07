package agent

import (
	"testing"

	"github.com/cloudwego/eino/schema"
)

// TestShouldExposeMessage 验证只有最终 assistant 文本会暴露给前端。
func TestShouldExposeMessage(t *testing.T) {
	tests := []struct {
		name string
		msg  *schema.Message
		want bool
	}{
		{
			name: "final assistant answer",
			msg:  schema.AssistantMessage("done", nil),
			want: true,
		},
		{
			name: "assistant tool call planning text",
			msg: &schema.Message{
				Role:    schema.Assistant,
				Content: "I will call a tool.",
				ToolCalls: []schema.ToolCall{
					{Function: schema.FunctionCall{Name: "skill", Arguments: `{"skill":"im"}`}},
				},
			},
			want: false,
		},
		{
			name: "tool result",
			msg: &schema.Message{
				Role:       schema.Tool,
				Content:    "internal skill content",
				ToolCallID: "call_1",
			},
			want: false,
		},
		{
			name: "empty assistant message",
			msg:  schema.AssistantMessage("  ", nil),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldExposeMessage(tt.msg); got != tt.want {
				t.Fatalf("shouldExposeMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}
