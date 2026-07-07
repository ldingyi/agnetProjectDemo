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

// NewRequestTrace 为一次 Agent/Graph 执行创建本地追踪收集器。
func NewRequestTrace(cfg Config) *RequestTrace {
	return &RequestTrace{
		id:        uuid.NewString(),
		startedAt: time.Now().UTC(),
		dir:       traceDir(cfg.Trace.Path),
		nodes:     make([]traceNode, 0),
	}
}

// NewEinoTraceRunOptions 把追踪 handler 挂到 ADK Runner 上，记录模型和工具事件。
func NewEinoTraceRunOptions(trace *RequestTrace) []adk.AgentRunOption {
	if trace == nil {
		return nil
	}
	// Runner 只接受运行选项，这里把 RequestTrace 包装成 ADK callback 选项传入。
	return []adk.AgentRunOption{
		adk.WithCallbacks(trace.Handler()),
	}
}

// NewEinoTraceHandler 创建一个独立的 Eino callback handler，用于不需要显式 Finish 的场景。
func NewEinoTraceHandler(cfg Config) callbacks.Handler {
	return NewRequestTrace(cfg).Handler()
}

// Handler 构造覆盖模型、工具和 Graph 节点的统一追踪回调。
func (t *RequestTrace) Handler() callbacks.Handler {
	// 同一个 handler 覆盖 Graph、Lambda、ChatModel、Tool 和 ToolsNode，输出时按事件顺序合并。
	return callbacktemplate.NewHandlerHelper().
		Graph(t.composeHandler("graph")).
		Lambda(t.composeHandler("lambda")).
		ChatModel(t.modelHandler()).
		Tool(t.toolHandler()).
		ToolsNode(t.toolsNodeHandler()).
		Handler()
}

