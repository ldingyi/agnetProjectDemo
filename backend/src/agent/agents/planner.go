package agents

import (
	"context"
	"fmt"
	"sort"
	"strings"

	agenttools "agnet-project-demo/backend/src/agent/tools"
	"agnet-project-demo/backend/src/infrastructure"

	localfs "github.com/cloudwego/eino-ext/adk/backend/local"
	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const planExecutePlannerMaxSteps = 5

const planExecutePlannerSkillMaxRunes = 6000

func newPlanner(ctx context.Context, cfg infrastructure.Config) (adk.Agent, error) {
	plannerModel, err := newPlannerChatModel(ctx, cfg)
	if err != nil {
		return nil, err
	}

	skillBackend, err := newPlanExecuteSkillBackend(ctx)
	if err != nil {
		return nil, err
	}
	executorTools, err := agenttools.NewTools(ctx, cfg)
	if err != nil {
		return nil, err
	}
	skillMiddleware, err := skill.New(ctx, &skill.Config{
		Backend:    skillBackend,
		UseChinese: true,
	})
	if err != nil {
		return nil, fmt.Errorf("new planner skill middleware: %w", err)
	}
	executorTools = append(executorTools, skillMiddleware.AdditionalTools...)

	plannerCapabilityContext, err := buildPlanExecutePlannerCapabilityContext(ctx, skillBackend, executorTools)
	if err != nil {
		return nil, err
	}

	return planexecute.NewPlanner(ctx, &planexecute.PlannerConfig{
		ChatModelWithFormattedOutput: plannerModel,
		GenInputFn:                   newPlannerInputGen(plannerCapabilityContext),
	})
}

func newPlannerChatModel(ctx context.Context, cfg infrastructure.Config) (*deepseek.ChatModel, error) {
	plannerModel, err := deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
		APIKey:             cfg.DeepSeek.APIKey,
		BaseURL:            cfg.DeepSeek.BaseURL,
		Model:              cfg.DeepSeek.Model,
		Temperature:        0,
		ResponseFormatType: deepseek.ResponseFormatTypeJSONObject,
	})
	if err != nil {
		return nil, fmt.Errorf("new planner model: %w", err)
	}
	return plannerModel, nil
}

func newPlanExecuteSkillBackend(ctx context.Context) (skill.Backend, error) {
	fsBackend, err := localfs.NewBackend(ctx, &localfs.Config{})
	if err != nil {
		return nil, fmt.Errorf("new local skill filesystem backend: %w", err)
	}
	skillsDir, err := resolveAgentResourcePath("skills")
	if err != nil {
		return nil, err
	}
	skillBackend, err := skill.NewBackendFromFilesystem(ctx, &skill.BackendFromFilesystemConfig{
		Backend: fsBackend,
		BaseDir: skillsDir,
	})
	if err != nil {
		return nil, fmt.Errorf("new skill backend: %w", err)
	}
	if _, err := skillBackend.List(ctx); err != nil {
		return nil, fmt.Errorf("list agent skills from %s: %w", skillsDir, err)
	}
	return skillBackend, nil
}

func buildPlanExecutePlannerCapabilityContext(ctx context.Context, skillBackend skill.Backend, tools []tool.BaseTool) (string, error) {
	skills, err := loadPlanExecutePlannerSkills(ctx, skillBackend)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("## 可用 Skill 规划说明（Planner 只读，真正执行仍由 Executor 完成）\n")
	if len(skills) == 0 {
		b.WriteString("- 当前没有发现可用 Skill。\n")
	} else {
		for _, availableSkill := range skills {
			name := strings.TrimSpace(availableSkill.Name)
			if name == "" {
				continue
			}
			desc := promptSingleLine(availableSkill.Description)
			if desc == "" {
				desc = "无描述。"
			}
			_, _ = fmt.Fprintf(&b, "### Skill: %s\n", name)
			_, _ = fmt.Fprintf(&b, "Description: %s\n", desc)
			_, _ = fmt.Fprintf(&b, "Planning Guide:\n%s\n\n", trimPromptText(strings.TrimSpace(availableSkill.Content), planExecutePlannerSkillMaxRunes))
		}
	}

	b.WriteString("\n## Executor 可调用工具（Planner 只读，不代表 Planner 可以调用）\n")
	toolSummaries := make([]planExecuteToolSummary, 0, len(tools))
	for _, availableTool := range tools {
		if availableTool == nil {
			continue
		}
		info, err := availableTool.Info(ctx)
		if err != nil {
			return "", fmt.Errorf("read tool info for planning context: %w", err)
		}
		if info == nil || strings.TrimSpace(info.Name) == "" {
			continue
		}
		desc := promptSingleLine(info.Desc)
		if info.Name == "skill" {
			desc = "加载指定 Skill 的完整说明；当任务命中某个 Skill 时，Executor 应先调用该工具。"
		}
		if desc == "" {
			desc = "无描述。"
		}
		toolSummaries = append(toolSummaries, planExecuteToolSummary{
			Name: strings.TrimSpace(info.Name),
			Desc: trimPromptText(desc, 320),
		})
	}
	if len(toolSummaries) == 0 {
		b.WriteString("- 当前没有发现可用工具。\n")
	} else {
		sort.SliceStable(toolSummaries, func(i, j int) bool {
			return toolSummaries[i].Name < toolSummaries[j].Name
		})
		for _, summary := range toolSummaries {
			_, _ = fmt.Fprintf(&b, "- %s: %s\n", summary.Name, summary.Desc)
		}
	}

	return strings.TrimSpace(b.String()), nil
}

