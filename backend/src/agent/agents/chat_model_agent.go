package agents

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	baseagent "agnet-project-demo/backend/src/agent"
	"agnet-project-demo/backend/src/infrastructure"

	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type ChatModelAgent struct {
	runner *adk.Runner
	cfg    infrastructure.Config
}

func NewChatModelAgent(ctx context.Context, cfg infrastructure.Config) (*ChatModelAgent, error) {
	if cfg.DeepSeek.APIKey == "" {
		return nil, errors.New("DEEPSEEK_API_KEY is required")
	}

	chatModel, err := deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
		APIKey:  cfg.DeepSeek.APIKey,
		BaseURL: cfg.DeepSeek.BaseURL,
		Model:   cfg.DeepSeek.Model,
	})
	if err != nil {
		return nil, err
	}
	mcpTools, err := infrastructure.NewThirdMCPTools(ctx, cfg)
	if err != nil {
		return nil, err
	}
	instruction, err := os.ReadFile(filepath.Join("src", "agent", "agents", "agent.md"))
	if err != nil {
		return nil, err
	}
	a, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "ChatAgent",
		Description: "A minimal DeepSeek-backed chat agents.",
		Instruction: string(instruction),
		Model:       chatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: mcpTools,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	return &ChatModelAgent{
		runner: adk.NewRunner(ctx, adk.RunnerConfig{
			Agent:           a,
			EnableStreaming: true,
		}),
		cfg: cfg,
	}, nil
}

func (a *ChatModelAgent) Generate(ctx context.Context, messages []*schema.Message) (string, error) {
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

func (a *ChatModelAgent) Stream(ctx context.Context, messages []*schema.Message, onDelta func(delta string) error) error {
	trace := infrastructure.NewRequestTrace(a.cfg)
	defer trace.Finish()

	return baseagent.ConsumeAgentEvents(a.runner.Run(ctx, messages, infrastructure.NewEinoTraceRunOptions(trace)...), onDelta)
}
