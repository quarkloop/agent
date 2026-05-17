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
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/quarkloop/pkg/plugin"
)

// Client wraps a provider with the inference loop.
type Client struct {
	provider      Provider
	model         string
	ContextWindow int // token limit from the model entry (0 = unknown)
	limits        InferenceLimits
}

type FinalGuard func(content string) (instruction string, retry bool)

// InferenceLimits bounds one user-visible inference request. They prevent a
// provider from keeping the runtime in an endless tool/finalization loop.
type InferenceLimits struct {
	MaxTurns             int
	MaxFinalGuardRetries int
}

func defaultInferenceLimits() InferenceLimits {
	return InferenceLimits{
		MaxTurns:             48,
		MaxFinalGuardRetries: 8,
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
	}
}

// Infer runs the full inference loop:
//
//	context → LLM call → tool handling → response streaming.
//
// It fires onMessage for streamed text and tool execution data.
// If onTool is nil, tool calls are ignored.
func (c *Client) Infer(ctx context.Context, messages []plugin.Message, tools []plugin.ToolSchema, onTool plugin.ToolHandler, onMessage func(msgType string, data any), finalGuard FinalGuard) (string, error) {
	turns := 0
	finalGuardRetries := 0
	for {
		turns++
		if turns > c.limits.MaxTurns {
			return "", fmt.Errorf("inference loop exceeded %d model turns for model %q", c.limits.MaxTurns, c.model)
		}

		stream, err := c.provider.ChatCompletionStream(ctx, &plugin.ChatRequest{
			Model:    c.model,
			Messages: messages,
			Tools:    tools,
		})
		if err != nil {
			return "", fmt.Errorf("llm call: %w", err)
		}

		var fullContent string
		var toolCalls []plugin.ToolCall

		for ev := range stream {
			if ev.Err != nil {
				return "", fmt.Errorf("stream: %w", ev.Err)
			}
			if ev.Done {
				break
			}
			if ev.Delta != "" {
				fullContent += ev.Delta
				if onMessage != nil {
					onMessage("token", ev.Delta)
				}
			}
			toolCalls = mergeToolCalls(toolCalls, ev.ToolCalls)
		}

		// No native tool calls — try parsing from text output
		if len(toolCalls) == 0 && c.provider != nil {
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

		// No tool calls at all — we're done
		if len(toolCalls) == 0 {
			if droppedToolCalls > 0 {
				messages = append(messages, plugin.Message{
					Role:    "system",
					Content: "The previous assistant response contained malformed tool calls. Retry with exactly one valid tool call using a known function name and one complete valid JSON object for arguments, or answer directly if no tool is needed.",
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
					messages = append(messages,
						plugin.Message{Role: "assistant", Content: fullContent},
						plugin.Message{Role: "system", Content: instruction},
					)
					fullContent = ""
					continue
				}
			}
			return fullContent, nil
		}
		finalGuardRetries = 0

		// No handler — return what we have
		if onTool == nil {
			return fullContent, nil
		}

		slog.Info("tool calls", "count", len(toolCalls), "names", toolCallNames(toolCalls))

		// Append assistant message with tool calls
		messages = append(messages, plugin.Message{
			Role:      "assistant",
			Content:   fullContent,
			ToolCalls: toolCalls,
		})

		// Execute each tool and append results
		for _, tc := range toolCalls {
			callID := tc.ID
			if onMessage != nil {
				onMessage("tool_start", map[string]any{
					"id":        callID,
					"name":      tc.Function.Name,
					"arguments": tc.Function.Arguments,
				})
			}

			started := time.Now()
			result, err := onTool(ctx, tc.Function.Name, tc.Function.Arguments)
			durationMillis := time.Since(started).Milliseconds()
			toolErr := err != nil
			if err != nil {
				result = fmt.Sprintf("error: %v", err)
			}
			if onMessage != nil {
				onMessage("tool_result", map[string]any{
					"id":              callID,
					"name":            tc.Function.Name,
					"result":          preview(result, 2000),
					"error":           toolErr,
					"duration_millis": durationMillis,
				})
			}
			messages = append(messages, plugin.Message{
				Role:       "tool",
				Content:    result,
				ToolCallID: callID,
			})
		}

		// Reset for next LLM call
		fullContent = ""
	}
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