// Finish 等待异步流式消费结束，并把本次请求的追踪节点写入本地 JSON 文件。
func (t *RequestTrace) Finish() {
	if t == nil || t.dir == "" {
		return
	}
	// 流式模型输出在 goroutine 中消费，落盘前必须等消费完成。
	t.wg.Wait()

	t.mu.Lock()
	nodes := make([]traceNode, len(t.nodes))
	copy(nodes, t.nodes)
	t.mu.Unlock()

	// callback 可能来自不同 goroutine，落盘前按采集时分配的序号恢复事件顺序。
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

// addNode 线程安全地追加一个追踪节点，并给节点分配递增序号。
func (t *RequestTrace) addNode(node traceNode) {
	if t == nil {
		return
	}
	t.mu.Lock()
	node.Index = len(t.nodes) + 1
	t.nodes = append(t.nodes, node)
	t.mu.Unlock()
}

// modelHandler 创建模型回调，记录模型输入、输出、流式输出和错误。
func (t *RequestTrace) modelHandler() *callbacktemplate.ModelCallbackHandler {
	return &callbacktemplate.ModelCallbackHandler{
		OnStart: func(ctx context.Context, info *callbacks.RunInfo, input *model.CallbackInput) context.Context {
			if input == nil {
				return ctx
			}
			// OnStart 记录完整消息上下文，便于回放模型看到的系统消息、用户消息和工具结果。
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
			// 非流式输出可以立即记录，同时根据是否有工具调用标记模型决策类型。
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
			// callback 不阻塞主链路，流式输出在后台拼接为一条完整模型结果。
			t.wg.Add(1)
			go t.consumeModelStream(info, output)
			return ctx
		},
		OnError: func(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
			// 错误节点只记录元信息和错误文本，避免异常路径再触发复杂序列化。
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

// toolHandler 创建单个工具调用的追踪回调，记录入参、出参和执行错误。
func (t *RequestTrace) toolHandler() *callbacktemplate.ToolCallbackHandler {
	return &callbacktemplate.ToolCallbackHandler{
		OnStart: func(ctx context.Context, info *callbacks.RunInfo, input *tool.CallbackInput) context.Context {
			if input == nil {
				return ctx
			}
			// 工具入参通常是模型生成的 JSON，原样记录便于定位参数拼装问题。
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
			// 工具结果同时保留文本响应和结构化输出，兼顾前端展示和排障查看。
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
			// 工具失败通常会影响 ReAct 下一步，把错误单独作为节点便于在 trace 中定位。
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

// composeHandler 为 Graph/Lambda 等通用组件创建 start/end/error 追踪回调。
func (t *RequestTrace) composeHandler(kind string) callbacks.Handler {
	return callbacks.NewHandlerBuilder().
		OnStartFn(func(ctx context.Context, info *callbacks.RunInfo, input callbacks.CallbackInput) context.Context {
			// 通用组件的输入输出类型不固定，先压缩后记录，避免 trace 文件过大。
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
			// end 节点和 start 节点分开记录，可以直接看到每个组件的边界和产物。
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
			// 通用错误只保留错误文本，保证 callback 本身不影响主业务链路。
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

// toolsNodeHandler 创建 ToolsNode 级别的追踪回调，记录批量工具节点的输入输出。
func (t *RequestTrace) toolsNodeHandler() *callbacktemplate.ToolsNodeCallbackHandlers {
	return &callbacktemplate.ToolsNodeCallbackHandlers{
		OnStart: func(ctx context.Context, info *callbacks.RunInfo, input *schema.Message) context.Context {
			// ToolsNode 的输入通常是包含 tool_calls 的 assistant 消息，转换后更容易阅读。
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
			// ToolsNode 可能产出多条 tool 消息，统一包装成 messages 数组保持 JSON 结构稳定。
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
			// 批量工具节点失败时记录节点级错误，单个工具错误仍由 toolHandler 补充。
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

// traceDir 把兼容旧配置的 trace 文件路径转换为实际写入目录。
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

// consumeModelStream 在后台消费模型流式输出，并合并成一条模型结果节点。
func (t *RequestTrace) consumeModelStream(info *callbacks.RunInfo, output *schema.StreamReader[*model.CallbackOutput]) {
	defer t.wg.Done()
	defer output.Close()
	var chunks []*schema.Message
	var usage any
	var model string
	for {
		chunk, err := output.Recv()
		if errors.Is(err, io.EOF) {
			// trace 里记录合并后的模型消息，避免把每个 token/chunk 都落成一条节点。
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
		// 持续收集消息 chunk，同时保留最后一次非空 usage/model 信息作为完整输出元数据。
		chunks = append(chunks, chunk.Message)
		if chunk.TokenUsage != nil {
			usage = chunk.TokenUsage
		}
		if chunk.Config != nil {
			model = chunk.Config.Model
		}
	}
}

// nowTraceTime 返回 trace 文件中统一使用的 UTC 纳秒时间字符串。
func nowTraceTime() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// modelName 安全读取模型配置中的模型名。
func modelName(cfg *model.Config) string {
	if cfg == nil {
		return ""
	}
	return cfg.Model
}

// runName 安全读取 callback 运行信息中的名称。
func runName(info *callbacks.RunInfo) string {
	if info == nil {
		return ""
	}
	return info.Name
}

// runType 安全读取 callback 运行信息中的类型。
func runType(info *callbacks.RunInfo) string {
	if info == nil {
		return ""
	}
	return info.Type
}

// runComponent 安全读取 callback 运行信息中的组件类型。
func runComponent(info *callbacks.RunInfo) string {
	if info == nil {
		return ""
	}
	return string(info.Component)
}

// toTraceMessages 把 Eino 消息列表转换为 trace 文件中的轻量消息结构。
func toTraceMessages(messages []*schema.Message) []traceMessage {
	result := make([]traceMessage, 0, len(messages))
	for _, msg := range messages {
		// 跳过 nil 消息，避免 trace 中出现空对象干扰排查。
		if item := toTraceMessage(msg); item != nil {
			result = append(result, *item)
		}
	}
	return result
}

// toTraceMessage 把单条 Eino 消息转换为可 JSON 序列化的 trace 消息。
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

// toTraceToolCalls 提取模型消息中的工具调用信息。
func toTraceToolCalls(toolCalls []schema.ToolCall) []traceToolCall {
	if len(toolCalls) == 0 {
		return nil
	}
	result := make([]traceToolCall, 0, len(toolCalls))
	for _, tc := range toolCalls {
		// 只保留工具调用的稳定字段，避免把完整 SDK 对象写入 trace。
		result = append(result, traceToolCall{
			ID:        tc.ID,
			Type:      tc.Type,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return result
}

// compactTraceValue 把任意 callback 输入输出压缩成适合落盘的 JSON 值。
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
			// 成功解码后返回结构化值，方便直接用 JSON 查看器展开。
			return decoded
		}
		return string(data)
	}
	// trace 只用于本地排障，超大输入输出保留预览，避免把文件写到不可控大小。
	return map[string]any{
		"truncated": true,
		"size":      len(data),
		"preview":   string(data[:limit]),
	}
}

// modelDecision 根据模型消息是否包含工具调用，标记为回答或工具调用决策。
func modelDecision(msg *schema.Message) string {
	if msg != nil && len(msg.ToolCalls) > 0 {
		return "model_tool_call"
	}
	return "model_answer"
}
