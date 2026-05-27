// Package llm provides the high-level inference loop.
//
// The Infer function implements the full call order:
//  1. Call LLM with context (streaming)
//  2. If LLM returns tool calls → execute tools → feed results back → loop
//  3. Stream text tokens to response channel
//  4. Return full assistant response
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/quarkloop/pkg/boundary/redaction"
	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/runtime/pkg/runcontext"
)

// Client wraps a provider with the inference loop.
type Client struct {
	provider      Provider
	model         string
	ContextWindow int // token limit from the model entry (0 = unknown)
	limits        InferenceLimits
	progressEvery time.Duration
}

type FinalGuard func(content string) (instruction string, retry bool)

// ToolCallGate can accept streamed assistant content before executing the next
// tool calls. It is used for workflow-owned completion rules where the model
// has already produced a final answer and the remaining calls are redundant.
type ToolCallGate func(content string, toolCalls []plugin.ToolCall) bool

// ToolCallGuard can reject the next tool-call batch and ask the model to retry.
// It is used by runtime workflow policy when the model starts finalizing while
// the proposed calls do not advance required service-backed steps.
type ToolCallGuard func(content string, toolCalls []plugin.ToolCall) (instruction string, retry bool)

// ToolResultGate can accept streamed assistant content after the current tool
// calls have executed successfully. It is used for terminal workflow calls that
// complete the required service-backed steps while the model already streamed a
// user-facing final answer.
type ToolResultGate func(content string, toolCalls []plugin.ToolCall) bool

// ToolResultContext can add a transient structured runtime fact after a tool
// batch executes and before the next model turn. It must not author guidance
// that belongs in an agent plugin.
type ToolResultContext func() string

// ContextPreparer packages the raw session/tool transcript for one model
// turn. Harness supplies this boundary in production.
type ContextPreparer func(context.Context, []plugin.Message) ([]plugin.Message, error)

// ToolSurface selects the function schemas exposed for one model turn.
// Workflow owners use it to align callable operations with current progress.
type ToolSurface func([]plugin.ToolSchema) []plugin.ToolSchema

// RequiredToolContinuation returns a runtime-required tool call after prior
// accepted tool results make the action mechanical. The call still executes
// through the ordinary traced tool envelope.
type RequiredToolContinuation func() []plugin.ToolCall

// ToolCallArgumentNormalizer can rewrite tool-call arguments before workflow
// guards, trace events, and execution. Runtime service adapters use it for
// deterministic argument normalization that should not depend on provider
// behavior.
type ToolCallArgumentNormalizer func(ctx context.Context, name, arguments string) (string, error)

const (
	toolCallHistoryArgumentMaxRunes = 4000
	toolCallHistoryStringMaxRunes   = 600
	toolCallHistoryArrayMaxItems    = 3
	defaultModelProgressInterval    = 15 * time.Second
	runtimeToolValidationFactType   = "runtime.tool_call.validation"
	runtimeWorkflowValidationType   = "runtime.workflow.validation"
	runtimeWorkflowStatusType       = "runtime.workflow.status"
)

type toolCallArgumentNormalizerKey struct{}

func WithToolCallArgumentNormalizer(ctx context.Context, normalizer ToolCallArgumentNormalizer) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if normalizer == nil {
		return ctx
	}
	return context.WithValue(ctx, toolCallArgumentNormalizerKey{}, normalizer)
}

// InferenceLimits bounds one user-visible inference request. They prevent a
// provider from keeping the runtime in an endless tool/finalization loop.
type InferenceLimits struct {
	MaxTurns                  int
	MaxFinalGuardRetries      int
	MaxTransientStreamRetries int
}

func defaultInferenceLimits() InferenceLimits {
	return InferenceLimits{
		MaxTurns:                  48,
		MaxFinalGuardRetries:      8,
		MaxTransientStreamRetries: 1,
	}
}

