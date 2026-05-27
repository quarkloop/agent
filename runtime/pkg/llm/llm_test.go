package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/runtime/pkg/runcontext"
)

type fakeProvider struct {
	stream func(context.Context, *plugin.ChatRequest) (<-chan plugin.StreamEvent, error)
	parse  func(string) ([]plugin.ToolCall, string)
}

func (p fakeProvider) ChatCompletionStream(ctx context.Context, req *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
	return p.stream(ctx, req)
}

func (p fakeProvider) ParseToolCalls(content string) ([]plugin.ToolCall, string) {
	if p.parse == nil {
		return nil, content
	}
	return p.parse(content)
}

func TestInferStopsEndlessToolLoop(t *testing.T) {
	provider := fakeProvider{
		stream: func(context.Context, *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			ch := make(chan plugin.StreamEvent, 2)
			ch <- plugin.StreamEvent{
				ToolCalls: []plugin.ToolCall{{
					Index: 0,
					ID:    "call-1",
					Type:  "function",
					Function: plugin.ToolCallFunction{
						Name:      "looping_tool",
						Arguments: `{}`,
					},
				}},
			}
			ch <- plugin.StreamEvent{Done: true}
			close(ch)
			return ch, nil
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{MaxTurns: 3, MaxFinalGuardRetries: 2})

	_, err := client.Infer(
		context.Background(),
		[]plugin.Message{{Role: "user", Content: "start"}},
		[]plugin.ToolSchema{{Name: "looping_tool"}},
		func(context.Context, string, string) (string, error) { return "{}", nil },
		nil,
		nil,
	)
	if err == nil {
		t.Fatal("expected endless tool loop to fail")
	}
	if !strings.Contains(err.Error(), "exceeded 3 model turns") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInferStopsEndlessFinalGuardLoop(t *testing.T) {
	provider := fakeProvider{
		stream: func(context.Context, *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			ch := make(chan plugin.StreamEvent, 2)
			ch <- plugin.StreamEvent{Delta: "not done"}
			ch <- plugin.StreamEvent{Done: true}
			close(ch)
			return ch, nil
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{MaxTurns: 10, MaxFinalGuardRetries: 2})
	var rejectedFinalTurns int

	_, err := client.Infer(
		context.Background(),
		[]plugin.Message{{Role: "user", Content: "start"}},
		nil,
		nil,
		func(kind string, data any) {
			if kind == "progress" && data.(map[string]string)["phase"] == "final_response_validation" {
				rejectedFinalTurns++
			}
		},
		func(string) (string, bool) { return `{"reason":"finalization_incomplete"}`, true },
	)
	if err == nil {
		t.Fatal("expected endless finalization guard loop to fail")
	}
	if !strings.Contains(err.Error(), "finalization guard exceeded 2 retries") {
		t.Fatalf("unexpected error: %v", err)
	}
	if rejectedFinalTurns != 2 {
		t.Fatalf("reported rejected final turns = %d, want 2", rejectedFinalTurns)
	}
}

func TestInferDoesNotRetainRejectedFinalProtocolMarkup(t *testing.T) {
	providerCalls := 0
	var retryMessages []plugin.Message
	provider := fakeProvider{
		stream: func(_ context.Context, req *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			providerCalls++
			if providerCalls == 1 {
				return streamEvents(
					plugin.StreamEvent{Delta: `I will call a tool. <longcat_tool_call>{"name":"extra"}</longcat_tool_call>`},
					plugin.StreamEvent{Done: true},
				), nil
			}
			retryMessages = append([]plugin.Message(nil), req.Messages...)
			return streamEvents(
				plugin.StreamEvent{Delta: "The indexed documents are ready for questions."},
				plugin.StreamEvent{Done: true},
			), nil
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{MaxTurns: 2, MaxFinalGuardRetries: 1})

	result, err := client.Infer(
		context.Background(),
		[]plugin.Message{{Role: "user", Content: "index these documents"}},
		nil,
		nil,
		nil,
		func(content string) (string, bool) {
			if strings.Contains(content, "<longcat_tool_call>") {
				return `{"type":"runtime.workflow.validation","status":"blocked","reason":"internal_tool_markup_in_final_response","response_mode":"user_answer_only"}`, true
			}
			return "", false
		},
	)
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if result != "The indexed documents are ready for questions." {
		t.Fatalf("result = %q", result)
	}
	var correctionSeen bool
	for _, message := range retryMessages {
		if message.Role == "assistant" && strings.Contains(message.Content, "<longcat_tool_call>") {
			t.Fatalf("rejected protocol markup retained in retry context: %#v", retryMessages)
		}
		if message.Role == "system" && strings.Contains(message.Content, `"response_mode":"user_answer_only"`) {
			correctionSeen = true
		}
	}
	if !correctionSeen {
		t.Fatalf("retry context is missing final correction: %#v", retryMessages)
	}
}

func TestInferPropagatesProviderError(t *testing.T) {
	want := errors.New("provider down")
	provider := fakeProvider{
		stream: func(context.Context, *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			return nil, want
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{MaxTurns: 3, MaxFinalGuardRetries: 2})

	_, err := client.Infer(context.Background(), nil, nil, nil, nil, nil)
	if !errors.Is(err, want) {
		t.Fatalf("expected provider error %v, got %v", want, err)
	}
}

func TestInferStreamsTraceableToolEvents(t *testing.T) {
	provider := fakeProvider{
		stream: func(context.Context, *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			ch := make(chan plugin.StreamEvent, 2)
			ch <- plugin.StreamEvent{
				ToolCalls: []plugin.ToolCall{{
					Index: 0,
					ID:    "call-1",
					Type:  "function",
					Function: plugin.ToolCallFunction{
						Name:      "indexer_UpsertChunk",
						Arguments: `{"chunkId":"chunk-1"}`,
					},
				}},
			}
			ch <- plugin.StreamEvent{Done: true}
			close(ch)
			return ch, nil
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{MaxTurns: 1, MaxFinalGuardRetries: 1})

	var events []map[string]any
	ctx := runcontext.WithRunID(runcontext.WithSessionID(context.Background(), "session-1"), "run-1")
	_, err := client.Infer(
		ctx,
		[]plugin.Message{{Role: "user", Content: "index"}},
		[]plugin.ToolSchema{{Name: "indexer_UpsertChunk"}},
		func(context.Context, string, string) (string, error) { return "", fmt.Errorf("write failed") },
		func(kind string, data any) {
			payload, ok := data.(map[string]any)
			if !ok {
				t.Fatalf("event %s payload type = %T", kind, data)
			}
			payload["kind"] = kind
			events = append(events, payload)
		},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "exceeded 1 model turns") {
		t.Fatalf("expected bounded loop error after tool result, got %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events = %+v, want start and result", events)
	}
	if events[0]["kind"] != "tool_start" || events[0]["id"] != "call-1" || events[0]["name"] != "indexer_UpsertChunk" {
		t.Fatalf("tool start event not traceable: %+v", events[0])
	}
	if events[0]["session_id"] != "session-1" || events[0]["run_id"] != "run-1" || events[0]["tool_call_id"] != "call-1" || events[0]["service_call_id"] != nil || events[0]["observed_at"] == "" {
		t.Fatalf("tool start event missing correlation fields: %+v", events[0])
	}
	if events[1]["kind"] != "tool_result" || events[1]["id"] != "call-1" || events[1]["error"] != true {
		t.Fatalf("tool result event not traceable: %+v", events[1])
	}
	if events[1]["session_id"] != "session-1" || events[1]["run_id"] != "run-1" || events[1]["tool_call_id"] != "call-1" || events[1]["service_call_id"] != nil || events[1]["observed_at"] == "" {
		t.Fatalf("tool result event missing correlation fields: %+v", events[1])
	}
}

func TestInferPublishesProgressWhileWaitingForModelTurn(t *testing.T) {
	provider := fakeProvider{
		stream: func(context.Context, *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			ch := make(chan plugin.StreamEvent, 1)
			go func() {
				time.Sleep(15 * time.Millisecond)
				ch <- plugin.StreamEvent{Delta: "ready"}
				ch <- plugin.StreamEvent{Done: true}
				close(ch)
			}()
			return ch, nil
		},
	}
	client := NewClient(provider, "test-model", 0)
	client.progressEvery = time.Millisecond
	var progress bool

	response, err := client.Infer(context.Background(), nil, nil, nil, func(kind string, _ any) {
		if kind == "progress" {
			progress = true
		}
	}, nil)
	if err != nil {
		t.Fatalf("infer: %v", err)
	}
	if response != "ready" || !progress {
		t.Fatalf("response = %q progress = %t", response, progress)
	}
}

func TestInferRetriesTransientUncommittedModelTurn(t *testing.T) {
	providerCalls := 0
	provider := fakeProvider{
		stream: func(context.Context, *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			providerCalls++
			if providerCalls == 1 {
				return streamEvents(plugin.StreamEvent{Err: context.DeadlineExceeded}), nil
			}
			return streamEvents(plugin.StreamEvent{Delta: "recovered"}, plugin.StreamEvent{Done: true}), nil
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{
		MaxTurns:                  3,
		MaxFinalGuardRetries:      1,
		MaxTransientStreamRetries: 1,
	})

	var retry map[string]string
	result, err := client.Infer(context.Background(), nil, nil, nil, func(kind string, data any) {
		if kind == "progress" {
			event := data.(map[string]string)
			if event["state"] == "retrying" {
				retry = event
			}
		}
	}, nil)
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if result != "recovered" || providerCalls != 2 {
		t.Fatalf("result = %q provider calls = %d", result, providerCalls)
	}
	if retry["phase"] != "model_turn" || retry["reason"] != "transient_stream_timeout" ||
		!strings.Contains(retry["diagnostic"], "no tool call was executed") {
		t.Fatalf("retry progress = %+v", retry)
	}
}

func TestInferBoundsTransientUncommittedModelTurnRetries(t *testing.T) {
	providerCalls := 0
	provider := fakeProvider{
		stream: func(context.Context, *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			providerCalls++
			return streamEvents(plugin.StreamEvent{Err: context.DeadlineExceeded}), nil
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{
		MaxTurns:                  3,
		MaxFinalGuardRetries:      1,
		MaxTransientStreamRetries: 1,
	})

	_, err := client.Infer(context.Background(), nil, nil, nil, nil, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
	if providerCalls != 2 {
		t.Fatalf("provider calls = %d, want one original attempt and one retry", providerCalls)
	}
}

func TestInferDoesNotRetryAfterRequestContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	providerCalls := 0
	provider := fakeProvider{
		stream: func(context.Context, *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			providerCalls++
			return streamEvents(plugin.StreamEvent{Err: context.DeadlineExceeded}), nil
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{
		MaxTurns:                  3,
		MaxFinalGuardRetries:      1,
		MaxTransientStreamRetries: 1,
	})

	_, err := client.Infer(ctx, nil, nil, nil, nil, nil)
	if err == nil {
		t.Fatal("expected cancelled request to fail")
	}
	if providerCalls != 1 {
		t.Fatalf("provider calls = %d, want no retry after request cancellation", providerCalls)
	}
}

func TestServiceCallFieldsFromResultUsesServiceEnvelopeReferences(t *testing.T) {
	fields := serviceCallFieldsFromResult(`{"_serviceCall":{"serviceCallId":"svc-call-1","referenceId":"svc-ref-1","auditRef":"urn:quark:audit:service-call:svc-ref-1","traceId":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","subject":"svc.gateway.v1.embed"}}`)
	if fields["service_call_id"] != "svc-call-1" || fields["reference_id"] != "svc-ref-1" || fields["audit_ref"] == "" || fields["trace_id"] == "" || fields["subject"] != "svc.gateway.v1.embed" {
		t.Fatalf("service call fields = %+v", fields)
	}
	if fields := serviceCallFieldsFromResult(`{"answer":"ordinary tool result"}`); fields != nil {
		t.Fatalf("ordinary tool result produced service fields: %+v", fields)
	}
}

func TestInferNormalizesToolCallArgumentsBeforeTraceAndExecution(t *testing.T) {
	provider := fakeProvider{
		stream: func(context.Context, *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			return streamEvents(
				plugin.StreamEvent{ToolCalls: []plugin.ToolCall{{
					ID: "call-1",
					Function: plugin.ToolCallFunction{
						Name:      "indexer_UpsertChunk",
						Arguments: `{"chunkId":"chunk-1"}`,
					},
				}}},
				plugin.StreamEvent{Done: true},
			), nil
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{MaxTurns: 1, MaxFinalGuardRetries: 1})
	ctx := WithToolCallArgumentNormalizer(context.Background(), func(ctx context.Context, name, arguments string) (string, error) {
		if name != "indexer_UpsertChunk" {
			return arguments, nil
		}
		return `{"chunkId":"chunk-1","citations":[{"id":"cite-1"}]}`, nil
	})

	var tracedArguments string
	var executedArguments string
	_, err := client.Infer(
		ctx,
		[]plugin.Message{{Role: "user", Content: "index"}},
		[]plugin.ToolSchema{{Name: "indexer_UpsertChunk"}},
		func(ctx context.Context, name, arguments string) (string, error) {
			executedArguments = arguments
			return `{}`, nil
		},
		func(kind string, data any) {
			if kind != "tool_start" {
				return
			}
			payload := data.(map[string]any)
			tracedArguments = payload["arguments"].(string)
		},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "exceeded 1 model turns") {
		t.Fatalf("expected bounded loop error after tool result, got %v", err)
	}
	for label, got := range map[string]string{"trace": tracedArguments, "execution": executedArguments} {
		if !strings.Contains(got, `"citations"`) {
			t.Fatalf("%s arguments were not normalized: %s", label, got)
		}
	}
}

func TestInferRetriesArgumentNormalizationFailuresAsStructuredValidation(t *testing.T) {
	providerCalls := 0
	var retryInstruction string
	provider := fakeProvider{
		stream: func(_ context.Context, req *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			providerCalls++
			switch providerCalls {
			case 1:
				return streamEvents(
					plugin.StreamEvent{ToolCalls: []plugin.ToolCall{{
						ID: "bad-inputs",
						Function: plugin.ToolCallFunction{
							Name:      "gateway_Embed",
							Arguments: `{"inputs":"[]"}`,
						},
					}}},
					plugin.StreamEvent{Done: true},
				), nil
			case 2:
				retryInstruction = req.Messages[len(req.Messages)-1].Content
				return streamEvents(
					plugin.StreamEvent{ToolCalls: []plugin.ToolCall{{
						ID: "valid-inputs",
						Function: plugin.ToolCallFunction{
							Name:      "gateway_Embed",
							Arguments: `{"inputs":[]}`,
						},
					}}},
					plugin.StreamEvent{Done: true},
				), nil
			case 3:
				return streamEvents(plugin.StreamEvent{Delta: "done"}, plugin.StreamEvent{Done: true}), nil
			default:
				return nil, fmt.Errorf("unexpected provider call %d", providerCalls)
			}
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{MaxTurns: 3, MaxFinalGuardRetries: 1})
	ctx := WithToolCallArgumentNormalizer(context.Background(), func(_ context.Context, _ string, arguments string) (string, error) {
		if strings.Contains(arguments, `"inputs":"`) {
			return "", errors.New("inputs must be an array")
		}
		return arguments, nil
	})

	var executedArguments string
	result, err := client.Infer(
		ctx,
		[]plugin.Message{{Role: "user", Content: "embed"}},
		[]plugin.ToolSchema{{Name: "gateway_Embed"}},
		func(_ context.Context, _ string, arguments string) (string, error) {
			executedArguments = arguments
			return `{}`, nil
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if result != "done" || executedArguments != `{"inputs":[]}` {
		t.Fatalf("result = %q executed arguments = %q", result, executedArguments)
	}
	if !strings.Contains(retryInstruction, `"reason":"invalid_arguments"`) ||
		!strings.Contains(retryInstruction, `"function":"gateway_Embed"`) {
		t.Fatalf("retry instruction = %q", retryInstruction)
	}
	if !strings.Contains(retryInstruction, "inputs must be an array") {
		t.Fatalf("retry instruction did not tell the model how to repair the request: %q", retryInstruction)
	}
}

func TestInferRedactsArgumentValidationFeedback(t *testing.T) {
	secret := "sk-or-v1-test-validation-secret"
	t.Setenv("OPENROUTER_API_KEY", secret)
	providerCalls := 0
	var retryInstruction string
	provider := fakeProvider{
		stream: func(_ context.Context, req *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			providerCalls++
			if providerCalls == 1 {
				return streamEvents(
					plugin.StreamEvent{ToolCalls: []plugin.ToolCall{{
						ID:       "bad-input",
						Function: plugin.ToolCallFunction{Name: "gateway_Embed", Arguments: `{}`},
					}}},
					plugin.StreamEvent{Done: true},
				), nil
			}
			retryInstruction = req.Messages[len(req.Messages)-1].Content
			return streamEvents(plugin.StreamEvent{Delta: "done"}, plugin.StreamEvent{Done: true}), nil
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{MaxTurns: 2, MaxFinalGuardRetries: 1})
	ctx := WithToolCallArgumentNormalizer(context.Background(), func(context.Context, string, string) (string, error) {
		return "", fmt.Errorf("authorization bearer %s is invalid", secret)
	})
	_, err := client.Infer(ctx, []plugin.Message{{Role: "user", Content: "embed"}}, []plugin.ToolSchema{{Name: "gateway_Embed"}}, func(context.Context, string, string) (string, error) {
		t.Fatal("invalid tool call executed")
		return "", nil
	}, nil, nil)
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if strings.Contains(retryInstruction, secret) || !strings.Contains(retryInstruction, "[redacted]") {
		t.Fatalf("validation feedback was not redacted: %q", retryInstruction)
	}
}

func TestInferBoundsRepeatedArgumentNormalizationFailures(t *testing.T) {
	providerCalls := 0
	provider := fakeProvider{
		stream: func(context.Context, *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			providerCalls++
			return streamEvents(
				plugin.StreamEvent{ToolCalls: []plugin.ToolCall{{
					ID: "bad-inputs",
					Function: plugin.ToolCallFunction{
						Name:      "gateway_Embed",
						Arguments: `{"inputs":"[]"}`,
					},
				}}},
				plugin.StreamEvent{Done: true},
			), nil
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{MaxTurns: 4, MaxFinalGuardRetries: 1})
	ctx := WithToolCallArgumentNormalizer(context.Background(), func(_ context.Context, _ string, _ string) (string, error) {
		return "", errors.New("invalid arguments")
	})

	_, err := client.Infer(
		ctx,
		[]plugin.Message{{Role: "user", Content: "embed"}},
		[]plugin.ToolSchema{{Name: "gateway_Embed"}},
		func(context.Context, string, string) (string, error) {
			t.Fatal("invalid normalized tool call should not execute")
			return "", nil
		},
		nil,
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "tool-call argument validation exceeded 1 retries") {
		t.Fatalf("expected bounded argument-validation error, got %v", err)
	}
	if providerCalls != 2 {
		t.Fatalf("provider calls = %d, want 2", providerCalls)
	}
}

func TestInferWithToolCallGateAcceptsCompletedWorkflowContent(t *testing.T) {
	provider := fakeProvider{
		stream: func(context.Context, *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			return streamEvents(
				plugin.StreamEvent{Delta: "Indexing is complete."},
				plugin.StreamEvent{
					ToolCalls: []plugin.ToolCall{{
						Index: 0,
						ID:    "call-1",
						Type:  "function",
						Function: plugin.ToolCallFunction{
							Name:      "runstate_MarkComplete",
							Arguments: `{"runId":"run-1"}`,
						},
					}},
				},
				plugin.StreamEvent{Done: true},
			), nil
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{MaxTurns: 3, MaxFinalGuardRetries: 1})

	var toolCalled bool
	result, err := client.InferWithToolCallGate(
		context.Background(),
		[]plugin.Message{{Role: "user", Content: "index documents"}},
		[]plugin.ToolSchema{{Name: "runstate_MarkComplete"}},
		func(context.Context, string, string) (string, error) {
			toolCalled = true
			return `{}`, nil
		},
		nil,
		nil,
		func(content string, toolCalls []plugin.ToolCall) bool {
			return strings.Contains(content, "complete") && len(toolCalls) == 1 && toolCalls[0].Function.Name == "runstate_MarkComplete"
		},
	)
	if err != nil {
		t.Fatalf("InferWithToolCallGate returned error: %v", err)
	}
	if toolCalled {
		t.Fatal("redundant tool call executed after completion gate accepted final content")
	}
	if result != "Indexing is complete." {
		t.Fatalf("result = %q", result)
	}
}

func TestInferWithToolCallGuardRetriesBeforeExecutingNonAdvancingCalls(t *testing.T) {
	providerCalls := 0
	var guardPromptSeen bool
	provider := fakeProvider{
		stream: func(_ context.Context, req *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			providerCalls++
			switch providerCalls {
			case 1:
				return streamEvents(
					plugin.StreamEvent{Delta: "The batch is indexed."},
					plugin.StreamEvent{
						ToolCalls: []plugin.ToolCall{{
							Index: 0,
							ID:    "call-1",
							Type:  "function",
							Function: plugin.ToolCallFunction{
								Name:      "runstate_UpdateItemState",
								Arguments: `{}`,
							},
						}},
					},
					plugin.StreamEvent{Done: true},
				), nil
			case 2:
				if len(req.Messages) == 0 {
					return nil, errors.New("retry request has no messages")
				}
				last := req.Messages[len(req.Messages)-1]
				guardPromptSeen = last.Role == "system" && strings.Contains(last.Content, "runstate_MarkComplete")
				return streamEvents(
					plugin.StreamEvent{Delta: "The durable run is now complete."},
					plugin.StreamEvent{Done: true},
				), nil
			default:
				return nil, fmt.Errorf("unexpected provider call %d", providerCalls)
			}
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{MaxTurns: 3, MaxFinalGuardRetries: 1})

	result, err := client.InferWithToolCallPolicy(
		context.Background(),
		[]plugin.Message{{Role: "user", Content: "index documents"}},
		[]plugin.ToolSchema{{Name: "runstate_UpdateItemState", Description: "progress"}},
		func(context.Context, string, string) (string, error) {
			t.Fatal("non-advancing tool call should not execute after guard retry")
			return "", nil
		},
		nil,
		nil,
		nil,
		func(content string, toolCalls []plugin.ToolCall) (string, bool) {
			if !strings.Contains(content, "indexed") || len(toolCalls) != 1 {
				return "", false
			}
			return "Call runstate_MarkComplete before finalizing.", true
		},
		nil,
	)
	if err != nil {
		t.Fatalf("InferWithToolCallPolicy returned error: %v", err)
	}
	if !guardPromptSeen {
		t.Fatal("tool-call guard retry instruction was not sent")
	}
	if result != "The durable run is now complete." {
		t.Fatalf("result = %q", result)
	}
}

func TestInferWithToolResultGateAcceptsTerminalWorkflowContentAfterToolExecution(t *testing.T) {
	providerCalls := 0
	provider := fakeProvider{
		stream: func(context.Context, *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			providerCalls++
			if providerCalls > 1 {
				return nil, fmt.Errorf("unexpected provider call %d after terminal workflow tool", providerCalls)
			}
			return streamEvents(
				plugin.StreamEvent{Delta: "All documents are indexed and ready for questions."},
				plugin.StreamEvent{
					ToolCalls: []plugin.ToolCall{{
						Index: 0,
						ID:    "call-1",
						Type:  "function",
						Function: plugin.ToolCallFunction{
							Name:      "runstate_MarkComplete",
							Arguments: `{"runId":"run-1"}`,
						},
					}},
				},
				plugin.StreamEvent{Done: true},
			), nil
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{MaxTurns: 3, MaxFinalGuardRetries: 1})

	var toolCalled bool
	result, err := client.InferWithToolCallPolicy(
		context.Background(),
		[]plugin.Message{{Role: "user", Content: "index documents"}},
		[]plugin.ToolSchema{{Name: "runstate_MarkComplete"}},
		func(context.Context, string, string) (string, error) {
			toolCalled = true
			return `{"success":true}`, nil
		},
		nil,
		nil,
		nil,
		nil,
		func(content string, toolCalls []plugin.ToolCall) bool {
			return strings.Contains(content, "ready for questions") &&
				len(toolCalls) == 1 &&
				toolCalls[0].Function.Name == "runstate_MarkComplete"
		},
	)
	if err != nil {
		t.Fatalf("InferWithToolCallPolicy returned error: %v", err)
	}
	if !toolCalled {
		t.Fatal("terminal workflow tool was not executed")
	}
	if providerCalls != 1 {
		t.Fatalf("providerCalls = %d, want 1", providerCalls)
	}
	if result != "All documents are indexed and ready for questions." {
		t.Fatalf("result = %q", result)
	}
}

func TestInferStreamsOnlyFinalUserFacingContentAfterToolTurns(t *testing.T) {
	providerCalls := 0
	provider := fakeProvider{
		stream: func(context.Context, *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			providerCalls++
			switch providerCalls {
			case 1:
				return streamEvents(
					plugin.StreamEvent{Delta: "I will search now."},
					plugin.StreamEvent{ToolCalls: []plugin.ToolCall{{
						Index: 0,
						ID:    "call-1",
						Type:  "function",
						Function: plugin.ToolCallFunction{
							Name:      "indexer_QueryContext",
							Arguments: `{}`,
						},
					}}},
					plugin.StreamEvent{Done: true},
				), nil
			case 2:
				return streamEvents(
					plugin.StreamEvent{Delta: "The indexed answer is ready."},
					plugin.StreamEvent{Done: true},
				), nil
			default:
				return nil, fmt.Errorf("unexpected provider call %d", providerCalls)
			}
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{MaxTurns: 3, MaxFinalGuardRetries: 1})

	var streamed strings.Builder
	result, err := client.Infer(
		context.Background(),
		[]plugin.Message{{Role: "user", Content: "search"}},
		[]plugin.ToolSchema{{Name: "indexer_QueryContext"}},
		func(context.Context, string, string) (string, error) {
			return `{"chunks":[{"text":"evidence"}]}`, nil
		},
		func(kind string, value any) {
			if kind == "token" {
				streamed.WriteString(value.(string))
			}
		},
		nil,
	)
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if result != "The indexed answer is ready." {
		t.Fatalf("result = %q", result)
	}
	if got := streamed.String(); got != result {
		t.Fatalf("streamed content = %q, want only final answer %q", got, result)
	}
}

func TestInferAppendsToolResultContinuationInstruction(t *testing.T) {
	providerCalls := 0
	var continuationSeen bool
	provider := fakeProvider{
		stream: func(_ context.Context, req *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			providerCalls++
			switch providerCalls {
			case 1:
				return streamEvents(
					plugin.StreamEvent{
						ToolCalls: []plugin.ToolCall{{
							Index: 0,
							ID:    "call-1",
							Type:  "function",
							Function: plugin.ToolCallFunction{
								Name:      "gateway_Embed",
								Arguments: `{}`,
							},
						}},
					},
					plugin.StreamEvent{Done: true},
				), nil
			case 2:
				if len(req.Messages) == 0 {
					return nil, errors.New("continuation request has no messages")
				}
				last := req.Messages[len(req.Messages)-1]
				continuationSeen = last.Role == "system" && strings.Contains(last.Content, `"type":"runtime.workflow.status"`)
				return streamEvents(
					plugin.StreamEvent{Delta: "continued"},
					plugin.StreamEvent{Done: true},
				), nil
			default:
				return nil, fmt.Errorf("unexpected provider call %d", providerCalls)
			}
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{MaxTurns: 3, MaxFinalGuardRetries: 1})

	result, err := client.InferWithToolCallPolicyAndContinuation(
		context.Background(),
		[]plugin.Message{{Role: "user", Content: "index documents"}},
		[]plugin.ToolSchema{{Name: "gateway_Embed"}},
		func(context.Context, string, string) (string, error) {
			return `{"success":true}`, nil
		},
		nil,
		nil,
		nil,
		nil,
		nil,
		func() string { return `{"type":"runtime.workflow.status","status":"incomplete"}` },
	)
	if err != nil {
		t.Fatalf("InferWithToolCallPolicyAndContinuation returned error: %v", err)
	}
	if !continuationSeen {
		t.Fatal("continuation instruction was not sent after tool result")
	}
	if result != "continued" {
		t.Fatalf("result = %q", result)
	}
}

func TestInferRemovesSupersededRuntimeValidationAndStatusFactsAfterProgress(t *testing.T) {
	providerCalls := 0
	var finalMessages []plugin.Message
	provider := fakeProvider{
		stream: func(_ context.Context, req *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			providerCalls++
			switch providerCalls {
			case 1, 2:
				return streamEvents(
					plugin.StreamEvent{ToolCalls: []plugin.ToolCall{{
						ID: "call-1",
						Function: plugin.ToolCallFunction{
							Name:      "gateway_Embed",
							Arguments: `{}`,
						},
					}}},
					plugin.StreamEvent{Done: true},
				), nil
			case 3:
				finalMessages = append([]plugin.Message(nil), req.Messages...)
				return streamEvents(plugin.StreamEvent{Delta: "done"}, plugin.StreamEvent{Done: true}), nil
			default:
				return nil, fmt.Errorf("unexpected provider call %d", providerCalls)
			}
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{MaxTurns: 4, MaxFinalGuardRetries: 2})
	guardCalls := 0
	var progressReason string

	result, err := client.InferWithToolCallPolicyAndContinuation(
		context.Background(),
		[]plugin.Message{
			{Role: "system", Content: "installed agent prompt material"},
			{Role: "user", Content: "index this"},
			{Role: "system", Content: `{"type":"runtime.workflow.status","status":"old"}`},
		},
		[]plugin.ToolSchema{{Name: "gateway_Embed"}},
		func(context.Context, string, string) (string, error) { return `{"success":true}`, nil },
		func(kind string, data any) {
			if kind == "progress" {
				progressReason = data.(map[string]string)["reason"]
			}
		},
		nil,
		nil,
		func(string, []plugin.ToolCall) (string, bool) {
			guardCalls++
			if guardCalls == 1 {
				return `{"type":"runtime.workflow.validation","status":"blocked","reason":"incomplete_canonical_record"}`, true
			}
			return "", false
		},
		nil,
		func() string { return `{"type":"runtime.workflow.status","status":"current"}` },
	)
	if err != nil {
		t.Fatalf("InferWithToolCallPolicyAndContinuation returned error: %v", err)
	}
	if result != "done" {
		t.Fatalf("result = %q", result)
	}
	if progressReason != "incomplete_canonical_record" {
		t.Fatalf("progress reason = %q", progressReason)
	}
	var promptFound, currentStatusFound, staleFactFound, pairedHistoryFound bool
	for _, message := range finalMessages {
		switch {
		case message.Role == "system" && message.Content == "installed agent prompt material":
			promptFound = true
		case message.Role == "system" && strings.Contains(message.Content, `"status":"current"`):
			currentStatusFound = true
		case message.Role == "system" && (strings.Contains(message.Content, `"status":"old"`) || strings.Contains(message.Content, `"type":"runtime.workflow.validation"`)):
			staleFactFound = true
		case message.Role == "assistant" && len(message.ToolCalls) == 1:
			pairedHistoryFound = true
		}
	}
	if !promptFound || !currentStatusFound || staleFactFound || !pairedHistoryFound {
		t.Fatalf("final model messages did not preserve only current state and tool history: %#v", finalMessages)
	}
}

func TestInferRetainsOnlyCurrentWorkflowCorrectionDuringRejectedTurns(t *testing.T) {
	providerCalls := 0
	var retryMessages []plugin.Message
	provider := fakeProvider{
		stream: func(_ context.Context, req *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			providerCalls++
			switch providerCalls {
			case 1, 2:
				return streamEvents(
					plugin.StreamEvent{Delta: "unaccepted narration"},
					plugin.StreamEvent{ToolCalls: []plugin.ToolCall{{
						ID:       "call-retry",
						Function: plugin.ToolCallFunction{Name: "indexer_UpsertChunk", Arguments: `{}`},
					}}},
					plugin.StreamEvent{Done: true},
				), nil
			case 3:
				retryMessages = append([]plugin.Message(nil), req.Messages...)
				return streamEvents(plugin.StreamEvent{Delta: "done"}, plugin.StreamEvent{Done: true}), nil
			default:
				return nil, fmt.Errorf("unexpected provider call %d", providerCalls)
			}
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{MaxTurns: 3, MaxFinalGuardRetries: 2})
	guardCalls := 0
	result, err := client.InferWithToolCallPolicyAndContinuation(
		context.Background(),
		[]plugin.Message{{Role: "user", Content: "index source"}},
		[]plugin.ToolSchema{{Name: "indexer_UpsertChunk"}},
		func(context.Context, string, string) (string, error) { return `{"success":true}`, nil },
		nil,
		nil,
		nil,
		func(string, []plugin.ToolCall) (string, bool) {
			guardCalls++
			return fmt.Sprintf(`{"type":"runtime.workflow.validation","status":"blocked","reason":"invalid_chunk_%d"}`, guardCalls), true
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("InferWithToolCallPolicyAndContinuation returned error: %v", err)
	}
	if result != "done" {
		t.Fatalf("result = %q", result)
	}
	validationFacts := 0
	for _, message := range retryMessages {
		if message.Role == "assistant" && strings.Contains(message.Content, "unaccepted narration") {
			t.Fatalf("rejected assistant narration retained in retry context: %#v", retryMessages)
		}
		if message.Role == "system" && strings.Contains(message.Content, `"type":"runtime.workflow.validation"`) {
			validationFacts++
			if !strings.Contains(message.Content, `"reason":"invalid_chunk_2"`) {
				t.Fatalf("stale correction retained in retry context: %#v", retryMessages)
			}
		}
	}
	if validationFacts != 1 {
		t.Fatalf("validation facts = %d, want exactly one current correction: %#v", validationFacts, retryMessages)
	}
}

func TestInferAppliesToolSurfaceForEachModelTurn(t *testing.T) {
	providerCalls := 0
	var seen [][]string
	provider := fakeProvider{
		stream: func(_ context.Context, req *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			names := make([]string, 0, len(req.Tools))
			for _, tool := range req.Tools {
				names = append(names, tool.Name)
			}
			seen = append(seen, names)
			providerCalls++
			if providerCalls == 1 {
				return streamEvents(
					plugin.StreamEvent{ToolCalls: []plugin.ToolCall{{
						ID:       "call-1",
						Function: plugin.ToolCallFunction{Name: "step_one", Arguments: `{}`},
					}}},
					plugin.StreamEvent{Done: true},
				), nil
			}
			return streamEvents(plugin.StreamEvent{Delta: "done"}, plugin.StreamEvent{Done: true}), nil
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{MaxTurns: 3, MaxFinalGuardRetries: 1})
	surfaceCall := 0
	result, err := client.InferWithPreparedContextAndToolSurface(
		context.Background(),
		[]plugin.Message{{Role: "user", Content: "run"}},
		[]plugin.ToolSchema{{Name: "step_one"}, {Name: "step_two"}},
		func(context.Context, string, string) (string, error) { return `{"success":true}`, nil },
		nil,
		nil,
		func(tools []plugin.ToolSchema) []plugin.ToolSchema {
			surfaceCall++
			if surfaceCall == 1 {
				return tools[:1]
			}
			return tools[1:]
		},
		nil, nil, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("InferWithPreparedContextAndToolSurface returned error: %v", err)
	}
	if result != "done" {
		t.Fatalf("result = %q", result)
	}
	if len(seen) != 2 || len(seen[0]) != 1 || seen[0][0] != "step_one" ||
		len(seen[1]) != 1 || seen[1][0] != "step_two" {
		t.Fatalf("turn tool surfaces = %v", seen)
	}
}

func TestInferDoesNotParseTextToolCallsWhenSurfaceIsClosed(t *testing.T) {
	providerCalls := 0
	parseCalls := 0
	executed := 0
	provider := fakeProvider{
		stream: func(_ context.Context, _ *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			providerCalls++
			if providerCalls == 1 {
				return streamEvents(plugin.StreamEvent{ToolCalls: []plugin.ToolCall{{
					ID: "terminal", Type: "function", Function: plugin.ToolCallFunction{Name: "runstate_MarkComplete", Arguments: `{}`},
				}}}, plugin.StreamEvent{Done: true}), nil
			}
			return streamEvents(plugin.StreamEvent{Delta: "Indexed documents are ready."}, plugin.StreamEvent{Done: true}), nil
		},
		parse: func(content string) ([]plugin.ToolCall, string) {
			parseCalls++
			return []plugin.ToolCall{{Function: plugin.ToolCallFunction{Name: "invented_Persist", Arguments: `{}`}}}, ""
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{MaxTurns: 3, MaxFinalGuardRetries: 1})
	surfaceCalls := 0
	result, err := client.InferWithPreparedContextAndToolSurface(
		context.Background(),
		[]plugin.Message{{Role: "user", Content: "index"}},
		[]plugin.ToolSchema{{Name: "runstate_MarkComplete"}},
		func(context.Context, string, string) (string, error) {
			executed++
			return `{"success":true}`, nil
		},
		nil,
		nil,
		func(tools []plugin.ToolSchema) []plugin.ToolSchema {
			surfaceCalls++
			if surfaceCalls == 1 {
				return tools
			}
			return nil
		},
		nil, nil, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("InferWithPreparedContextAndToolSurface returned error: %v", err)
	}
	if result != "Indexed documents are ready." || executed != 1 {
		t.Fatalf("result = %q executed = %d", result, executed)
	}
	if parseCalls != 0 {
		t.Fatalf("text parser called %d times after answer-only surface was established", parseCalls)
	}
}

func TestInferExecutesRequiredToolContinuationBeforeNextModelTurn(t *testing.T) {
	providerCalls := 0
	indexed := false
	continuationIssued := false
	var executed []string
	var continuationProgress map[string]string
	provider := fakeProvider{
		stream: func(_ context.Context, _ *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			providerCalls++
			if providerCalls == 1 {
				return streamEvents(plugin.StreamEvent{ToolCalls: []plugin.ToolCall{{
					ID: "chunk", Type: "function", Function: plugin.ToolCallFunction{Name: "indexer_UpsertChunk", Arguments: `{}`},
				}}}, plugin.StreamEvent{Done: true}), nil
			}
			return streamEvents(plugin.StreamEvent{Delta: "Ready."}, plugin.StreamEvent{Done: true}), nil
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{MaxTurns: 2, MaxFinalGuardRetries: 1})
	result, err := client.InferWithPreparedContextAndToolContinuation(
		context.Background(),
		[]plugin.Message{{Role: "user", Content: "index"}},
		[]plugin.ToolSchema{{Name: "indexer_UpsertChunk"}, {Name: "runstate_MarkComplete"}},
		func(_ context.Context, name, _ string) (string, error) {
			executed = append(executed, name)
			if name == "indexer_UpsertChunk" {
				indexed = true
			}
			return `{"success":true}`, nil
		},
		func(kind string, data any) {
			if kind == "progress" {
				event := data.(map[string]string)
				if event["state"] == "executing_required_action" {
					continuationProgress = event
				}
			}
		},
		nil,
		nil,
		func() []plugin.ToolCall {
			if !indexed || continuationIssued {
				return nil
			}
			continuationIssued = true
			return []plugin.ToolCall{{
				ID: "runtime-complete", Type: "function",
				Function: plugin.ToolCallFunction{Name: "runstate_MarkComplete", Arguments: `{"runId":"run-1"}`},
			}}
		},
		nil, nil, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("InferWithPreparedContextAndToolContinuation returned error: %v", err)
	}
	if result != "Ready." || providerCalls != 2 {
		t.Fatalf("result = %q provider calls = %d", result, providerCalls)
	}
	if strings.Join(executed, ",") != "indexer_UpsertChunk,runstate_MarkComplete" {
		t.Fatalf("executed = %v", executed)
	}
	if continuationProgress["function"] != "runstate_MarkComplete" ||
		!strings.Contains(continuationProgress["diagnostic"], "runtime.workflow") {
		t.Fatalf("continuation progress = %+v", continuationProgress)
	}
}

func TestInferRejectsToolCallsOutsideCurrentSurfaceBeforeExecution(t *testing.T) {
	providerCalls := 0
	executed := 0
	var rejectionReason string
	correctionIncludedAllowedFunction := false
	provider := fakeProvider{
		stream: func(_ context.Context, req *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			providerCalls++
			switch providerCalls {
			case 1:
				return streamEvents(plugin.StreamEvent{ToolCalls: []plugin.ToolCall{
					{Index: 0, ID: "allowed", Type: "function", Function: plugin.ToolCallFunction{Name: "step_one", Arguments: `{}`}},
					{Index: 1, ID: "hidden", Type: "function", Function: plugin.ToolCallFunction{Name: "step_two", Arguments: `{}`}},
				}}, plugin.StreamEvent{Done: true}), nil
			case 2:
				for _, message := range req.Messages {
					if message.Role == "system" &&
						strings.Contains(message.Content, `"reason":"function_not_exposed_this_turn"`) &&
						strings.Contains(message.Content, `"allowed_functions":["step_one"]`) {
						correctionIncludedAllowedFunction = true
					}
				}
				return streamEvents(plugin.StreamEvent{ToolCalls: []plugin.ToolCall{{
					ID: "allowed", Type: "function", Function: plugin.ToolCallFunction{Name: "step_one", Arguments: `{}`},
				}}}, plugin.StreamEvent{Done: true}), nil
			case 3:
				return streamEvents(plugin.StreamEvent{Delta: "done"}, plugin.StreamEvent{Done: true}), nil
			default:
				return nil, fmt.Errorf("unexpected provider call %d", providerCalls)
			}
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{MaxTurns: 4, MaxFinalGuardRetries: 2})
	result, err := client.InferWithPreparedContextAndToolSurface(
		context.Background(),
		[]plugin.Message{{Role: "user", Content: "run"}},
		[]plugin.ToolSchema{{Name: "step_one"}, {Name: "step_two"}},
		func(context.Context, string, string) (string, error) {
			executed++
			return `{"success":true}`, nil
		},
		func(kind string, data any) {
			if kind == "progress" {
				rejectionReason = data.(map[string]string)["reason"]
			}
		},
		nil,
		func(tools []plugin.ToolSchema) []plugin.ToolSchema { return tools[:1] },
		nil, nil, nil, nil, nil,
	)
	if err != nil {
		t.Fatalf("InferWithPreparedContextAndToolSurface returned error: %v", err)
	}
	if result != "done" || executed != 1 {
		t.Fatalf("result = %q executed = %d", result, executed)
	}
	if rejectionReason != "function_not_exposed_this_turn" {
		t.Fatalf("rejection reason = %q", rejectionReason)
	}
	if !correctionIncludedAllowedFunction {
		t.Fatal("surface rejection did not expose the callable function contract for the retry")
	}
}

func TestInferNormalizesToolCallsBeforeExecutionAndHistory(t *testing.T) {
	providerCalls := 0
	var historyErr error
	provider := fakeProvider{
		stream: func(_ context.Context, req *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			providerCalls++
			switch providerCalls {
			case 1:
				return streamEvents(
					plugin.StreamEvent{
						ToolCalls: []plugin.ToolCall{
							{
								Index: 0,
								Function: plugin.ToolCallFunction{
									Name:      " indexer_UpsertChunk ",
									Arguments: `{"chunkId":"chunk-1"}`,
								},
							},
							{
								Index: 1,
								ID:    "bad-call",
								Type:  "function",
								Function: plugin.ToolCallFunction{
									Arguments: `{}`,
								},
							},
							{
								Index: 2,
								ID:    "bad-args",
								Type:  "function",
								Function: plugin.ToolCallFunction{
									Name:      "indexer_UpsertChunk",
									Arguments: `{"chunkId":`,
								},
							},
						},
					},
					plugin.StreamEvent{Done: true},
				), nil
			case 2:
				historyErr = assertSingleValidToolCallHistory(req.Messages)
				return streamEvents(
					plugin.StreamEvent{Delta: "indexed"},
					plugin.StreamEvent{Done: true},
				), nil
			default:
				return nil, fmt.Errorf("unexpected provider call %d", providerCalls)
			}
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{MaxTurns: 3, MaxFinalGuardRetries: 1})

	var calledNames []string
	result, err := client.Infer(
		context.Background(),
		[]plugin.Message{{Role: "user", Content: "index"}},
		[]plugin.ToolSchema{{Name: "indexer_UpsertChunk"}},
		func(_ context.Context, name, arguments string) (string, error) {
			calledNames = append(calledNames, name)
			if arguments != `{"chunkId":"chunk-1"}` {
				t.Fatalf("arguments = %q", arguments)
			}
			return `{"ok":true}`, nil
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if historyErr != nil {
		t.Fatal(historyErr)
	}
	if result != "indexed" {
		t.Fatalf("result = %q", result)
	}
	if len(calledNames) != 1 || calledNames[0] != "indexer_UpsertChunk" {
		t.Fatalf("calledNames = %v", calledNames)
	}
}

func TestInferCompactsLargeExecutedToolArgumentsBeforeHistoryReplay(t *testing.T) {
	longText := strings.Repeat("source paragraph ", 700)
	providerCalls := 0
	var historyErr error
	provider := fakeProvider{
		stream: func(_ context.Context, req *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			providerCalls++
			switch providerCalls {
			case 1:
				return streamEvents(
					plugin.StreamEvent{ToolCalls: []plugin.ToolCall{{
						Index: 0,
						ID:    "call-1",
						Type:  "function",
						Function: plugin.ToolCallFunction{
							Name:      "indexer_UpsertChunk",
							Arguments: `{"chunkId":"chunk-1","textContent":"` + longText + `","embeddingRef":"emb_1","facts":[{"subject":"doc","predicate":"contains","object":"text"}]}`,
						},
					}}},
					plugin.StreamEvent{Done: true},
				), nil
			case 2:
				assistant := req.Messages[len(req.Messages)-2]
				if len(assistant.ToolCalls) != 1 {
					historyErr = fmt.Errorf("assistant tool calls = %+v", assistant.ToolCalls)
					return streamEvents(plugin.StreamEvent{Done: true}), nil
				}
				arguments := assistant.ToolCalls[0].Function.Arguments
				if strings.Contains(arguments, longText) {
					historyErr = fmt.Errorf("history replay contains full tool arguments")
					return streamEvents(plugin.StreamEvent{Done: true}), nil
				}
				var payload map[string]any
				if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
					historyErr = fmt.Errorf("history arguments are not valid JSON: %w", err)
					return streamEvents(plugin.StreamEvent{Done: true}), nil
				}
				if payload["textContentTruncated"] != true {
					historyErr = fmt.Errorf("history arguments were not marked truncated: %s", arguments)
					return streamEvents(plugin.StreamEvent{Done: true}), nil
				}
				return streamEvents(
					plugin.StreamEvent{Delta: "indexed"},
					plugin.StreamEvent{Done: true},
				), nil
			default:
				return nil, fmt.Errorf("unexpected provider call %d", providerCalls)
			}
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{MaxTurns: 3, MaxFinalGuardRetries: 1})

	var executedArguments string
	result, err := client.Infer(
		context.Background(),
		[]plugin.Message{{Role: "user", Content: "index"}},
		[]plugin.ToolSchema{{Name: "indexer_UpsertChunk"}},
		func(_ context.Context, _ string, arguments string) (string, error) {
			executedArguments = arguments
			return `{"success":true}`, nil
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if historyErr != nil {
		t.Fatal(historyErr)
	}
	if !strings.Contains(executedArguments, longText) {
		t.Fatal("tool execution did not receive full arguments")
	}
	if result != "indexed" {
		t.Fatalf("result = %q", result)
	}
}

func TestInferRetriesAfterOnlyMalformedToolCalls(t *testing.T) {
	providerCalls := 0
	var retryPromptSeen bool
	provider := fakeProvider{
		stream: func(_ context.Context, req *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			providerCalls++
			switch providerCalls {
			case 1:
				return streamEvents(
					plugin.StreamEvent{
						ToolCalls: []plugin.ToolCall{{
							Index: 0,
							ID:    "missing-name",
							Type:  "function",
							Function: plugin.ToolCallFunction{
								Arguments: `{}`,
							},
						}},
					},
					plugin.StreamEvent{Done: true},
				), nil
			case 2:
				if len(req.Messages) == 0 {
					return nil, errors.New("retry request has no messages")
				}
				last := req.Messages[len(req.Messages)-1]
				retryPromptSeen = last.Role == "system" && strings.Contains(last.Content, `"reason":"malformed_tool_calls"`)
				return streamEvents(
					plugin.StreamEvent{Delta: "answered without malformed history"},
					plugin.StreamEvent{Done: true},
				), nil
			default:
				return nil, fmt.Errorf("unexpected provider call %d", providerCalls)
			}
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{MaxTurns: 3, MaxFinalGuardRetries: 1})

	result, err := client.Infer(
		context.Background(),
		[]plugin.Message{{Role: "user", Content: "start"}},
		[]plugin.ToolSchema{{Name: "indexer_UpsertChunk"}},
		func(context.Context, string, string) (string, error) {
			t.Fatal("malformed tool call should not execute")
			return "", nil
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if !retryPromptSeen {
		t.Fatal("retry prompt for malformed tool call was not sent")
	}
	if result != "answered without malformed history" {
		t.Fatalf("result = %q", result)
	}
}

func TestInferRetriesAfterOnlyMalformedToolCallArguments(t *testing.T) {
	providerCalls := 0
	var retryPromptSeen bool
	provider := fakeProvider{
		stream: func(_ context.Context, req *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			providerCalls++
			switch providerCalls {
			case 1:
				return streamEvents(
					plugin.StreamEvent{
						ToolCalls: []plugin.ToolCall{{
							Index: 0,
							ID:    "bad-json",
							Type:  "function",
							Function: plugin.ToolCallFunction{
								Name:      "build_DryRunRelease",
								Arguments: `{"projectRoot":`,
							},
						}},
					},
					plugin.StreamEvent{Done: true},
				), nil
			case 2:
				if len(req.Messages) == 0 {
					return nil, errors.New("retry request has no messages")
				}
				last := req.Messages[len(req.Messages)-1]
				retryPromptSeen = last.Role == "system" && strings.Contains(last.Content, `"reason":"malformed_tool_calls"`)
				return streamEvents(
					plugin.StreamEvent{Delta: "retried without executing malformed call"},
					plugin.StreamEvent{Done: true},
				), nil
			default:
				return nil, fmt.Errorf("unexpected provider call %d", providerCalls)
			}
		},
	}
	client := NewClientWithLimits(provider, "test-model", 0, InferenceLimits{MaxTurns: 3, MaxFinalGuardRetries: 1})

	result, err := client.Infer(
		context.Background(),
		[]plugin.Message{{Role: "user", Content: "dry run"}},
		[]plugin.ToolSchema{{Name: "build_DryRunRelease"}},
		func(context.Context, string, string) (string, error) {
			t.Fatal("malformed tool call arguments should not execute")
			return "", nil
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if !retryPromptSeen {
		t.Fatal("retry prompt for malformed tool arguments was not sent")
	}
	if result != "retried without executing malformed call" {
		t.Fatalf("result = %q", result)
	}
}

func assertSingleValidToolCallHistory(messages []plugin.Message) error {
	if len(messages) != 3 {
		return fmt.Errorf("messages = %d, want user + assistant + tool", len(messages))
	}
	assistant := messages[1]
	if assistant.Role != "assistant" {
		return fmt.Errorf("assistant role = %q", assistant.Role)
	}
	if len(assistant.ToolCalls) != 1 {
		return fmt.Errorf("assistant tool calls = %+v, want one valid call", assistant.ToolCalls)
	}
	call := assistant.ToolCalls[0]
	if call.ID == "" {
		return errors.New("normalized tool call has empty id")
	}
	if call.Type != "function" {
		return fmt.Errorf("normalized tool call type = %q", call.Type)
	}
	if call.Index != 0 {
		return fmt.Errorf("normalized tool call index = %d", call.Index)
	}
	if call.Function.Name != "indexer_UpsertChunk" {
		return fmt.Errorf("normalized tool call name = %q", call.Function.Name)
	}

	toolMessage := messages[2]
	if toolMessage.Role != "tool" {
		return fmt.Errorf("tool message role = %q", toolMessage.Role)
	}
	if toolMessage.ToolCallID != call.ID {
		return fmt.Errorf("tool message id = %q, want %q", toolMessage.ToolCallID, call.ID)
	}
	return nil
}

func streamEvents(events ...plugin.StreamEvent) <-chan plugin.StreamEvent {
	ch := make(chan plugin.StreamEvent, len(events))
	for _, event := range events {
		ch <- event
	}
	close(ch)
	return ch
}
