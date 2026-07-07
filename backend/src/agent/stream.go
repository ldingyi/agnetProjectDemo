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
			continue
		}

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

func ConsumeMessageStream(stream *schema.StreamReader[*schema.Message]) (*schema.Message, error) {
	defer stream.Close()
	chunks := make([]*schema.Message, 0)
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			if len(chunks) == 0 {
				return nil, nil
			}
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

func shouldExposeMessage(msg *schema.Message) bool {
	if msg == nil {
		return false
	}
	if msg.Role != schema.Assistant {
		return false
	}
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

func NewStreamDeltaSender(ctx context.Context, conversationID string, stream chat.AgentChatService_ChatStreamServer) *StreamDeltaSender {
	return &StreamDeltaSender{
		ctx:            ctx,
		conversationID: conversationID,
		stream:         stream,
	}
}

func (s *StreamDeltaSender) SendDelta(delta string) error {
	return s.stream.Send(s.ctx, &chat.ChatStreamChunk{
		ConversationId: s.conversationID,
		Delta:          delta,
		Done:           false,
	})
}

func (s *StreamDeltaSender) SendDone() error {
	return s.stream.Send(s.ctx, &chat.ChatStreamChunk{
		ConversationId: s.conversationID,
		Done:           true,
	})
}

func (s *StreamDeltaSender) SendStructured(contentType string, payload string, content string) error {
	return s.stream.Send(s.ctx, &chat.ChatStreamChunk{
		ConversationId: s.conversationID,
		Delta:          content,
		Done:           false,
		ContentType:    contentType,
		Payload:        payload,
	})
}