func normalizeInferenceLimits(limits InferenceLimits) InferenceLimits {
	defaults := defaultInferenceLimits()
	if limits.MaxTurns <= 0 {
		limits.MaxTurns = defaults.MaxTurns
	}
	if limits.MaxFinalGuardRetries <= 0 {
		limits.MaxFinalGuardRetries = defaults.MaxFinalGuardRetries
	}
	if limits.MaxTransientStreamRetries <= 0 {
		limits.MaxTransientStreamRetries = defaults.MaxTransientStreamRetries
	}
	return limits
}

// NewClient creates a new LLM client.
func NewClient(p Provider, model string, contextWindow int) *Client {
	return NewClientWithLimits(p, model, contextWindow, InferenceLimits{})
}

// NewClientWithLimits creates a client with explicit inference limits.
func NewClientWithLimits(p Provider, model string, contextWindow int, limits InferenceLimits) *Client {
	return &Client{
		provider:      p,
		model:         model,
		ContextWindow: contextWindow,
		limits:        normalizeInferenceLimits(limits),
		progressEvery: defaultModelProgressInterval,
	}
}

// Infer runs the full inference loop:
//
//	context → LLM call → tool handling → response streaming.
//
// It fires onMessage for accepted user-facing text and tool execution data.
// If onTool is nil, tool calls are ignored.
func (c *Client) Infer(ctx context.Context, messages []plugin.Message, tools []plugin.ToolSchema, onTool plugin.ToolHandler, onMessage func(msgType string, data any), finalGuard FinalGuard) (string, error) {
	return c.InferWithToolCallGate(ctx, messages, tools, onTool, onMessage, finalGuard, nil)
}

// InferWithToolCallGate runs Infer with an optional pre-tool completion gate.
func (c *Client) InferWithToolCallGate(ctx context.Context, messages []plugin.Message, tools []plugin.ToolSchema, onTool plugin.ToolHandler, onMessage func(msgType string, data any), finalGuard FinalGuard, toolCallGate ToolCallGate) (string, error) {
	return c.InferWithToolCallPolicy(ctx, messages, tools, onTool, onMessage, finalGuard, toolCallGate, nil, nil)
}

// InferWithToolCallPolicy runs Infer with workflow-owned pre-tool policies.
func (c *Client) InferWithToolCallPolicy(ctx context.Context, messages []plugin.Message, tools []plugin.ToolSchema, onTool plugin.ToolHandler, onMessage func(msgType string, data any), finalGuard FinalGuard, toolCallGate ToolCallGate, toolCallGuard ToolCallGuard, toolResultGate ToolResultGate) (string, error) {
	return c.InferWithToolCallPolicyAndContinuation(ctx, messages, tools, onTool, onMessage, finalGuard, toolCallGate, toolCallGuard, toolResultGate, nil)
}

// InferWithToolCallPolicyAndContinuation runs Infer with workflow-owned
// pre-tool policies and post-tool continuation instructions.
func (c *Client) InferWithToolCallPolicyAndContinuation(ctx context.Context, messages []plugin.Message, tools []plugin.ToolSchema, onTool plugin.ToolHandler, onMessage func(msgType string, data any), finalGuard FinalGuard, toolCallGate ToolCallGate, toolCallGuard ToolCallGuard, toolResultGate ToolResultGate, toolResultContext ToolResultContext) (string, error) {
	return c.inferWithPolicy(ctx, messages, tools, onTool, onMessage, nil, nil, nil, finalGuard, toolCallGate, toolCallGuard, toolResultGate, toolResultContext)
}

// InferWithPreparedContextAndPolicy runs the bounded model/tool loop while
// delegating each outgoing context package to Harness.
func (c *Client) InferWithPreparedContextAndPolicy(ctx context.Context, messages []plugin.Message, tools []plugin.ToolSchema, onTool plugin.ToolHandler, onMessage func(msgType string, data any), prepare ContextPreparer, finalGuard FinalGuard, toolCallGate ToolCallGate, toolCallGuard ToolCallGuard, toolResultGate ToolResultGate, toolResultContext ToolResultContext) (string, error) {
	return c.inferWithPolicy(ctx, messages, tools, onTool, onMessage, prepare, nil, nil, finalGuard, toolCallGate, toolCallGuard, toolResultGate, toolResultContext)
}

