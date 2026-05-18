package message

import (
	"context"
	"fmt"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/runtime/pkg/llm"
	"github.com/quarkloop/runtime/pkg/llmcontext"
)

// Handle runs the full message handling flow:
//  1. Build LLM context (system prompt + work status + session history)
//  2. Call LLM via Infer loop (streaming + tool calling)
//  3. Return full assistant response text
//
// Tokens are streamed to resp as they arrive.
func Handle(ctx context.Context, history []Message, llmClient *llm.Client, systemPrompt string, workSummary string, tools []plugin.ToolSchema, onTool plugin.ToolHandler, resp chan<- StreamMessage, finalGuard llm.FinalGuard) (string, error) {
	return HandleWithToolCallGate(ctx, history, llmClient, systemPrompt, workSummary, tools, onTool, resp, finalGuard, nil)
}

// HandleWithToolCallGate runs Handle with an optional workflow completion gate.
func HandleWithToolCallGate(ctx context.Context, history []Message, llmClient *llm.Client, systemPrompt string, workSummary string, tools []plugin.ToolSchema, onTool plugin.ToolHandler, resp chan<- StreamMessage, finalGuard llm.FinalGuard, toolCallGate llm.ToolCallGate) (string, error) {
	return HandleWithToolCallPolicy(ctx, history, llmClient, systemPrompt, workSummary, tools, onTool, resp, finalGuard, toolCallGate, nil, nil)
}

// HandleWithToolCallPolicy runs Handle with optional workflow tool-call policy.
func HandleWithToolCallPolicy(ctx context.Context, history []Message, llmClient *llm.Client, systemPrompt string, workSummary string, tools []plugin.ToolSchema, onTool plugin.ToolHandler, resp chan<- StreamMessage, finalGuard llm.FinalGuard, toolCallGate llm.ToolCallGate, toolCallGuard llm.ToolCallGuard, toolResultGate llm.ToolResultGate) (string, error) {
	return HandleWithToolCallPolicyAndContinuation(ctx, history, llmClient, systemPrompt, workSummary, tools, onTool, resp, finalGuard, toolCallGate, toolCallGuard, toolResultGate, nil)
}

// HandleWithToolCallPolicyAndContinuation runs Handle with optional workflow
// tool-call policy and post-tool continuation instructions.
func HandleWithToolCallPolicyAndContinuation(ctx context.Context, history []Message, llmClient *llm.Client, systemPrompt string, workSummary string, tools []plugin.ToolSchema, onTool plugin.ToolHandler, resp chan<- StreamMessage, finalGuard llm.FinalGuard, toolCallGate llm.ToolCallGate, toolCallGuard llm.ToolCallGuard, toolResultGate llm.ToolResultGate, toolResultInstruction llm.ToolResultInstruction) (string, error) {
	if llmClient == nil {
		return "", fmt.Errorf("no LLM client configured")
	}

	// Build LLM messages
	var msgs []plugin.Message

	// System prompt
	if systemPrompt != "" {
		msgs = append(msgs, plugin.Message{Role: "system", Content: systemPrompt})
	}

	// Work status injection
	if workSummary != "" && workSummary != "No active work." {
		msgs = append(msgs, plugin.Message{
			Role:    "system",
			Content: "Current work status: " + workSummary,
		})
	}

	// Session history — compact only when approaching the model's context window limit.
	contents := make([]int, len(history))
	for i, m := range history {
		contents[i] = len(m.Content)
	}
	start := llmcontext.CompactIndex(contents, llmClient.ContextWindow)
	for _, m := range history[start:] {
		msgs = append(msgs, plugin.Message{Role: m.Role, Content: m.Content})
	}

	// Infer (LLM call → tool loop → streaming)
	return llmClient.InferWithToolCallPolicyAndContinuation(ctx, msgs, tools, onTool, func(msgType string, data any) {
		Emit(ctx, resp, StreamMessage{Type: msgType, Data: data})
	}, finalGuard, toolCallGate, toolCallGuard, toolResultGate, toolResultInstruction)
}
