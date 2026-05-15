package llm

import (
	"context"
	"testing"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/runtime/pkg/modelservice"
)

func TestRegistryLoadsEntriesThroughModelService(t *testing.T) {
	var usage []modelservice.Usage
	models := modelservice.New(map[string]Provider{
		"openrouter": fakeProvider{stream: func(context.Context, *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
			ch := make(chan plugin.StreamEvent, 2)
			ch <- plugin.StreamEvent{Delta: "ready"}
			ch <- plugin.StreamEvent{Done: true}
			close(ch)
			return ch, nil
		}},
	}, func(_ context.Context, record modelservice.Usage) {
		usage = append(usage, record)
	})

	registry := NewRegistry()
	err := registry.LoadEntriesWithModelService([]plugin.ModelEntry{
		{ID: "openai/gpt-test", Provider: "openrouter", Default: true, ContextWindow: 1024},
		{ID: "missing/model", Provider: "missing"},
	}, models)
	if err != nil {
		t.Fatalf("load entries: %v", err)
	}
	if registry.Default != "openai/gpt-test" {
		t.Fatalf("default model = %q", registry.Default)
	}

	client := registry.GetDefault()
	if client == nil {
		t.Fatal("default client is nil")
	}
	ctx := modelservice.WithSessionID(context.Background(), "session-1")
	got, err := client.Infer(ctx, []plugin.Message{{Role: "user", Content: "hello"}}, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("infer: %v", err)
	}
	if got != "ready" {
		t.Fatalf("response = %q", got)
	}
	if len(usage) != 1 || usage[0].Provider != "openrouter" || usage[0].SessionID != "session-1" {
		t.Fatalf("model usage = %+v", usage)
	}
}