// InferWithPreparedContextAndToolSurface runs the bounded loop with Harness
// context packaging and a workflow-owned per-turn callable surface.
func (c *Client) InferWithPreparedContextAndToolSurface(ctx context.Context, messages []plugin.Message, tools []plugin.ToolSchema, onTool plugin.ToolHandler, onMessage func(msgType string, data any), prepare ContextPreparer, surface ToolSurface, finalGuard FinalGuard, toolCallGate ToolCallGate, toolCallGuard ToolCallGuard, toolResultGate ToolResultGate, toolResultContext ToolResultContext) (string, error) {
	return c.inferWithPolicy(ctx, messages, tools, onTool, onMessage, prepare, surface, nil, finalGuard, toolCallGate, toolCallGuard, toolResultGate, toolResultContext)
}

// InferWithPreparedContextAndToolContinuation runs the bounded loop with a
// workflow-owned per-turn surface and required mechanical tool continuations.
func (c *Client) InferWithPreparedContextAndToolContinuation(ctx context.Context, messages []plugin.Message, tools []plugin.ToolSchema, onTool plugin.ToolHandler, onMessage func(msgType string, data any), prepare ContextPreparer, surface ToolSurface, requiredTools RequiredToolContinuation, finalGuard FinalGuard, toolCallGate ToolCallGate, toolCallGuard ToolCallGuard, toolResultGate ToolResultGate, toolResultContext ToolResultContext) (string, error) {
	return c.inferWithPolicy(ctx, messages, tools, onTool, onMessage, prepare, surface, requiredTools, finalGuard, toolCallGate, toolCallGuard, toolResultGate, toolResultContext)
}

