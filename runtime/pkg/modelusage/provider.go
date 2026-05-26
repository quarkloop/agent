package modelusage

import (
	"context"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/runtime/pkg/runcontext"
)

type Sink func(context.Context, Usage)

// Observe returns an adapter that forwards Gateway output unchanged and emits
// the accounting payload supplied by Gateway. Policy and retry remain in
// Gateway; this adapter never synthesizes or reroutes a model operation.
func Observe(provider plugin.Provider, sink Sink) plugin.Provider {
	if provider == nil || sink == nil {
		return provider
	}
	return &observer{provider: provider, sink: sink}
}

func ObserveProviders(providers map[string]plugin.Provider, sink Sink) map[string]plugin.Provider {
	out := make(map[string]plugin.Provider, len(providers))
	for id, provider := range providers {
		out[id] = Observe(provider, sink)
	}
	return out
}

type observer struct {
	provider plugin.Provider
	sink     Sink
}

func (o *observer) ChatCompletionStream(ctx context.Context, req *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
	stream, err := o.provider.ChatCompletionStream(ctx, req)
	if err != nil {
		return nil, err
	}
	out := make(chan plugin.StreamEvent)
	go func() {
		defer close(out)
		recorded := false
		for event := range stream {
			if !recorded && event.Usage != nil {
				recorded = true
				o.sink(ctx, fromStreamUsage(ctx, *event.Usage))
			}
			out <- event
		}
	}()
	return out, nil
}

func (o *observer) ParseToolCalls(content string) ([]plugin.ToolCall, string) {
	return o.provider.ParseToolCalls(content)
}

func fromStreamUsage(ctx context.Context, usage plugin.StreamUsage) Usage {
	return Usage{
		SessionID:       runcontext.SessionID(ctx),
		RunID:           runcontext.RunID(ctx),
		Provider:        usage.Provider,
		Model:           usage.Model,
		InputTokens:     usage.InputTokens,
		OutputTokens:    usage.OutputTokens,
		EmbeddingTokens: usage.EmbeddingTokens,
		LatencyMillis:   usage.LatencyMillis,
		CostEstimate:    usage.CostEstimate,
		FallbackChain:   append([]string(nil), usage.FallbackChain...),
		RequestID:       usage.RequestID,
		FinishReason:    usage.FinishReason,
	}
}
