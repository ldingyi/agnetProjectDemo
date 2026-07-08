package agents

import (
	"context"
	"fmt"

	"agnet-project-demo/backend/src/infrastructure"

	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
	"github.com/cloudwego/eino/schema"
)

func newReplanner(ctx context.Context, cfg infrastructure.Config) (adk.Agent, error) {
	replannerModel, err := newReplannerChatModel(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return planexecute.NewReplanner(ctx, &planexecute.ReplannerConfig{
		ChatModel:  replannerModel,
		GenInputFn: genPlanExecuteReplannerInput,
	})
}

func newReplannerChatModel(ctx context.Context, cfg infrastructure.Config) (*deepseek.ChatModel, error) {
	replannerModel, err := deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
		APIKey:      cfg.DeepSeek.APIKey,
		BaseURL:     cfg.DeepSeek.BaseURL,
		Model:       cfg.DeepSeek.Model,
		Temperature: 0,
	})
	if err != nil {
		return nil, fmt.Errorf("new replanner model: %w", err)
	}
	return replannerModel, nil
}

func genPlanExecuteReplannerInput(ctx context.Context, in *planexecute.ExecutionContext) ([]adk.Message, error) {
	planContent, err := in.Plan.MarshalJSON()
	if err != nil {
		return nil, err
	}

	return []adk.Message{
		schema.SystemMessage(planExecuteReplannerPrompt()),
		schema.UserMessage(fmt.Sprintf(`## 用户原始目标和上下文
%s

## 当前计划
%s

## 已完成步骤及结果
%s`, formatInputMessages(in.UserInput), string(planContent), formatExecutedSteps(in.ExecutedSteps))),
	}, nil
}

func planExecuteReplannerPrompt() string {
	return `你是 Replanner，负责判断任务是否已经足够回复用户。

你的输入：
- 用户原始目标和上下文。
- 当前计划。
- 已完成步骤及结果。

你必须且只能调用一个工具：
- 如果用户目标已经完成，或下一步需要用户补充信息，调用 respond，response 写给用户看的最终回复或追问。
- 如果仍需继续执行，调用 plan，steps 只包含剩余步骤，不要包含已完成步骤。

判断规则：
- 以用户原始目标为准，不只看 Planner 的字面步骤。
- 如果多个意图都已完成，才能最终回复。
- 如果某个意图缺少必要信息，并且 Executor 已经无法继续查询，应向用户追问。
- 如果已有执行结果不足以支撑最终结论，应继续规划剩余步骤。

回复规则：
- 面向业务用户，不暴露 Planner、Executor、工具名、RPC、HTTP、MCP、trace、日志、错误码或内部实现。
- 查询类任务说明关键结果和依据；操作类任务说明对象、结果、失败原因和后续建议。
- 如果当前需要用户补充信息，问题要短而明确，只问完成任务所需的最少信息。
- 不要输出 JSON 到用户，JSON 只用于工具参数。`
}
