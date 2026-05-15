package modelservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/quarkloop/pkg/plugin"
)

// Sink receives redacted usage emitted after each model operation. The sink is
// owned by runtime/Core storage; the model service never calls another service.
type Sink func(context.Context, Usage)

type Option func(*Service)

// WithFallbacks configures explicit provider fallback order by primary
// provider ID. The order is copied during construction.
func WithFallbacks(fallbacks map[string][]string) Option {
	return func(s *Service) {
		s.fallbacks = cloneFallbacks(fallbacks)
	}
}

type clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// Service is the runtime model boundary. Provider plugins are implementation
// adapters behind this boundary; runtime code asks the service for a provider
// adapter and receives usage through Sink.
type Service struct {
	mu        sync.RWMutex
	providers map[string]plugin.Provider
	fallbacks map[string][]string
	sink      Sink
	clock     clock
}

// New creates a model service with copied provider and fallback maps.
func New(providers map[string]plugin.Provider, sink Sink, opts ...Option) *Service {
	svc := &Service{
		providers: cloneProviders(providers),
		fallbacks: make(map[string][]string),
		sink:      sink,
		clock:     realClock{},
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

// HasProvider reports whether the service can dispatch to providerID.
func (s *Service) HasProvider(providerID string) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.providers[providerID]
	return ok
}

// Adapter returns a plugin.Provider compatible adapter for the requested
// primary provider. The adapter records usage for every call and can try
// configured startup fallbacks without hiding the fallback chain.
func (s *Service) Adapter(providerID string) plugin.Provider {
	return &adapter{service: s, primary: strings.TrimSpace(providerID)}
}

func (s *Service) emit(ctx context.Context, usage Usage) {
	if s == nil || s.sink == nil {
		return
	}
	usage.SessionID = SessionID(ctx)
	usage.RunID = RunID(ctx)
	s.sink(ctx, cloneUsage(usage))
}

func (s *Service) providerChain(primary string) []string {
	if s == nil {
		return []string{primary}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	chain := []string{primary}
	for _, fallback := range s.fallbacks[primary] {
		fallback = strings.TrimSpace(fallback)
		if fallback == "" || contains(chain, fallback) {
			continue
		}
		chain = append(chain, fallback)
	}
	return chain
}

func (s *Service) provider(providerID string) (plugin.Provider, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	provider, ok := s.providers[providerID]
	return provider, ok
}

func (s *Service) now() time.Time {
	if s == nil || s.clock == nil {
		return time.Now()
	}
	return s.clock.Now()
}

func cloneProviders(providers map[string]plugin.Provider) map[string]plugin.Provider {
	out := make(map[string]plugin.Provider, len(providers))
	for id, provider := range providers {
		id = strings.TrimSpace(id)
		if id == "" || provider == nil {
			continue
		}
		out[id] = provider
	}
	return out
}

func cloneFallbacks(fallbacks map[string][]string) map[string][]string {
	out := make(map[string][]string, len(fallbacks))
	for provider, chain := range fallbacks {
		provider = strings.TrimSpace(provider)
		if provider == "" {
			continue
		}
		for _, fallback := range chain {
			fallback = strings.TrimSpace(fallback)
			if fallback == "" || contains(out[provider], fallback) {
				continue
			}
			out[provider] = append(out[provider], fallback)
		}
	}
	return out
}

func contains(items []string, item string) bool {
	for _, existing := range items {
		if existing == item {
			return true
		}
	}
	return false
}

type adapter struct {
	service *Service
	primary string
}

func (a *adapter) ChatCompletionStream(ctx context.Context, req *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
	if a.service == nil {
		return nil, fmt.Errorf("model service is not configured")
	}
	chain := a.service.providerChain(a.primary)
	attempted := make([]string, 0, len(chain))
	failures := make([]error, 0, len(chain))
	inputTokens := estimateInputTokens(req)

	for _, providerID := range chain {
		attempted = append(attempted, providerID)
		provider, ok := a.service.provider(providerID)
		started := a.service.now()
		if !ok {
			err := plugin.NewProviderError(plugin.ProviderErrorModelUnavailable, providerID, modelName(req), 0, fmt.Errorf("provider unavailable"))
			failures = append(failures, err)
			a.service.emit(ctx, Usage{
				Provider:        providerID,
				Model:           modelName(req),
				InputTokens:     inputTokens,
				LatencyMillis:   elapsedMillis(started, a.service.now()),
				FallbackChain:   append([]string(nil), attempted...),
				FailureCategory: string(plugin.ProviderErrorModelUnavailable),
				FinishReason:    "provider_unavailable",
			})
			continue
		}
		stream, err := provider.ChatCompletionStream(ctx, req)
		if err != nil {
			failure := providerFailureInfo(err)
			failures = append(failures, fmt.Errorf("%s: %w", providerID, err))
			a.service.emit(ctx, Usage{
				Provider:        providerID,
				Model:           modelName(req),
				InputTokens:     inputTokens,
				LatencyMillis:   elapsedMillis(started, a.service.now()),
				FallbackChain:   append([]string(nil), attempted...),
				FailureCategory: failure.category,
				FailureResetAt:  failure.resetAt,
				FinishReason:    "error",
			})
			if !canFallbackAfter(err) {
				return nil, err
			}
			continue
		}
		return a.wrapStream(ctx, providerID, req, stream, started, attempted, inputTokens), nil
	}
	return nil, plugin.NewProviderError(plugin.ProviderErrorExhausted, a.primary, modelName(req), 0, fmt.Errorf("model service has no available provider for %q: %w", a.primary, errors.Join(failures...)))
}

func (a *adapter) ParseToolCalls(content string) ([]plugin.ToolCall, string) {
	if a.service == nil {
		return nil, content
	}
	for _, providerID := range a.service.providerChain(a.primary) {
		provider, ok := a.service.provider(providerID)
		if !ok {
			continue
		}
		return provider.ParseToolCalls(content)
	}
	return nil, content
}

func (a *adapter) wrapStream(ctx context.Context, providerID string, req *plugin.ChatRequest, stream <-chan plugin.StreamEvent, started time.Time, attempted []string, inputTokens int64) <-chan plugin.StreamEvent {
	out := make(chan plugin.StreamEvent)
	go func() {
		defer close(out)
		var outputChars int64
		finishReason := "stop"
		emitted := false
		emit := func(reason string) {
			if emitted {
				return
			}
			emitted = true
			a.service.emit(ctx, Usage{
				Provider:      providerID,
				Model:         modelName(req),
				InputTokens:   inputTokens,
				OutputTokens:  estimateTokensFromChars(outputChars),
				LatencyMillis: elapsedMillis(started, a.service.now()),
				FallbackChain: append([]string(nil), attempted...),
				FinishReason:  reason,
			})
		}
		for ev := range stream {
			outputChars += int64(len(ev.Delta))
			if ev.Err != nil {
				finishReason = "error"
				emit(finishReason)
			}
			if ev.Done && finishReason != "error" {
				finishReason = "stop"
				emit(finishReason)
			}
			out <- ev
		}
		emit(finishReason)
	}()
	return out
}

func modelName(req *plugin.ChatRequest) string {
	if req == nil {
		return ""
	}
	return req.Model
}

func estimateInputTokens(req *plugin.ChatRequest) int64 {
	if req == nil {
		return 0
	}
	var chars int64
	for _, msg := range req.Messages {
		chars += int64(len(msg.Role) + len(msg.Content) + len(msg.ToolCallID))
		for _, call := range msg.ToolCalls {
			chars += int64(len(call.ID) + len(call.Type) + len(call.Function.Name) + len(call.Function.Arguments))
		}
	}
	for _, tool := range req.Tools {
		chars += int64(len(tool.Name) + len(tool.Description))
		encoded, _ := json.Marshal(tool.Parameters)
		chars += int64(len(encoded))
	}
	return estimateTokensFromChars(chars)
}

func estimateTokensFromChars(chars int64) int64 {
	if chars <= 0 {
		return 0
	}
	tokens := chars / 4
	if chars%4 != 0 {
		tokens++
	}
	if tokens == 0 {
		return 1
	}
	return tokens
}

func elapsedMillis(start, end time.Time) int64 {
	if end.Before(start) {
		return 0
	}
	return end.Sub(start).Milliseconds()
}