func (c *Client) inferWithPolicy(ctx context.Context, messages []plugin.Message, tools []plugin.ToolSchema, onTool plugin.ToolHandler, onMessage func(msgType string, data any), prepare ContextPreparer, surface ToolSurface, requiredTools RequiredToolContinuation, finalGuard FinalGuard, toolCallGate ToolCallGate, toolCallGuard ToolCallGuard, toolResultGate ToolResultGate, toolResultContext ToolResultContext) (string, error) {
	turns := 0
	finalGuardRetries := 0
	toolCallValidationRetries := 0
	transientStreamRetries := 0
	for {
		turnTools := tools
		if surface != nil {
			turnTools = surface(append([]plugin.ToolSchema(nil), tools...))
		}
		var fullContent string
		var toolCalls []plugin.ToolCall
		if requiredTools != nil {
			toolCalls = requiredTools()
		}
		if len(toolCalls) > 0 {
			emitRequiredToolContinuationProgress(onMessage, toolCalls)
		} else {
			turns++
			if turns > c.limits.MaxTurns {
				return "", fmt.Errorf("inference loop exceeded %d model turns for model %q", c.limits.MaxTurns, c.model)
			}
			turnMessages := messages
			if prepare != nil {
				var err error
				turnMessages, err = prepare(ctx, append([]plugin.Message(nil), messages...))
				if err != nil {
					return "", fmt.Errorf("prepare model context: %w", err)
				}
			}
			stream, err := c.provider.ChatCompletionStream(ctx, &plugin.ChatRequest{
				Model:    c.model,
				Messages: turnMessages,
				Tools:    turnTools,
			})
			if err != nil {
				return "", fmt.Errorf("llm call: %w", err)
			}

			fullContent, toolCalls, err = c.readModelTurn(ctx, stream, onMessage)
			if err != nil {
				if transientStreamRetries < c.limits.MaxTransientStreamRetries && retryableUncommittedStreamError(ctx, err) {
					transientStreamRetries++
					emitModelTurnRetryProgress(onMessage, transientStreamRetries)
					continue
				}
				return "", err
			}
			transientStreamRetries = 0
		}

		// Some providers express callable functions in text rather than native
		// tool events. Parse that fallback only while a function surface is
		// exposed; once a workflow enters its answer-only turn, text must stay
		// text and cannot re-enter execution as a fabricated call.
		if len(toolCalls) == 0 && c.provider != nil && len(turnTools) > 0 {
			parsedTools, cleaned := c.provider.ParseToolCalls(fullContent)
			if len(parsedTools) > 0 {
				toolCalls = parsedTools
				fullContent = strings.TrimSpace(cleaned)
			}
		}
		normalizedToolCalls, droppedToolCalls := normalizeToolCalls(toolCalls)
		if droppedToolCalls > 0 {
			slog.Warn("dropped malformed tool calls", "count", droppedToolCalls)
		}
		toolCalls = normalizedToolCalls
		if unavailable := unexposedToolCallNames(toolCalls, turnTools); len(unavailable) > 0 {
			toolCallValidationRetries++
			if toolCallValidationRetries > c.limits.MaxFinalGuardRetries {
				return "", fmt.Errorf("tool-call surface validation exceeded %d retries for model %q", c.limits.MaxFinalGuardRetries, c.model)
			}
			emitToolValidationProgress(onMessage, unavailable[0], "function_not_exposed_this_turn")
			messages = pruneTransientRuntimeFacts(messages, false)
			messages = append(messages, plugin.Message{
				Role:    "system",
				Content: unexposedToolCallsInstruction(unavailable, turnTools),
			})
			fullContent = ""
			continue
		}
		if len(toolCalls) > 0 {
			normalized, functionName, err := normalizeToolCallArgumentsFromContext(ctx, toolCalls)
			if err != nil {
				toolCallValidationRetries++
				if toolCallValidationRetries > c.limits.MaxFinalGuardRetries {
					return "", fmt.Errorf("tool-call argument validation exceeded %d retries for model %q: %w", c.limits.MaxFinalGuardRetries, c.model, err)
				}
				emitToolArgumentValidationProgress(onMessage, functionName, err)
				messages = pruneTransientRuntimeFacts(messages, false)
				messages = append(messages, plugin.Message{
					Role:    "system",
					Content: invalidToolArgumentsInstruction(functionName, err),
				})
				fullContent = ""
				continue
			}
			toolCalls = normalized
			toolCallValidationRetries = 0
		}
		if len(toolCalls) > 0 && toolCallGuard != nil {
			instruction, retry := toolCallGuard(fullContent, toolCalls)
			if retry {
				finalGuardRetries++
				if finalGuardRetries > c.limits.MaxFinalGuardRetries {
					return "", fmt.Errorf("tool-call guard exceeded %d retries for model %q", c.limits.MaxFinalGuardRetries, c.model)
				}
				emitToolValidationProgress(onMessage, firstToolCallName(toolCalls), runtimeValidationReason(instruction))
				messages = pruneTransientRuntimeFacts(messages, false)
				messages = append(messages, plugin.Message{Role: "system", Content: instruction})
				fullContent = ""
				continue
			}
		}
		if len(toolCalls) > 0 && toolCallGate != nil && toolCallGate(fullContent, toolCalls) {
			if finalGuard != nil {
				instruction, retry := finalGuard(fullContent)
				if retry {
					finalGuardRetries++
					if finalGuardRetries > c.limits.MaxFinalGuardRetries {
						return "", fmt.Errorf("finalization guard exceeded %d retries for model %q", c.limits.MaxFinalGuardRetries, c.model)
					}
					emitFinalValidationProgress(onMessage, runtimeValidationReason(instruction))
					// Invalid final output is not conversation history. In
					// particular, retaining protocol markup gives a model an
					// example of the exact representation it must stop using.
					messages = pruneTransientRuntimeFacts(messages, false)
					messages = append(messages, plugin.Message{Role: "system", Content: instruction})
					fullContent = ""
					continue
				}
			}
			emitFinalContent(onMessage, fullContent)
			return fullContent, nil
		}

		// No tool calls at all — we're done
		if len(toolCalls) == 0 {
			if droppedToolCalls > 0 {
				emitToolValidationProgress(onMessage, "", "malformed_tool_calls")
				messages = append(messages, plugin.Message{
					Role:    "system",
					Content: `{"type":"` + runtimeToolValidationFactType + `","status":"rejected","reason":"malformed_tool_calls"}`,
				})
				continue
			}
			if finalGuard != nil {
				instruction, retry := finalGuard(fullContent)
				if retry {
					finalGuardRetries++
					if finalGuardRetries > c.limits.MaxFinalGuardRetries {
						return "", fmt.Errorf("finalization guard exceeded %d retries for model %q", c.limits.MaxFinalGuardRetries, c.model)
					}
					emitFinalValidationProgress(onMessage, runtimeValidationReason(instruction))
					// Do not feed rejected answer text or internal protocol
					// markup back into an answer-only model turn.
					messages = pruneTransientRuntimeFacts(messages, false)
					messages = append(messages, plugin.Message{Role: "system", Content: instruction})
					fullContent = ""
					continue
				}
			}
			emitFinalContent(onMessage, fullContent)
			return fullContent, nil
		}
		finalGuardRetries = 0

		// No handler — return what we have
		if onTool == nil {
			emitFinalContent(onMessage, fullContent)
			return fullContent, nil
		}

		sessionID := runcontext.SessionID(ctx)
		runID := runcontext.RunID(ctx)
		slog.Info("tool calls", "count", len(toolCalls), "names", toolCallNames(toolCalls), "session_id", sessionID, "run_id", runID)

		// Validation facts are corrective input for one retry only. Once a
		// valid tool batch advances the workflow, keeping them in later turns
		// increases context size and can contradict the current status fact.
		messages = pruneTransientRuntimeFacts(messages, false)

		// Append assistant message with tool calls
		messages = append(messages, plugin.Message{
			Role:      "assistant",
			Content:   fullContent,
			ToolCalls: compactToolCallsForHistory(toolCalls),
		})

		// Execute each tool and append results
		for _, tc := range toolCalls {
			callID := tc.ID
			startedAt := time.Now().UTC()
			if onMessage != nil {
				onMessage("tool_start", map[string]any{
					"id":           callID,
					"tool_call_id": callID,
					"name":         tc.Function.Name,
					"arguments":    tc.Function.Arguments,
					"session_id":   sessionID,
					"run_id":       runID,
					"observed_at":  startedAt.Format(time.RFC3339Nano),
				})
			}

			started := time.Now()
			result, err := onTool(ctx, tc.Function.Name, tc.Function.Arguments)
			durationMillis := time.Since(started).Milliseconds()
			finishedAt := time.Now().UTC()
			toolErr := err != nil
			if err != nil {
				result = fmt.Sprintf("error: %v", err)
			}
			if onMessage != nil {
				event := map[string]any{
					"id":              callID,
					"tool_call_id":    callID,
					"name":            tc.Function.Name,
					"result":          preview(result, 2000),
					"error":           toolErr,
					"duration_millis": durationMillis,
					"session_id":      sessionID,
					"run_id":          runID,
					"observed_at":     finishedAt.Format(time.RFC3339Nano),
				}
				for key, value := range serviceCallFieldsFromResult(result) {
					event[key] = value
				}
				onMessage("tool_result", event)
			}
			messages = append(messages, plugin.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: callID,
			})
		}

		if toolResultGate != nil && toolResultGate(fullContent, toolCalls) {
			if finalGuard != nil {
				instruction, retry := finalGuard(fullContent)
				if retry {
					finalGuardRetries++
					if finalGuardRetries > c.limits.MaxFinalGuardRetries {
						return "", fmt.Errorf("finalization guard exceeded %d retries for model %q", c.limits.MaxFinalGuardRetries, c.model)
					}
					emitFinalValidationProgress(onMessage, runtimeValidationReason(instruction))
					messages = append(messages, plugin.Message{Role: "system", Content: instruction})
					fullContent = ""
					continue
				}
			}
			emitFinalContent(onMessage, fullContent)
			return fullContent, nil
		}

		if toolResultContext != nil {
			if fact := strings.TrimSpace(toolResultContext()); fact != "" {
				messages = pruneTransientRuntimeFacts(messages, true)
				messages = append(messages, plugin.Message{Role: "system", Content: fact})
			}
		}

		// Reset for next LLM call
		fullContent = ""
	}
}

