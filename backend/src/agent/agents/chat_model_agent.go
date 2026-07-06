package agents

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	baseagent "agnet-project-demo/backend/src/agent"
	thirdtools "agnet-project-demo/backend/src/agent/tools/third"
	"agnet-project-demo/backend/src/infrastructure"

	localfs "github.com/cloudwego/eino-ext/adk/backend/local"
	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino/adk"
	adkskill "github.com/cloudwego/eino/adk/middlewares/skill"
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
	mcpTools, err := thirdtools.NewTools(ctx, cfg)
	if err != nil {
		return nil, err
	}
	instruction, err := readAgentInstruction()
	if err != nil {
		return nil, err
	}
	skillHandler, err := newSkillMiddleware(ctx)
	if err != nil {
		return nil, err
	}
	a, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "ChatAgent",
		Description: "A minimal DeepSeek-backed chat agents.",
		Instruction: instruction,
		Model:       chatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: mcpTools,
			},
		},
		Handlers: []adk.ChatModelAgentMiddleware{skillHandler},
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

func readAgentInstruction() (string, error) {
	path, err := resolveAgentResourcePath(filepath.Join("agents", "agent.md"))
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read agent instruction %s: %w", path, err)
	}
	return string(data), nil
}

func newSkillMiddleware(ctx context.Context) (adk.ChatModelAgentMiddleware, error) {
	fsBackend, err := localfs.NewBackend(ctx, &localfs.Config{})
	if err != nil {
		return nil, fmt.Errorf("new local skill filesystem backend: %w", err)
	}

	skillsDir, err := resolveAgentResourcePath("skills")
	if err != nil {
		return nil, err
	}
	skillBackend, err := adkskill.NewBackendFromFilesystem(ctx, &adkskill.BackendFromFilesystemConfig{
		Backend: fsBackend,
		BaseDir: skillsDir,
	})
	if err != nil {
		return nil, fmt.Errorf("new skill backend: %w", err)
	}
	if _, err := skillBackend.List(ctx); err != nil {
		return nil, fmt.Errorf("list agent skills from %s: %w", skillsDir, err)
	}

	handler, err := adkskill.NewMiddleware(ctx, &adkskill.Config{
		Backend:    skillBackend,
		UseChinese: true,
	})
	if err != nil {
		return nil, fmt.Errorf("new skill middleware: %w", err)
	}
	return handler, nil
}

func resolveAgentResourcePath(resource string) (string, error) {
	candidates := []string{
		filepath.Join("src", "agent", resource),
		filepath.Join("backend", "src", "agent", resource),
	}

	if _, file, _, ok := runtime.Caller(0); ok {
		agentDir := filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
		candidates = append(candidates, filepath.Join(agentDir, resource))
	}

	for _, candidate := range candidates {
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err == nil {
			return abs, nil
		}
	}

	return "", fmt.Errorf("agent resource not found: %s", resource)
}
