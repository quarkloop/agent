package modelservice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/quarkloop/pkg/plugin"
)

// Sink receives redacted usage emitted after each model operation. The sink is
// owned by runtime/Core storage; the model service never calls another service.
type Sink func(context.Context, Usage)

type Option func(*Service)

// RetryPolicy bounds same-provider retries for transient provider transport
// failures. Fallbacks, when configured, are attempted after this policy is
// exhausted for the primary provider.
type RetryPolicy struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

func defaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 3,
		BaseDelay:   500 * time.Millisecond,
		MaxDelay:    2 * time.Second,
	}
}

func normalizeRetryPolicy(policy RetryPolicy) RetryPolicy {
	defaults := defaultRetryPolicy()
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = defaults.MaxAttempts
	}
	if policy.BaseDelay < 0 {
		policy.BaseDelay = 0
	}
	if policy.BaseDelay == 0 {
		policy.MaxDelay = 0
	} else if policy.MaxDelay <= 0 {
		policy.MaxDelay = defaults.MaxDelay
	}
	return policy
}

// WithRetryPolicy configures retry for transient same-provider failures.
func WithRetryPolicy(policy RetryPolicy) Option {
	return func(s *Service) {
		s.retryPolicy = normalizeRetryPolicy(policy)
	}
}

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
	mu          sync.RWMutex
	providers   map[string]plugin.Provider
	fallbacks   map[string][]string
	sink        Sink
	clock       clock
	retryPolicy RetryPolicy
}

// New creates a model service with copied provider and fallback maps.
func New(providers map[string]plugin.Provider, sink Sink, opts ...Option) *Service {
	svc := &Service{
		providers:   cloneProviders(providers),
		fallbacks:   make(map[string][]string),
		sink:        sink,
		clock:       realClock{},
		retryPolicy: defaultRetryPolicy(),
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
		if id == "" || providerIsNil(provider) {
			continue
		}
		out[id] = provider
	}
	return out
}

func providerIsNil(provider plugin.Provider) bool {
	if provider == nil {
		return true
	}
	value := reflect.ValueOf(provider)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
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

func retryDelay(policy RetryPolicy, failedAttempt int) time.Duration {
	if policy.BaseDelay <= 0 {
		return 0
	}
	delay := policy.BaseDelay
	for i := 1; i < failedAttempt; i++ {
		delay *= 2
		if policy.MaxDelay > 0 && delay >= policy.MaxDelay {
			return policy.MaxDelay
		}
	}
	if policy.MaxDelay > 0 && delay > policy.MaxDelay {
		return policy.MaxDelay
	}
	return delay
}

func sleepRetryDelay(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
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
		if !ok {
			started := a.service.now()
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
		stream, err := a.chatCompletionStreamWithRetry(ctx, providerID, provider, req, inputTokens, attempted)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", providerID, err))
			if !canFallbackAfter(err) {
				return nil, err
			}
			continue
		}
		return stream, nil
	}
	return nil, plugin.NewProviderError(plugin.ProviderErrorExhausted, a.primary, modelName(req), 0, fmt.Errorf("model service has no available provider for %q: %w", a.primary, errors.Join(failures...)))
}

func (a *adapter) chatCompletionStreamWithRetry(ctx context.Context, providerID string, provider plugin.Provider, req *plugin.ChatRequest, inputTokens int64, attempted []string) (<-chan plugin.StreamEvent, error) {
	policy := normalizeRetryPolicy(a.service.retryPolicy)
	var lastErr error
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		started := a.service.now()
		stream, err := provider.ChatCompletionStream(ctx, req)
		if err == nil {
			return a.wrapStream(ctx, providerID, req, stream, started, attempted, inputTokens), nil
		}

		lastErr = err
		failure := providerFailureInfo(err)
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
		if attempt == policy.MaxAttempts || !canRetryAfter(err) {
			return nil, err
		}
		if err := sleepRetryDelay(ctx, retryDelay(policy, attempt)); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
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
