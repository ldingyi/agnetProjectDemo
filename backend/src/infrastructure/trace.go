package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	callbacktemplate "github.com/cloudwego/eino/utils/callbacks"
	"github.com/google/uuid"
)

type RequestTrace struct {
	id        string
	startedAt time.Time
	dir       string
	mu        sync.Mutex
	wg        sync.WaitGroup
	nodes     []traceNode
}

type requestTraceFile struct {
	ID        string      `json:"id"`
	StartedAt string      `json:"started_at"`
	EndedAt   string      `json:"ended_at"`
	Nodes     []traceNode `json:"nodes"`
}

type traceNode struct {
	Index     int    `json:"index"`
	StartedAt string `json:"started_at,omitempty"`
	EndedAt   string `json:"ended_at,omitempty"`
	Kind      string `json:"kind"`
	Name      string `json:"name,omitempty"`
	Type      string `json:"type,omitempty"`
	Component string `json:"component,omitempty"`
	Model     string `json:"model,omitempty"`
	Input     any    `json:"input,omitempty"`
	Output    any    `json:"output,omitempty"`
	Error     string `json:"error,omitempty"`
}

type traceMessages struct {
	Messages []traceMessage `json:"messages"`
}

type traceMessage struct {
	Role             string          `json:"role"`
	Content          string          `json:"content,omitempty"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	ToolCalls        []traceToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string          `json:"tool_call_id,omitempty"`
}

type traceToolCall struct {
	ID        string `json:"id,omitempty"`
	Type      string `json:"type,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type traceModelOutput struct {
	Message *traceMessage `json:"message,omitempty"`
	Usage   any           `json:"usage,omitempty"`
}

type traceToolInput struct {
	ArgumentsInJSON string `json:"arguments_in_json,omitempty"`
}

type traceToolOutput struct {
	Response   string `json:"response,omitempty"`
	ToolOutput any    `json:"tool_output,omitempty"`
}

func NewRequestTrace(cfg Config) *RequestTrace {
	return &RequestTrace{
		id:        uuid.NewString(),
		startedAt: time.Now().UTC(),
		dir:       traceDir(cfg.Trace.Path),
		nodes:     make([]traceNode, 0),
	}
}

func NewEinoTraceRunOptions(trace *RequestTrace) []adk.AgentRunOption {
	if trace == nil {
		return nil
	}
	return []adk.AgentRunOption{
		adk.WithCallbacks(trace.Handler()),
	}
}

func NewEinoTraceHandler(cfg Config) callbacks.Handler {
	return NewRequestTrace(cfg).Handler()
}

func (t *RequestTrace) Handler() callbacks.Handler {
	return callbacktemplate.NewHandlerHelper().
		Graph(t.composeHandler("graph")).
		Lambda(t.composeHandler("lambda")).
		ChatModel(t.modelHandler()).
		Tool(t.toolHandler()).
		ToolsNode(t.toolsNodeHandler()).
		Handler()
}

func (t *RequestTrace) Finish() {
	if t == nil || t.dir == "" {
		return
	}
	t.wg.Wait()

	t.mu.Lock()
	nodes := make([]traceNode, len(t.nodes))
	copy(nodes, t.nodes)
	t.mu.Unlock()

	sort.SliceStable(nodes, func(i, j int) bool {
		return nodes[i].Index < nodes[j].Index
	})

	data, err := json.MarshalIndent(requestTraceFile{
		ID:        t.id,
		StartedAt: t.startedAt.Format(time.RFC3339Nano),
		EndedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Nodes:     nodes,
	}, "", "  ")
	if err != nil {
		return
	}

	if err := os.MkdirAll(t.dir, 0o755); err != nil {
		return
	}
	fileName := "trace-" + t.startedAt.Format("20060102T150405.000000000Z") + "-" + t.id + ".json"
	_ = os.WriteFile(filepath.Join(t.dir, fileName), append(data, '\n'), 0o644)
}

func (t *RequestTrace) addNode(node traceNode) {
	if t == nil {
		return
	}
	t.mu.Lock()
	node.Index = len(t.nodes) + 1
	t.nodes = append(t.nodes, node)
	t.mu.Unlock()
}

