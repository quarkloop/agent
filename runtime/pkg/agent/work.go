package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/runtime/pkg/loop"
)

func (a *Agent) workStepTicker(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			select {
			case <-a.Plan.NextStep():
				a.loop.Send(NewWorkStepMsg())
			default:
			}
		}
	}
}

func (a *Agent) handleWorkStep(ctx context.Context, _ loop.Message) error {
	client := a.Models.GetDefault()
	if client == nil {
		return nil
	}
	infer := func(ctx context.Context, messages []plugin.Message, tools []plugin.ToolSchema, onTool plugin.ToolHandler, onMessage func(string, any)) (string, error) {
		return client.InferWithPreparedContextAndPolicy(ctx, messages, tools, onTool, onMessage, a.contextPreparer(client.ContextWindow, ""), a.finalGuard(), nil, nil, nil, nil)
	}
	if err := a.Plan.ExecuteStep(ctx, infer, nil, a.defaultTools(), a.executeTool); err != nil {
		slog.Error("work step error", "error", err)
		return err
	}
	return nil
}

func (a *Agent) processWork(ctx context.Context, _ string, task string) (string, error) {
	client := a.Models.GetDefault()
	if client == nil {
		return "", fmt.Errorf("no LLM client configured")
	}
	messages := []plugin.Message{{Role: "user", Content: task}}
	return client.InferWithPreparedContextAndPolicy(ctx, messages, a.defaultTools(), a.executeTool, nil, a.contextPreparer(client.ContextWindow, ""), nil, nil, nil, nil, nil)
}