func (c *Client) readModelTurn(ctx context.Context, stream <-chan plugin.StreamEvent, onMessage func(msgType string, data any)) (string, []plugin.ToolCall, error) {
	var fullContent string
	var toolCalls []plugin.ToolCall
	progressEvery := c.progressEvery
	if progressEvery <= 0 {
		progressEvery = defaultModelProgressInterval
	}
	ticker := time.NewTicker(progressEvery)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return "", nil, ctx.Err()
		case <-ticker.C:
			if onMessage != nil {
				onMessage("progress", map[string]string{
					"phase": "model_turn",
					"state": "waiting",
				})
			}
		case ev, ok := <-stream:
			if !ok {
				return fullContent, toolCalls, nil
			}
			if ev.Err != nil {
				return "", nil, fmt.Errorf("stream: %w", ev.Err)
			}
			if ev.Done {
				return fullContent, toolCalls, nil
			}
			if ev.Delta != "" {
				fullContent += ev.Delta
			}
			toolCalls = mergeToolCalls(toolCalls, ev.ToolCalls)
		}
	}
}

func emitFinalContent(onMessage func(msgType string, data any), content string) {
	if onMessage != nil && content != "" {
		onMessage("token", content)
	}
}

func retryableUncommittedStreamError(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil {
		return false
	}
	return errors.Is(err, context.DeadlineExceeded)
}