func (t *RequestTrace) modelHandler() *callbacktemplate.ModelCallbackHandler {
	return &callbacktemplate.ModelCallbackHandler{
		OnStart: func(ctx context.Context, info *callbacks.RunInfo, input *model.CallbackInput) context.Context {
			if input == nil {
				return ctx
			}
			t.addNode(traceNode{
				StartedAt: nowTraceTime(),
				Kind:      "model",
				Name:      runName(info),
				Type:      runType(info),
				Component: runComponent(info),
				Model:     modelName(input.Config),
				Input:     traceMessages{Messages: toTraceMessages(input.Messages)},
			})
			return ctx
		},
		OnEnd: func(ctx context.Context, info *callbacks.RunInfo, output *model.CallbackOutput) context.Context {
			if output == nil {
				return ctx
			}
			t.addNode(traceNode{
				EndedAt:   nowTraceTime(),
				Kind:      modelDecision(output.Message),
				Name:      runName(info),
				Type:      runType(info),
				Component: runComponent(info),
				Model:     modelName(output.Config),
				Output: traceModelOutput{
					Message: toTraceMessage(output.Message),
					Usage:   output.TokenUsage,
				},
			})
			return ctx
		},
		OnEndWithStreamOutput: func(ctx context.Context, info *callbacks.RunInfo, output *schema.StreamReader[*model.CallbackOutput]) context.Context {
			t.wg.Add(1)
			go t.consumeModelStream(info, output)
			return ctx
		},
		OnError: func(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
			t.addNode(traceNode{
				EndedAt:   nowTraceTime(),
				Kind:      "model_error",
				Name:      runName(info),
				Type:      runType(info),
				Component: runComponent(info),
				Error:     err.Error(),
			})
			return ctx
		},
	}
}

func (t *RequestTrace) toolHandler() *callbacktemplate.ToolCallbackHandler {
	return &callbacktemplate.ToolCallbackHandler{
		OnStart: func(ctx context.Context, info *callbacks.RunInfo, input *tool.CallbackInput) context.Context {
			if input == nil {
				return ctx
			}
			t.addNode(traceNode{
				StartedAt: nowTraceTime(),
				Kind:      "tool_call",
				Name:      runName(info),
				Type:      runType(info),
				Component: runComponent(info),
				Input: traceToolInput{
					ArgumentsInJSON: input.ArgumentsInJSON,
				},
			})
			return ctx
		},
		OnEnd: func(ctx context.Context, info *callbacks.RunInfo, output *tool.CallbackOutput) context.Context {
			if output == nil {
				return ctx
			}
			t.addNode(traceNode{
				EndedAt:   nowTraceTime(),
				Kind:      "tool_result",
				Name:      runName(info),
				Type:      runType(info),
				Component: runComponent(info),
				Output: traceToolOutput{
					Response:   output.Response,
					ToolOutput: output.ToolOutput,
				},
			})
			return ctx
		},
		OnError: func(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
			t.addNode(traceNode{
				EndedAt:   nowTraceTime(),
				Kind:      "tool_error",
				Name:      runName(info),
				Type:      runType(info),
				Component: runComponent(info),
				Error:     err.Error(),
			})
			return ctx
		},
	}
}

func (t *RequestTrace) composeHandler(kind string) callbacks.Handler {
	return callbacks.NewHandlerBuilder().
		OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
			t.addNode(traceNode{
				StartedAt: nowTraceTime(),
				Kind:      kind + "_start",
				Name:      runName(info),
				Type:      runType(info),
				Component: runComponent(info),
				Input:     compactTraceValue(input),
			})
			return ctx
		}).
		OnEndFn(func(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
			t.addNode(traceNode{
				EndedAt:   nowTraceTime(),
				Kind:      kind + "_end",
				Name:      runName(info),
				Type:      runType(info),
				Component: runComponent(info),
				Output:    compactTraceValue(output),
			})
			return ctx
		}).
		OnErrorFn(func(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
			t.addNode(traceNode{
				EndedAt:   nowTraceTime(),
				Kind:      kind + "_error",
				Name:      runName(info),
				Type:      runType(info),
				Component: runComponent(info),
				Error:     err.Error(),
			})
			return ctx
		}).
		Build()
}

