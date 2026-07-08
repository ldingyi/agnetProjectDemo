package agents

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
)

func TestConsumePlanExecuteEventsOnlyEmitsFinalResponse(t *testing.T) {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	gen.Send(namedMessageEvent("planner", `{"steps":["查询会话","回复用户"]}`))
	gen.Send(namedMessageEvent("executor", "已查询到会话信息"))
	gen.Send(namedMessageEvent("replanner", `{"response":"这是给用户看的最终回复"}`))
	gen.Close()

	var got string
	if err := consumePlanExecuteEvents(iter, func(delta string) error {
		got += delta
		return nil
	}); err != nil {
		t.Fatalf("consumePlanExecuteEvents returned error: %v", err)
	}

	if got != "这是给用户看的最终回复" {
		t.Fatalf("got %q", got)
	}
}

func TestConsumePlanExecuteEventsUsesReplannerTextFallback(t *testing.T) {
	iter, gen := adk.NewAsyncIteratorPair[*adk.AgentEvent]()
	gen.Send(namedMessageEvent("planner", `{"steps":["询问缺失信息"]}`))
	gen.Send(namedMessageEvent("replanner", "请补充你要查看的会话 ID。"))
	gen.Close()

	var got string
	if err := consumePlanExecuteEvents(iter, func(delta string) error {
		got += delta
		return nil
	}); err != nil {
		t.Fatalf("consumePlanExecuteEvents returned error: %v", err)
	}

	if got != "请补充你要查看的会话 ID。" {
		t.Fatalf("got %q", got)
	}
}

func namedMessageEvent(agentName string, content string) *adk.AgentEvent {
	event := adk.EventFromMessage(schema.AssistantMessage(content, nil), nil, schema.Assistant, "")
	event.AgentName = agentName
	return event
}

func TestPlanExecuteAgentImplementsChatAgent(t *testing.T) {
	var _ ChatAgent = (*PlanExecuteAgent)(nil)
}

func TestPlanExecutePromptsBuild(t *testing.T) {
	capabilityContext := "## 可用 Skill 规划说明（Planner 只读，真正执行仍由 Executor 完成）\n### Skill: im\nDescription: IM 相关任务。\nPlanning Guide:\n先读取会话消息，再提取邀约商品，最后按用户授权发送消息。\n\n## Executor 可调用工具（Planner 只读）\n- skill: 加载指定 Skill。"
	msgs, err := newPlannerInputGen(capabilityContext)(context.Background(), []adk.Message{schema.UserMessage("查一下 IM")})
	if err != nil {
		t.Fatalf("newPlannerInputGen returned error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages", len(msgs))
	}
	systemPrompt := msgs[0].Content
	for _, want := range []string{
		"你可以完整阅读下方 Skill 规划说明",
		"你不能调用任何 Skill 或工具",
		"并按该 Skill 的流程规则拆分后续步骤",
		"## 可用 Skill 规划说明（Planner 只读，真正执行仍由 Executor 完成）",
		"### Skill: im",
		"先读取会话消息，再提取邀约商品，最后按用户授权发送消息。",
	} {
		if !strings.Contains(systemPrompt, want) {
			t.Fatalf("planner prompt missing %q:\n%s", want, systemPrompt)
		}
	}

	executorPrompt := planExecuteExecutorPrompt("Skill 规则")
	for _, want := range []string{
		"真正的 Skill 加载、工具调用、业务查询、业务操作和必要追问都由你完成",
		"必须先调用 `skill` 工具读取完整 Skill 说明",
		"如果工具可以查询到缺失信息，先查询再判断",
	} {
		if !strings.Contains(executorPrompt, want) {
			t.Fatalf("executor prompt missing %q:\n%s", want, executorPrompt)
		}
	}
}