func emitModelTurnRetryProgress(onMessage func(msgType string, data any), attempt int) {
	if onMessage == nil {
		return
	}
	onMessage("progress", map[string]string{
		"phase":      "model_turn",
		"state":      "retrying",
		"reason":     "transient_stream_timeout",
		"diagnostic": fmt.Sprintf("retry=%d; no tool call was executed for the failed model turn", attempt),
	})
}

func emitRequiredToolContinuationProgress(onMessage func(msgType string, data any), calls []plugin.ToolCall) {
	if onMessage == nil || len(calls) == 0 {
		return
	}
	onMessage("progress", map[string]string{
		"phase":      "workflow",
		"state":      "executing_required_action",
		"function":   strings.TrimSpace(calls[0].Function.Name),
		"diagnostic": "origin=runtime.workflow; prerequisite service results were accepted",
	})
}

func emitFinalValidationProgress(onMessage func(msgType string, data any), reason string) {
	if onMessage == nil {
		return
	}
	onMessage("progress", map[string]string{
		"phase":  "final_response_validation",
		"state":  "rejected",
		"reason": reason,
	})
}

func emitToolValidationProgress(onMessage func(msgType string, data any), functionName, reason string) {
	if onMessage == nil {
		return
	}
	event := map[string]string{"phase": "tool_call_validation", "state": "rejected"}
	if functionName != "" {
		event["function"] = functionName
	}
	if reason != "" {
		event["reason"] = reason
	}
	onMessage("progress", event)
}

func emitToolArgumentValidationProgress(onMessage func(msgType string, data any), functionName string, err error) {
	if onMessage == nil {
		return
	}
	event := map[string]string{
		"phase":      "tool_call_validation",
		"state":      "rejected",
		"function":   functionName,
		"reason":     "invalid_arguments",
		"diagnostic": redaction.RedactString(err.Error()),
	}
	onMessage("progress", event)
}

func runtimeValidationReason(instruction string) string {
	var fact struct {
		Reason string `json:"reason"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(instruction)), &fact) != nil {
		return ""
	}
	return strings.TrimSpace(fact.Reason)
}

func firstToolCallName(calls []plugin.ToolCall) string {
	if len(calls) == 0 {
		return ""
	}
	return strings.TrimSpace(calls[0].Function.Name)
}

// pruneTransientRuntimeFacts removes only runtime-authored facts that have
// been superseded by accepted progress. Plugin prompt material and complete
// assistant/tool execution pairs are never changed here.
func pruneTransientRuntimeFacts(messages []plugin.Message, removeStatus bool) []plugin.Message {
	filtered := make([]plugin.Message, 0, len(messages))
	for _, message := range messages {
		factType := runtimeFactType(message)
		if factType == runtimeToolValidationFactType || factType == runtimeWorkflowValidationType ||
			(removeStatus && factType == runtimeWorkflowStatusType) {
			continue
		}
		filtered = append(filtered, message)
	}
	return filtered
}

func runtimeFactType(message plugin.Message) string {
	if message.Role != "system" {
		return ""
	}
	var fact struct {
		Type string `json:"type"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(message.Content)), &fact) != nil {
		return ""
	}
	return fact.Type
}

