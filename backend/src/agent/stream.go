package agent

import (
	"context"
	"errors"
	"io"
	"strings"

	"agnet-project-demo/backend/kitex_gen/chat"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

type DeltaHandler func(delta string) error

// ConsumeAgentEvents 消费 ADK Agent 事件流，并把可展示的 assistant 文本交给回调。
func ConsumeAgentEvents(iter *adk.AsyncIterator[*adk.AgentEvent], onDelta DeltaHandler) error {
	for {
		event, ok := iter.Next()
		if !ok {
			return nil
		}
		if event == nil {
			continue
		}
		if event.Err != nil {
			return event.Err
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			// 跳过没有消息输出的中间事件，避免把工具状态等内部事件透给前端。
			continue
		}

		// ADK 会输出模型中间态、工具调用和最终回答；这里只把最终 assistant 文本透给前端。
		msgOutput := event.Output.MessageOutput
		if msgOutput.IsStreaming && msgOutput.MessageStream != nil {
			msg, err := ConsumeMessageStream(msgOutput.MessageStream)
			if err != nil {
				return err
			}
			if shouldExposeMessage(msg) {
				if err := onDelta(msg.Content); err != nil {
					return err
				}
			}
			continue
		}
		if shouldExposeMessage(msgOutput.Message) {
			if err := onDelta(msgOutput.Message.Content); err != nil {
				return err
			}
		}
	}
}

// ConsumeMessageStream 合并 Eino 消息流中的所有 chunk，返回完整消息。
func ConsumeMessageStream(stream *schema.StreamReader[*schema.Message]) (*schema.Message, error) {
	defer stream.Close()
	chunks := make([]*schema.Message, 0)
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			if len(chunks) == 0 {
				return nil, nil
			}
			// Eino 提供消息拼接能力，用它合并 content、tool call 等增量字段。
			return schema.ConcatMessages(chunks)
		}
		if err != nil {
			return nil, err
		}
		if msg != nil {
			chunks = append(chunks, msg)
		}
	}
}

// shouldExposeMessage 判断一条消息是否应该展示为前端聊天气泡。
func shouldExposeMessage(msg *schema.Message) bool {
	if msg == nil {
		return false
	}
	if msg.Role != schema.Assistant {
		return false
	}
	// 工具调用计划和工具返回是内部 ReAct 过程，不应该作为聊天气泡展示。
	if len(msg.ToolCalls) > 0 || msg.ToolCallID != "" {
		return false
	}
	return strings.TrimSpace(msg.Content) != ""
}

type StreamDeltaSender struct {
	ctx            context.Context
	conversationID string
	stream         chat.AgentChatService_ChatStreamServer
}

// NewStreamDeltaSender 创建流式响应发送器，统一填充会话 ID 和上下文。
func NewStreamDeltaSender(ctx context.Context, conversationID string, stream chat.AgentChatService_ChatStreamServer) *StreamDeltaSender {
	return &StreamDeltaSender{
		ctx:            ctx,
		conversationID: conversationID,
		stream:         stream,
	}
}

// SendDelta 发送一段普通文本增量。
func (s *StreamDeltaSender) SendDelta(delta string) error {
	return s.stream.Send(s.ctx, &chat.ChatStreamChunk{
		ConversationId: s.conversationID,
		Delta:          delta,
		Done:           false,
	})
}

// SendDone 发送流式响应结束标记。
func (s *StreamDeltaSender) SendDone() error {
	return s.stream.Send(s.ctx, &chat.ChatStreamChunk{
		ConversationId: s.conversationID,
		Done:           true,
	})
}

// SendStructured 发送一条结构化消息，供前端按 contentType 和 payload 渲染。
func (s *StreamDeltaSender) SendStructured(contentType string, payload string, content string) error {
	// 结构化消息只发一个 chunk，delta 用于普通文本展示，payload 用于前端卡片渲染。
	return s.stream.Send(s.ctx, &chat.ChatStreamChunk{
		ConversationId: s.conversationID,
		Delta:          content,
		Done:           false,
		ContentType:    contentType,
		Payload:        payload,
	})
}
