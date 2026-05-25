package agent

import (
	"context"
	"fmt"

	"github.com/quarkloop/pkg/boundary"
	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/runtime/pkg/execution"
	"github.com/quarkloop/runtime/pkg/loop"
	"github.com/quarkloop/runtime/pkg/modelservice"
	"github.com/quarkloop/runtime/pkg/toolpolicy"
)

// handleToolCall executes an explicitly posted tool call through the same
// policy path used by LLM-requested tool calls.
func (a *Agent) handleToolCall(ctx context.Context, msg loop.Message) error {
	toolMsg := msg.(ToolCallMsg)
	result, err := a.executeTool(ctx, toolMsg.Tool, toolMsg.Arguments)
	toolMsg.ResultChan <- AgentToolResult{Output: result, Error: err}
	return err
}

func (a *Agent) defaultTools() []plugin.ToolSchema {
	tools := a.Plugins.GetTools()
	if len(tools) == 0 {
		return nil
	}
	filtered := make([]plugin.ToolSchema, 0, len(tools))
	for _, tool := range tools {
		if a.permissions != nil && !a.permissions.CanUseTool(tool.Name) {
			continue
		}
		filtered = append(filtered, cloneToolSchema(tool))
	}
	return filtered
}

func cloneToolSchema(schema plugin.ToolSchema) plugin.ToolSchema {
	schema.Parameters = cloneSchemaMap(schema.Parameters)
	return schema
}

func cloneSchemaMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = cloneSchemaValue(value)
	}
	return out
}

func cloneSchemaValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneSchemaMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = cloneSchemaValue(item)
		}
		return out
	default:
		return value
	}
}

func (a *Agent) executeTool(ctx context.Context, name, arguments string) (string, error) {
	if err := a.permissions.ValidateTool(name); err != nil {
		return "", a.toolPolicyDeniedError(ctx, name, arguments)
	}
	runtimeApproved, err := a.requireToolApproval(ctx, name, arguments)
	if err != nil {
		return "", err
	}
	if err := toolpolicy.Validate(toolpolicy.Invocation{
		Name: name, Arguments: arguments, RuntimeApproved: runtimeApproved,
	}); err != nil {
		return "", err
	}
	result, err := a.Plugins.ExecuteTool(ctx, name, arguments)
	if err != nil {
		return "", err
	}
	if a.config.ToolResultRef == nil {
		return result, nil
	}
	return a.config.ToolResultRef(name, arguments, result)
}

func (a *Agent) toolPolicyDeniedError(ctx context.Context, name, arguments string) error {
	sessionID := modelservice.SessionID(ctx)
	runID := modelservice.RunID(ctx)
	if a.Activity != nil {
		a.addActivity(sessionID, "policy.denied", map[string]any{
			"tool": name, "reason": "tool_not_allowed",
			"arguments_length": len(arguments), "run_id": runID,
		})
	}
	return boundary.New(
		boundary.Runtime,
		boundary.PolicyDenied,
		"tool."+name,
		fmt.Sprintf("tool %q is not allowed by the active agent policy", name),
	)
}

func (a *Agent) requireToolApproval(ctx context.Context, name, arguments string) (bool, error) {
	if a.execution == nil || a.execution.Mode() != execution.ModeAssistive || a.execution.Gate() == nil {
		return false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := a.execution.Gate().RequestApproval(ctx, name, arguments, modelservice.SessionID(ctx)); err != nil {
		return false, fmt.Errorf("tool call approval failed for %s: %w", name, err)
	}
	return true, nil
}