func serviceCallFieldsFromResult(result string) map[string]string {
	var payload struct {
		ServiceCall struct {
			ServiceCallID string `json:"serviceCallId"`
			ReferenceID   string `json:"referenceId"`
			AuditRef      string `json:"auditRef"`
			TraceID       string `json:"traceId"`
			Subject       string `json:"subject"`
		} `json:"_serviceCall"`
	}
	if err := json.Unmarshal([]byte(result), &payload); err != nil || payload.ServiceCall.ServiceCallID == "" {
		return nil
	}
	out := map[string]string{
		"service_call_id": payload.ServiceCall.ServiceCallID,
		"reference_id":    payload.ServiceCall.ReferenceID,
		"audit_ref":       payload.ServiceCall.AuditRef,
	}
	if payload.ServiceCall.TraceID != "" {
		out["trace_id"] = payload.ServiceCall.TraceID
	}
	if payload.ServiceCall.Subject != "" {
		out["subject"] = payload.ServiceCall.Subject
	}
	return out
}

func normalizeToolCallArgumentsFromContext(ctx context.Context, calls []plugin.ToolCall) ([]plugin.ToolCall, string, error) {
	normalizer, ok := ctx.Value(toolCallArgumentNormalizerKey{}).(ToolCallArgumentNormalizer)
	if !ok || normalizer == nil {
		return calls, "", nil
	}
	out := make([]plugin.ToolCall, len(calls))
	copy(out, calls)
	for i, call := range out {
		arguments, err := normalizer(ctx, call.Function.Name, call.Function.Arguments)
		if err != nil {
			return nil, call.Function.Name, fmt.Errorf("normalize tool call %s arguments: %w", call.Function.Name, err)
		}
		out[i].Function.Arguments = arguments
	}
	return out, "", nil
}

func invalidToolArgumentsInstruction(functionName string, err error) string {
	payload := map[string]string{
		"type":        runtimeToolValidationFactType,
		"status":      "rejected",
		"reason":      "invalid_arguments",
		"function":    functionName,
		"requirement": "Arguments must satisfy the declared JSON schema; arrays and objects must be native JSON values, not encoded strings.",
	}
	if err != nil {
		payload["validation_error"] = redaction.RedactString(err.Error())
	}
	data, _ := json.Marshal(payload)
	return string(data)
}

func unexposedToolCallNames(calls []plugin.ToolCall, tools []plugin.ToolSchema) []string {
	if len(calls) == 0 {
		return nil
	}
	available := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		available[tool.Name] = struct{}{}
	}
	var unavailable []string
	seen := make(map[string]struct{})
	for _, call := range calls {
		name := strings.TrimSpace(call.Function.Name)
		if _, ok := available[name]; ok {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		unavailable = append(unavailable, name)
	}
	return unavailable
}

func unexposedToolCallsInstruction(functions []string, tools []plugin.ToolSchema) string {
	allowed := make([]string, 0, len(tools))
	for _, tool := range tools {
		if name := strings.TrimSpace(tool.Name); name != "" {
			allowed = append(allowed, name)
		}
	}
	payload := map[string]any{
		"type":              runtimeToolValidationFactType,
		"status":            "rejected",
		"reason":            "function_not_exposed_this_turn",
		"functions":         append([]string(nil), functions...),
		"allowed_functions": allowed,
	}
	data, _ := json.Marshal(payload)
	return string(data)
}

func compactToolCallsForHistory(calls []plugin.ToolCall) []plugin.ToolCall {
	out := make([]plugin.ToolCall, len(calls))
	copy(out, calls)
	for i := range out {
		out[i].Function.Arguments = compactToolCallArgumentsForHistory(out[i].Function.Arguments)
	}
	return out
}

