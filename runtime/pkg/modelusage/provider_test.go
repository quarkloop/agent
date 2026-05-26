package modelusage

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/runtime/pkg/runcontext"
)

type fixtureProvider struct{}

func (fixtureProvider) ChatCompletionStream(context.Context, *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
	stream := make(chan plugin.StreamEvent, 2)
	stream <- plugin.StreamEvent{Delta: "private response content"}
	stream <- plugin.StreamEvent{Done: true, Usage: &plugin.StreamUsage{
		Provider:      "openrouter",
		Model:         "provider/model",
		InputTokens:   7,
		OutputTokens:  3,
		FallbackChain: []string{"openrouter"},
		RequestID:     "provider-request-1",
		FinishReason:  "stop",
	}}
	close(stream)
	return stream, nil
}

func (fixtureProvider) ParseToolCalls(content string) ([]plugin.ToolCall, string) {
	return nil, content
}

func TestObservePersistsOnlyGatewayReturnedUsage(t *testing.T) {
	var records []Usage
	provider := Observe(fixtureProvider{}, func(_ context.Context, usage Usage) {
		records = append(records, usage)
	})
	ctx := runcontext.WithRunID(runcontext.WithSessionID(context.Background(), "session-1"), "run-1")
	stream, err := provider.ChatCompletionStream(ctx, &plugin.ChatRequest{
		Model:    "provider/model",
		Messages: []plugin.Message{{Role: "user", Content: "private prompt content"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for range stream {
	}
	if len(records) != 1 {
		t.Fatalf("usage records = %+v", records)
	}
	if records[0].SessionID != "session-1" || records[0].RunID != "run-1" || records[0].InputTokens != 7 {
		t.Fatalf("usage record = %+v", records[0])
	}
	data, err := json.Marshal(records[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "private prompt") || strings.Contains(string(data), "private response") {
		t.Fatalf("usage leaked model content: %s", data)
	}
}
