package modelservice

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/quarkloop/pkg/plugin"
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

func TestAdapterRecordsStreamingUsageWithoutPromptLeak(t *testing.T) {
	var records []Usage
	svc := New(map[string]plugin.Provider{
		"openrouter": fakeProvider{stream: streamEvents(
			plugin.StreamEvent{Delta: "hello"},
			plugin.StreamEvent{Delta: " world"},
			plugin.StreamEvent{Done: true},
		)},
	}, func(_ context.Context, usage Usage) {
		records = append(records, usage)
	})
	ctx := WithSessionID(context.Background(), "session-1")

	stream, err := svc.Adapter("openrouter").ChatCompletionStream(ctx, &plugin.ChatRequest{
		Model:    "openai/gpt-test",
		Messages: []plugin.Message{{Role: "user", Content: "private prompt secret"}},
		Tools: []plugin.ToolSchema{{
			Name:        "fs",
			Description: "filesystem",
			Parameters:  map[string]any{"type": "object"},
		}},
	})
	if err != nil {
		t.Fatalf("chat completion: %v", err)
	}
	var got strings.Builder
	for ev := range stream {
		got.WriteString(ev.Delta)
	}
	if got.String() != "hello world" {
		t.Fatalf("stream content = %q", got.String())
	}
	if len(records) != 1 {
		t.Fatalf("usage records = %+v, want one", records)
	}
	usage := records[0]
	if usage.SessionID != "session-1" || usage.Provider != "openrouter" || usage.Model != "openai/gpt-test" {
		t.Fatalf("unexpected usage identity: %+v", usage)
	}
	if usage.InputTokens == 0 || usage.OutputTokens == 0 || usage.FinishReason != "stop" {
		t.Fatalf("unexpected usage accounting: %+v", usage)
	}
	data, err := json.Marshal(usage)
	if err != nil {
		t.Fatalf("marshal usage: %v", err)
	}
	if strings.Contains(string(data), "private prompt secret") || strings.Contains(string(data), "hello world") {
		t.Fatalf("usage leaked prompt or response text: %s", data)
	}
}

func TestAdapterRecordsFallbackUsage(t *testing.T) {
	primaryErr := plugin.NewProviderError(plugin.ProviderErrorRateLimit, "primary", "model-a", 429, errors.New("primary quota exhausted"))
	primaryErr.ResetAt = "2026-05-15T12:00:00Z"
	var records []Usage
	svc := New(map[string]plugin.Provider{
		"primary":  fakeProvider{stream: func(context.Context, *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) { return nil, primaryErr }},
		"fallback": fakeProvider{stream: streamEvents(plugin.StreamEvent{Delta: "ok"}, plugin.StreamEvent{Done: true})},
	}, func(_ context.Context, usage Usage) {
		records = append(records, usage)
	}, WithFallbacks(map[string][]string{"primary": {"fallback"}}))

	stream, err := svc.Adapter("primary").ChatCompletionStream(context.Background(), &plugin.ChatRequest{Model: "model-a"})
	if err != nil {
		t.Fatalf("fallback chat completion: %v", err)
	}
	for range stream {
	}
	if len(records) != 2 {
		t.Fatalf("usage records = %+v, want primary failure and fallback success", records)
	}
	if records[0].Provider != "primary" || records[0].FinishReason != "error" {
		t.Fatalf("primary usage = %+v", records[0])
	}
	if records[0].FailureCategory != string(plugin.ProviderErrorRateLimit) || records[0].FailureResetAt != "2026-05-15T12:00:00Z" {
		t.Fatalf("primary failure diagnostics = %+v", records[0])
	}
	if records[1].Provider != "fallback" || records[1].FinishReason != "stop" {
		t.Fatalf("fallback usage = %+v", records[1])
	}
	if got := strings.Join(records[1].FallbackChain, ","); got != "primary,fallback" {
		t.Fatalf("fallback chain = %q", got)
	}
}