func loadPlanExecutePlannerSkills(ctx context.Context, skillBackend skill.Backend) ([]skill.Skill, error) {
	frontMatters, err := skillBackend.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list agent skills for planner: %w", err)
	}
	sort.SliceStable(frontMatters, func(i, j int) bool {
		return frontMatters[i].Name < frontMatters[j].Name
	})

	skills := make([]skill.Skill, 0, len(frontMatters))
	for _, frontMatter := range frontMatters {
		name := strings.TrimSpace(frontMatter.Name)
		if name == "" {
			continue
		}
		loadedSkill, err := skillBackend.Get(ctx, name)
		if err != nil {
			return nil, fmt.Errorf("load agent skill %s for planner: %w", name, err)
		}
		skills = append(skills, loadedSkill)
	}
	return skills, nil
}

type planExecuteToolSummary struct {
	Name string
	Desc string
}

func newPlannerInputGen(plannerCapabilityContext string) planexecute.GenPlannerModelInputFn {
	return func(ctx context.Context, userInput []adk.Message) ([]adk.Message, error) {
		return []adk.Message{
			schema.SystemMessage(planExecutePlannerPrompt(plannerCapabilityContext)),
			schema.UserMessage("请基于以下会话上下文识别用户目标，并只输出 JSON 对象：\n" + formatInputMessages(userInput)),
		}, nil
	}
}

func planExecutePlannerPrompt(plannerCapabilityContext string) string {
	return fmt.Sprintf(`你是 Planner，只负责理解用户意图并产出执行步骤。你没有任何业务工具，也不得声称已经查询或操作。

输出必须是 JSON object，格式固定为 {"steps":["..."]}。

你的输入：
- 用户会话上下文。
- 当前可用 Skill 的名称、描述和规划说明。
- Executor 可调用工具的名称和描述。

核心职责：
- 识别用户真实业务意图，而不是按关键词机械分类。
- 如果一句话包含多个意图，必须拆成多个可执行步骤，并按依赖关系排序。
- 如果用户目标、对象、参数或授权状态不明确，必须规划一个追问步骤，让 Executor 向用户索取最少必要信息。
- 你必须基于可用 Skill 规划说明、可用工具描述、公共业务说明和会话上下文做规划；不得假设系统拥有未列出的能力。

Skill 和工具边界：
- 你可以完整阅读下方 Skill 规划说明，用其中的触发条件、流程规则、依赖顺序和安全边界来规划步骤。
- 你可以阅读工具清单，用它判断 Executor 是否具备执行某个步骤的工具能力。
- 你不能调用任何 Skill 或工具，也不能编造工具返回结果。
- 当任务命中某个 Skill 时，计划中应明确写出“让 Executor 先加载 <skill_name> Skill 并按说明执行”，并按该 Skill 的流程规则拆分后续步骤。
- 真正的 Skill 加载、工具调用、业务查询、业务操作和面向用户追问都由 Executor 完成。

规划规则：
- 只写 Executor 可以执行的步骤，最多 %d 步。
- 第一步要能直接推进用户目标；如果必须先加载 Skill，第一步应是加载对应 Skill 并收集执行依据。
- 查询、判断、操作、回复应拆成清晰步骤；不要把多个有依赖的动作压成一个模糊步骤。
- 对发送消息、加入选品车、申请样品、购买样品等有副作用动作，只有当对象和内容明确时才规划执行，否则规划确认或追问。
- 对无法通过上下文或工具确定的信息，规划追问；对能由 Executor 查询获得的信息，不要提前追问。
- 不要输出解释、Markdown、代码块或额外字段。

可用能力清单：
%s`, planExecutePlannerMaxSteps, plannerCapabilityContext)
}