func compactToolCallArgumentsForHistory(arguments string) string {
	runeCount := len([]rune(arguments))
	if runeCount <= toolCallHistoryArgumentMaxRunes {
		return arguments
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(arguments))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return compactFallbackToolArguments(runeCount)
	}
	compactJSONValueForHistory(value)
	data, err := json.Marshal(value)
	if err != nil {
		return compactFallbackToolArguments(runeCount)
	}
	if len([]rune(string(data))) <= toolCallHistoryArgumentMaxRunes {
		return string(data)
	}
	return compactFallbackToolArguments(runeCount)
}

func compactJSONValueForHistory(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			switch childValue := child.(type) {
			case string:
				if compacted, ok := compactStringForHistory(childValue); ok {
					typed[key] = compacted
					typed[key+"Chars"] = len([]rune(childValue))
					typed[key+"Truncated"] = true
				}
			case []any:
				originalLen := len(childValue)
				for i := range childValue {
					compactJSONValueForHistory(childValue[i])
				}
				if originalLen > toolCallHistoryArrayMaxItems {
					typed[key] = childValue[:toolCallHistoryArrayMaxItems]
					typed[key+"Count"] = originalLen
					typed[key+"Truncated"] = true
				}
			case map[string]any:
				compactJSONValueForHistory(childValue)
			}
		}
	case []any:
		for i := range typed {
			compactJSONValueForHistory(typed[i])
		}
	}
}

func compactStringForHistory(value string) (string, bool) {
	runes := []rune(value)
	if len(runes) <= toolCallHistoryStringMaxRunes {
		return value, false
	}
	return string(runes[:toolCallHistoryStringMaxRunes]), true
}

func compactFallbackToolArguments(chars int) string {
	data, _ := json.Marshal(map[string]any{
		"_argumentsChars":     chars,
		"_argumentsTruncated": true,
	})
	return string(data)
}

// mergeToolCalls accumulates streamed tool call deltas by index.
func mergeToolCalls(existing []plugin.ToolCall, deltas []plugin.ToolCall) []plugin.ToolCall {
	for _, d := range deltas {
		idx := d.Index

		// Grow slice to fit
		for len(existing) <= idx {
			existing = append(existing, plugin.ToolCall{})
		}

		tc := &existing[idx]
		tc.Index = idx // CRITICAL: Retain the proper loop index!
		if d.ID != "" {
			tc.ID = d.ID
		}
		if d.Type != "" {
			tc.Type = d.Type
		}
		if d.Function.Name != "" {
			tc.Function.Name = d.Function.Name
		}
		tc.Function.Arguments += d.Function.Arguments
	}
	return existing
}

func normalizeToolCalls(calls []plugin.ToolCall) ([]plugin.ToolCall, int) {
	normalized := make([]plugin.ToolCall, 0, len(calls))
	dropped := 0
	for _, call := range calls {
		name := strings.TrimSpace(call.Function.Name)
		if name == "" {
			dropped++
			continue
		}

		call.Index = len(normalized)
		call.Function.Name = name
		call.ID = strings.TrimSpace(call.ID)
		if call.ID == "" {
			call.ID = fmt.Sprintf("call_%d", len(normalized)+1)
		}
		call.Type = strings.TrimSpace(call.Type)
		if call.Type == "" {
			call.Type = "function"
		}
		if strings.TrimSpace(call.Function.Arguments) == "" {
			call.Function.Arguments = "{}"
		}
		call.Function.Arguments = strings.TrimSpace(call.Function.Arguments)
		if !validToolCallArguments(call.Function.Arguments) {
			dropped++
			continue
		}
		normalized = append(normalized, call)
	}
	return normalized, dropped
}

func validToolCallArguments(arguments string) bool {
	var payload map[string]json.RawMessage
	return json.Unmarshal([]byte(arguments), &payload) == nil
}

func toolCallNames(calls []plugin.ToolCall) []string {
	names := make([]string, 0, len(calls))
	for _, call := range calls {
		names = append(names, call.Function.Name)
	}
	return names
}

func preview(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "...(truncated)"
}