func TestAdapterRetriesTransientTransportErrors(t *testing.T) {
	transient := plugin.NewProviderError(plugin.ProviderErrorTransport, "openrouter", "model-a", 502, errors.New("upstream 502"))
	var calls int
	var records []Usage
	svc := New(map[string]plugin.Provider{
		"openrouter": fakeProvider{stream: func(context.Context, *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			calls++
			if calls == 1 {
				return nil, transient
			}
			return streamEvents(plugin.StreamEvent{Delta: "ok"}, plugin.StreamEvent{Done: true})(context.Background(), nil)
		}},
	}, func(_ context.Context, usage Usage) {
		records = append(records, usage)
	}, WithRetryPolicy(RetryPolicy{MaxAttempts: 2}))

	stream, err := svc.Adapter("openrouter").ChatCompletionStream(context.Background(), &plugin.ChatRequest{Model: "model-a"})
	if err != nil {
		t.Fatalf("chat completion after retry: %v", err)
	}
	for range stream {
	}
	if calls != 2 {
		t.Fatalf("provider calls = %d, want 2", calls)
	}
	if len(records) != 2 {
		t.Fatalf("usage records = %+v, want failure and success", records)
	}
	if records[0].FailureCategory != string(plugin.ProviderErrorTransport) || records[0].FinishReason != "error" {
		t.Fatalf("retry failure usage = %+v", records[0])
	}
	if records[1].Provider != "openrouter" || records[1].FinishReason != "stop" {
		t.Fatalf("retry success usage = %+v", records[1])
	}
}

func TestAdapterDoesNotFallbackForInvalidRequest(t *testing.T) {
	primaryErr := plugin.NewProviderError(plugin.ProviderErrorInvalidRequest, "primary", "model-a", 400, errors.New("bad request"))
	var records []Usage
	svc := New(map[string]plugin.Provider{
		"primary":  fakeProvider{stream: func(context.Context, *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) { return nil, primaryErr }},
		"fallback": fakeProvider{stream: streamEvents(plugin.StreamEvent{Delta: "should not run"}, plugin.StreamEvent{Done: true})},
	}, func(_ context.Context, usage Usage) {
		records = append(records, usage)
	}, WithFallbacks(map[string][]string{"primary": {"fallback"}}))

	_, err := svc.Adapter("primary").ChatCompletionStream(context.Background(), &plugin.ChatRequest{Model: "model-a"})
	if !errors.Is(err, primaryErr) {
		t.Fatalf("expected terminal primary error, got %v", err)
	}
	if len(records) != 1 || records[0].Provider != "primary" || records[0].FailureCategory != string(plugin.ProviderErrorInvalidRequest) {
		t.Fatalf("records = %+v", records)
	}
}

func TestAdapterReturnsExhaustedProviderError(t *testing.T) {
	svc := New(map[string]plugin.Provider{}, nil, WithFallbacks(map[string][]string{"missing": {"also-missing"}}))
	_, err := svc.Adapter("missing").ChatCompletionStream(context.Background(), &plugin.ChatRequest{Model: "model-a"})
	var providerErr *plugin.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected provider error, got %v", err)
	}
	if providerErr.Category != plugin.ProviderErrorExhausted {
		t.Fatalf("category = %s, want %s", providerErr.Category, plugin.ProviderErrorExhausted)
	}
}

func TestAdapterReturnsProviderErrorsAndRecordsUsage(t *testing.T) {
	want := errors.New("provider down")
	var records []Usage
	svc := New(map[string]plugin.Provider{
		"openai": fakeProvider{stream: func(context.Context, *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) { return nil, want }},
	}, func(_ context.Context, usage Usage) {
		records = append(records, usage)
	})

	_, err := svc.Adapter("openai").ChatCompletionStream(context.Background(), &plugin.ChatRequest{Model: "gpt-test"})
	if err == nil || !strings.Contains(err.Error(), "provider down") {
		t.Fatalf("expected provider error, got %v", err)
	}
	if len(records) != 1 || records[0].Provider != "openai" || records[0].FinishReason != "error" {
		t.Fatalf("provider error usage = %+v", records)
	}
}

func TestAdapterRecordsStreamErrorUsage(t *testing.T) {
	var records []Usage
	svc := New(map[string]plugin.Provider{
		"anthropic": fakeProvider{stream: streamEvents(plugin.StreamEvent{Err: errors.New("stream reset")})},
	}, func(_ context.Context, usage Usage) {
		records = append(records, usage)
	})

	stream, err := svc.Adapter("anthropic").ChatCompletionStream(context.Background(), &plugin.ChatRequest{Model: "claude-test"})
	if err != nil {
		t.Fatalf("chat completion: %v", err)
	}
	var sawStreamErr bool
	for ev := range stream {
		sawStreamErr = sawStreamErr || ev.Err != nil
	}
	if !sawStreamErr {
		t.Fatal("expected stream error event")
	}
	if len(records) != 1 || records[0].FinishReason != "error" {
		t.Fatalf("stream error usage = %+v", records)
	}
}

func streamEvents(events ...plugin.StreamEvent) func(context.Context, *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
	return func(context.Context, *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
		ch := make(chan plugin.StreamEvent, len(events))
		for _, ev := range events {
			ch <- ev
		}
		close(ch)
		return ch, nil
	}
}
