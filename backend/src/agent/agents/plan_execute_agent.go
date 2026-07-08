package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	baseagent "agnet-project-demo/backend/src/agent"
	"agnet-project-demo/backend/src/infrastructure"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
	"github.com/cloudwego/eino/schema"
)

const (
	planExecuteMaxIterations = 8
)

type PlanExecuteAgent struct {
	runner *adk.Runner
	cfg    infrastructure.Config
}

// ChatAgent is the application-facing boundary shared by supported agent architectures.
type ChatAgent interface {
	Generate(ctx context.Context, messages []*schema.Message) (string, error)
	Stream(ctx context.Context, messages []*schema.Message, onDelta func(delta string) error) error
}

// NewPlanExecuteAgent creates an ADK prebuilt Planner-Executor-Replanner agent.
func NewPlanExecuteAgent(ctx context.Context, cfg infrastructure.Config) (*PlanExecuteAgent, error) {
	if cfg.DeepSeek.APIKey == "" {
		return nil, errors.New("DEEPSEEK_API_KEY is required")
	}

	planner, err := newPlanner(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("new planner: %w", err)
	}
	executor, err := newExecutor(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("new executor: %w", err)
	}
	replanner, err := newReplanner(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("new replanner: %w", err)
	}

	entry, err := planexecute.New(ctx, &planexecute.Config{
		Planner:       planner,
		Executor:      executor,
		Replanner:     replanner,
		MaxIterations: planExecuteMaxIterations,
	})
	if err != nil {
		return nil, fmt.Errorf("new plan execute agent: %w", err)
	}

	return &PlanExecuteAgent{
		runner: adk.NewRunner(ctx, adk.RunnerConfig{
			Agent:           entry,
			EnableStreaming: true,
		}),
		cfg: cfg,
	}, nil
}

func (a *PlanExecuteAgent) Generate(ctx context.Context, messages []*schema.Message) (string, error) {
	var b strings.Builder
	err := a.Stream(ctx, messages, func(delta string) error {
		b.WriteString(delta)
		return nil
	})
	if err != nil {
		return "", err
	}
	return b.String(), nil
}

func (a *PlanExecuteAgent) Stream(ctx context.Context, messages []*schema.Message, onDelta func(delta string) error) error {
	trace := infrastructure.NewRequestTrace(a.cfg)
	defer trace.Finish()

	return consumePlanExecuteEvents(a.runner.Run(ctx, messages, infrastructure.NewEinoTraceRunOptions(trace)...), onDelta)
}

type planExecuteFinalResponse struct {
	Response string `json:"response"`
}

func consumePlanExecuteEvents(iter *adk.AsyncIterator[*adk.AgentEvent], onDelta baseagent.DeltaHandler) error {
	var final string
	var fallback string

	for {
		event, ok := iter.Next()
		if !ok {
			break
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

		msg, err := planExecuteEventMessage(event.Output.MessageOutput)
		if err != nil {
			return err
		}
		if !shouldExposePlanExecuteMessage(msg) {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}

		if response, ok := parsePlanExecuteFinalResponse(content); ok {
			final = response
			continue
		}
		if event.AgentName == "replanner" && !looksLikePlanJSON(content) {
			fallback = content
		}
	}

	output := strings.TrimSpace(final)
	if output == "" {
		output = strings.TrimSpace(fallback)
	}
	if output == "" {
		return nil
	}
	return onDelta(output)
}

func planExecuteEventMessage(output *adk.MessageVariant) (*schema.Message, error) {
	if output.IsStreaming && output.MessageStream != nil {
		return baseagent.ConsumeMessageStream(output.MessageStream)
	}
	return output.Message, nil
}

func shouldExposePlanExecuteMessage(msg *schema.Message) bool {
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

func parsePlanExecuteFinalResponse(content string) (string, bool) {
	var response planExecuteFinalResponse
	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return "", false
	}
	responseText := strings.TrimSpace(response.Response)
	return responseText, responseText != ""
}

func looksLikePlanJSON(content string) bool {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return false
	}
	_, hasSteps := raw["steps"]
	return hasSteps
}

func formatInputMessages(messages []adk.Message) string {
	var b strings.Builder
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(string(msg.Role))
		b.WriteString(": ")
		b.WriteString(content)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func formatExecutedSteps(steps []planexecute.ExecutedStep) string {
	if len(steps) == 0 {
		return "暂无。"
	}
	var b strings.Builder
	for i, step := range steps {
		_, _ = fmt.Fprintf(&b, "%d. Step: %s\nResult: %s\n\n", i+1, step.Step, step.Result)
	}
	return strings.TrimSpace(b.String())
}

func promptSingleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func trimPromptText(s string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(s))
	if maxRunes <= 0 || len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "..."
}
