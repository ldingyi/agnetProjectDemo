package agents

import (
	"context"
	"fmt"
	"strings"

	agenttools "agnet-project-demo/backend/src/agent/tools"
	"agnet-project-demo/backend/src/infrastructure"

	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const planExecuteStepMaxTurns = 8

func newExecutor(ctx context.Context, cfg infrastructure.Config) (adk.Agent, error) {
	executorModel, err := newExecutorChatModel(ctx, cfg)
	if err != nil {
		return nil, err
	}

	executorTools, skillInstruction, err := newExecutorTools(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return planexecute.NewExecutor(ctx, &planexecute.ExecutorConfig{
		Model: executorModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: executorTools,
			},
		},
		MaxIterations: planExecuteStepMaxTurns,
		GenInputFn:    newExecutorInputGen(skillInstruction),
	})
}

func newExecutorChatModel(ctx context.Context, cfg infrastructure.Config) (*deepseek.ChatModel, error) {
	executorModel, err := deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
		APIKey:  cfg.DeepSeek.APIKey,
		BaseURL: cfg.DeepSeek.BaseURL,
		Model:   cfg.DeepSeek.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("new executor model: %w", err)
	}
	return executorModel, nil
}

func newExecutorTools(ctx context.Context, cfg infrastructure.Config) ([]tool.BaseTool, string, error) {
	businessTools, err := agenttools.NewTools(ctx, cfg)
	if err != nil {
		return nil, "", err
	}

	skillBackend, err := newPlanExecuteSkillBackend(ctx)
	if err != nil {
		return nil, "", err
	}
	skillMiddleware, err := skill.New(ctx, &skill.Config{
		Backend:    skillBackend,
		UseChinese: true,
	})
	if err != nil {
		return nil, "", fmt.Errorf("new skill middleware: %w", err)
	}

	executorTools := append(businessTools, skillMiddleware.AdditionalTools...)
	return executorTools, skillMiddleware.AdditionalInstruction, nil
}

func newExecutorInputGen(skillInstruction string) planexecute.GenModelInputFn {
	return func(ctx context.Context, in *planexecute.ExecutionContext) ([]adk.Message, error) {
		planContent, err := in.Plan.MarshalJSON()
		if err != nil {
			return nil, err
		}

		return []adk.Message{
			schema.SystemMessage(planExecuteExecutorPrompt(skillInstruction)),
			schema.UserMessage(fmt.Sprintf(`## 用户目标和上下文
%s

## 当前计划
%s

## 已完成步骤及结果
%s

## 本轮只执行的步骤
%s`, formatInputMessages(in.UserInput), string(planContent), formatExecutedSteps(in.ExecutedSteps), in.Plan.FirstStep())),
		}, nil
	}
}

func planExecuteExecutorPrompt(skillInstruction string) string {
	var b strings.Builder
	b.WriteString("你是 Executor，只负责执行 Planner 给出的当前步骤。你拥有当前提供的业务工具和 Skill 工具，真正的 Skill 加载、工具调用、业务查询、业务操作和必要追问都由你完成。\n\n")
	b.WriteString("你的输入：\n")
	b.WriteString("- 用户目标和上下文。\n")
	b.WriteString("- Planner 生成的当前计划。\n")
	b.WriteString("- 已完成步骤和执行结果。\n")
	b.WriteString("- 本轮只执行的单个步骤。\n")
	b.WriteString("- Skill 使用指令。\n\n")
	b.WriteString("职责边界：\n")
	b.WriteString("- 严格执行“本轮只执行的步骤”，不要重新规划整个任务，不要提前执行后续步骤。\n")
	b.WriteString("- 如果 Planner 的当前步骤要求加载 Skill，必须先调用 `skill` 工具读取完整 Skill 说明，再按 Skill 说明和工具结果行动。\n")
	b.WriteString("- 如果当前步骤未显式提到 Skill，但用户目标明显命中某个 Skill，你也必须先调用 `skill` 工具加载相关 Skill，再继续执行当前步骤。\n")
	b.WriteString("- 你可以调用工具获取事实或执行业务动作；所有最终判断必须来自上下文、Skill 说明和工具返回结果。\n\n")
	b.WriteString("执行规则：\n")
	b.WriteString("- 严格围绕“本轮只执行的步骤”行动；需要工具时调用工具，不需要工具时直接产出该步骤结果。\n")
	b.WriteString("- 如果当前步骤是追问用户，直接给出最少必要问题，不要调用无关工具，不要一次性询问与当前任务无关的信息。\n")
	b.WriteString("- 对有副作用的操作，目标、对象、内容、商品列表或会话 ID 不明确时先要求确认；确认前不要发送消息、加车、申样或购买。\n")
	b.WriteString("- 如果工具可以查询到缺失信息，先查询再判断；只有上下文和工具都无法确定时才向用户追问。\n")
	b.WriteString("- 如果工具返回多个候选对象且无法可靠 disambiguate，向用户说明候选项并要求选择。\n")
	b.WriteString("- 工具调用结果是唯一事实来源，不要编造业务状态、商品、会话、样品或履约结果。\n")
	b.WriteString("- 工具返回业务失败时，按业务原因解释；工具异常时，用业务语言说明暂时无法完成，不暴露内部实现。\n")
	b.WriteString("- 每轮只输出当前步骤的执行结果，最终面向用户的整合回复由 Replanner 负责。\n\n")
	b.WriteString("业务原则：\n")
	b.WriteString("- 你服务达人和商家之间的撮合、沟通、选品、样品和履约相关场景。\n")
	b.WriteString("- 所有查询和操作都必须基于上下文、Skill 说明和工具返回结果，不得编造系统能力、业务状态、商品信息、IM 内容或执行结果。\n")
	b.WriteString("- `user_id` 必须使用系统消息中的当前登录用户 ID，不要向用户索要或猜测。\n")
	b.WriteString("- 对发送 IM、加入选品车、申请样品、购买样品等会改变业务状态的操作，目标、对象和操作内容明确后才能执行。\n")
	b.WriteString("- 回复用户时使用自然、简洁、可信的业务语言，不暴露内部工具名、参数名、调用过程、RPC、HTTP、MCP、trace、日志或技术错误细节。\n")
	if strings.TrimSpace(skillInstruction) != "" {
		b.WriteString("\n\nSkill 使用指令：\n")
		b.WriteString(skillInstruction)
	}
	return b.String()
}
