package message

import (
	"context"
	"fmt"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/runtime/pkg/llm"
)

// HandlePrepared executes the model/tool loop from raw transcript messages.
// The Harness-backed preparer owns every model-turn context package.
func HandlePrepared(ctx context.Context, messages []plugin.Message, llmClient *llm.Client, tools []plugin.ToolSchema, onTool plugin.ToolHandler, resp chan<- StreamMessage, prepare llm.ContextPreparer, toolSurface llm.ToolSurface, requiredTools llm.RequiredToolContinuation, finalGuard llm.FinalGuard, toolCallGate llm.ToolCallGate, toolCallGuard llm.ToolCallGuard, toolResultGate llm.ToolResultGate, toolResultContext llm.ToolResultContext) (string, error) {
	if llmClient == nil {
		return "", fmt.Errorf("no LLM client configured")
	}
	return llmClient.InferWithPreparedContextAndToolContinuation(ctx, messages, tools, onTool, func(msgType string, data any) {
		Emit(ctx, resp, StreamMessage{Type: msgType, Data: data})
	}, prepare, toolSurface, requiredTools, finalGuard, toolCallGate, toolCallGuard, toolResultGate, toolResultContext)
}