func (t *RequestTrace) toolsNodeHandler() *callbacktemplate.ToolsNodeCallbackHandlers {
	return &callbacktemplate.ToolsNodeCallbackHandlers{
		OnStart: func(ctx context.Context, info *callbacks.RunInfo, input *schema.Message) context.Context {
			t.addNode(traceNode{
				StartedAt: nowTraceTime(),
				Kind:      "tools_node_start",
				Name:      runName(info),
				Type:      runType(info),
				Component: runComponent(info),
				Input:     toTraceMessage(input),
			})
			return ctx
		},
		OnEnd: func(ctx context.Context, info *callbacks.RunInfo, output []*schema.Message) context.Context {
			t.addNode(traceNode{
				EndedAt:   nowTraceTime(),
				Kind:      "tools_node_end",
				Name:      runName(info),
				Type:      runType(info),
				Component: runComponent(info),
				Output:    traceMessages{Messages: toTraceMessages(output)},
			})
			return ctx
		},
		OnError: func(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
			t.addNode(traceNode{
				EndedAt:   nowTraceTime(),
				Kind:      "tools_node_error",
				Name:      runName(info),
				Type:      runType(info),
				Component: runComponent(info),
				Error:     err.Error(),
			})
			return ctx
		},
	}
}

func traceDir(path string) string {
	if path == "" {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".json" || ext == ".jsonl" {
		return filepath.Dir(path)
	}
	return path
}

func (t *RequestTrace) consumeModelStream(info *callbacks.RunInfo, output *schema.StreamReader[*model.CallbackOutput]) {
	defer t.wg.Done()
	defer output.Close()
	var chunks []*schema.Message
	var usage any
	var model string
	for {
		chunk, err := output.Recv()
		if errors.Is(err, io.EOF) {
			msg, _ := schema.ConcatMessages(chunks)
			t.addNode(traceNode{
				EndedAt:   nowTraceTime(),
				Kind:      modelDecision(msg),
				Name:      runName(info),
				Type:      runType(info),
				Component: runComponent(info),
				Model:     model,
				Output: traceModelOutput{
					Message: toTraceMessage(msg),
					Usage:   usage,
				},
			})
			return
		}
		if err != nil {
			t.addNode(traceNode{
				EndedAt:   nowTraceTime(),
				Kind:      "model_stream_error",
				Name:      runName(info),
				Type:      runType(info),
				Component: runComponent(info),
				Error:     err.Error(),
			})
			return
		}
		if chunk == nil || chunk.Message == nil {
			continue
		}
		chunks = append(chunks, chunk.Message)
		if chunk.TokenUsage != nil {
			usage = chunk.TokenUsage
		}
		if chunk.Config != nil {
			model = chunk.Config.Model
		}
	}
}

func nowTraceTime() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func modelName(cfg *model.Config) string {
	if cfg == nil {
		return ""
	}
	return cfg.Model
}

func runName(info *callbacks.RunInfo) string {
	if info == nil {
		return ""
	}
	return info.Name
}

func runType(info *callbacks.RunInfo) string {
	if info == nil {
		return ""
	}
	return info.Type
}

func runComponent(info *callbacks.RunInfo) string {
	if info == nil {
		return ""
	}
	return string(info.Component)
}

func toTraceMessages(messages []*schema.Message) []traceMessage {
	result := make([]traceMessage, 0, len(messages))
	for _, msg := range messages {
		if item := toTraceMessage(msg); item != nil {
			result = append(result, *item)
		}
	}
	return result
}

func toTraceMessage(msg *schema.Message) *traceMessage {
	if msg == nil {
		return nil
	}
	return &traceMessage{
		Role:             string(msg.Role),
		Content:          msg.Content,
		ReasoningContent: msg.ReasoningContent,
		ToolCalls:        toTraceToolCalls(msg.ToolCalls),
		ToolCallID:       msg.ToolCallID,
	}
}

func toTraceToolCalls(toolCalls []schema.ToolCall) []traceToolCall {
	if len(toolCalls) == 0 {
		return nil
	}
	result := make([]traceToolCall, 0, len(toolCalls))
	for _, tc := range toolCalls {
		result = append(result, traceToolCall{
			ID:        tc.ID,
			Type:      tc.Type,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return result
}

func compactTraceValue(value any) any {
	if value == nil {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return value
	}
	const limit = 8000
	if len(data) <= limit {
		var decoded any
		if err := json.Unmarshal(data, &decoded); err == nil {
			return decoded
		}
		return string(data)
	}
	return map[string]any{
		"truncated": true,
		"size":      len(data),
		"preview":   string(data[:limit]),
	}
}

func modelDecision(msg *schema.Message) string {
	if msg != nil && len(msg.ToolCalls) > 0 {
		return "model_tool_call"
	}
	return "model_answer"
}
