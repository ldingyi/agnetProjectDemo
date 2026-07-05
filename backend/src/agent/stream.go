package agent

import (
	"context"
	"errors"
	"io"

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
			if err := ConsumeMessageStream(msgOutput.MessageStream, onDelta); err != nil {
				return err
			}
			continue
		}
		if msgOutput.Message != nil && msgOutput.Message.Content != "" {
			if err := onDelta(msgOutput.Message.Content); err != nil {
				return err
			}
		}
	}
}

func ConsumeMessageStream(stream *schema.StreamReader[*schema.Message], onDelta DeltaHandler) error {
	defer stream.Close()
	for {
		msg, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if msg == nil || msg.Content == "" {
			continue
		}
		if err := onDelta(msg.Content); err != nil {
			return err
		}
	}
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
