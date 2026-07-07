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
	agenttools "agnet-project-demo/backend/src/agent/tools"
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

// NewChatModelAgent 创建只包含默认业务工具的聊天 Agent。
func NewChatModelAgent(ctx context.Context, cfg infrastructure.Config) (*ChatModelAgent, error) {
	if cfg.DeepSeek.APIKey == "" {
		return nil, errors.New("DEEPSEEK_API_KEY is required")
	}

	// ChatModelAgent 只负责语义理解和 ReAct 循环，外层 application 仍然掌握会话和用户身份。
	chatModel, err := deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
		APIKey:  cfg.DeepSeek.APIKey,
		BaseURL: cfg.DeepSeek.BaseURL,
		Model:   cfg.DeepSeek.Model,
	})
	if err != nil {
		return nil, err
	}
	tools, err := agenttools.NewTools(ctx, cfg)
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
	// Skill middleware 让模型先看到轻量技能索引，命中业务场景时再加载完整 SKILL.md。
	a, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        "ChatAgent",
		Description: "A minimal DeepSeek-backed chat agents.",
		Instruction: instruction,
		Model:       chatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: tools,
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

// Generate 执行一次非流式生成，并把流式增量拼接成完整文本返回。
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

// Stream 执行 Agent Runner，并把最终可展示内容以 delta 回调形式输出。
func (a *ChatModelAgent) Stream(ctx context.Context, messages []*schema.Message, onDelta func(delta string) error) error {
	trace := infrastructure.NewRequestTrace(a.cfg)
	defer trace.Finish()

	// Runner 输出的是 ADK 事件流，ConsumeAgentEvents 会过滤工具调用等内部消息。
	return baseagent.ConsumeAgentEvents(a.runner.Run(ctx, messages, infrastructure.NewEinoTraceRunOptions(trace)...), onDelta)
}

// readAgentInstruction 读取 Agent 的系统指令文件。
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

// newSkillMiddleware 创建本地技能中间件，让 Agent 按需读取技能说明。
func newSkillMiddleware(ctx context.Context) (adk.ChatModelAgentMiddleware, error) {
	fsBackend, err := localfs.NewBackend(ctx, &localfs.Config{})
	if err != nil {
		return nil, fmt.Errorf("new local skill filesystem backend: %w", err)
	}

	// 技能目录随代码一起发布，resolveAgentResourcePath 兼容从仓库根或 backend 目录启动。
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
		// 初始化时先列出技能目录，提前暴露路径错误或打包遗漏问题。
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

// resolveAgentResourcePath 解析随 Agent 代码发布的资源路径，兼容不同启动目录。
func resolveAgentResourcePath(resource string) (string, error) {
	candidates := []string{
		filepath.Join("src", "agent", resource),
		filepath.Join("backend", "src", "agent", resource),
	}

	if _, file, _, ok := runtime.Caller(0); ok {
		// 运行时源码路径作为兜底，支持从测试或 IDE 直接启动。
		agentDir := filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
		candidates = append(candidates, filepath.Join(agentDir, resource))
	}

	for _, candidate := range candidates {
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		// 返回第一个存在的绝对路径，调用方无需关心当前工作目录。
		if _, err := os.Stat(abs); err == nil {
			return abs, nil
		}
	}

	return "", fmt.Errorf("agent resource not found: %s", resource)
}
