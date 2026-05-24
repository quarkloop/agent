package gatewaysvc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/quarkloop/pkg/plugin"
	gatewayv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/gateway/v1"
	"github.com/quarkloop/pkg/serviceapi/serviceerrors"
)

type Server struct {
	mu              sync.RWMutex
	providers       map[string]provider
	providerConfigs map[string]ProviderConfig
	fallbacks       map[string][]string
	recorder        *usageRecorder
	logger          logger
}

func NewServer(cfg Config) (*Server, error) {
	providers, providerConfigs, err := buildProviders(cfg.Providers)
	if err != nil {
		return nil, err
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Server{
		providers:       providers,
		providerConfigs: providerConfigs,
		fallbacks:       cloneFallbacks(cfg.Fallbacks),
		recorder:        newUsageRecorder(),
		logger:          cfg.Logger,
	}, nil
}

func (s *Server) ProviderIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.providerIDsLocked()
}

func (s *Server) Generate(ctx context.Context, req *gatewayv1.GenerateRequest) (*gatewayv1.GenerateResponse, error) {
	cmd := generateFromProto(req.GetProvider(), req.GetModel(), req.GetMessages(), req.GetTools(), req.GetOptions())
	if len(cmd.Messages) == 0 {
		return nil, serviceerrors.InvalidArgument("messages are required")
	}
	text, calls, usage, err := s.generate(ctx, req.GetProvider(), cmd)
	if err != nil {
		return nil, grpcProviderError(err)
	}
	s.recordUsage(usage)
	return &gatewayv1.GenerateResponse{Text: text, ToolCalls: toolCallsToProto(calls), Usage: usageToProto(usage)}, nil
}

func (s *Server) StreamGenerateEvents(ctx context.Context, req *gatewayv1.StreamGenerateRequest) (<-chan StreamGenerateEvent, error) {
	cmd := generateFromProto(req.GetProvider(), req.GetModel(), req.GetMessages(), req.GetTools(), req.GetOptions())
	if len(cmd.Messages) == 0 {
		return nil, serviceerrors.InvalidArgument("messages are required")
	}
	providerID, p, err := s.resolveProvider(req.GetProvider())
	if err != nil {
		return nil, err
	}
	started := time.Now()
	ch, err := p.StreamGenerate(ctx, cmd)
	if err != nil {
		return nil, err
	}
	out := make(chan StreamGenerateEvent, 64)
	go func() {
		defer close(out)
		var output strings.Builder
		for event := range ch {
			if event.Err != nil {
				out <- StreamGenerateEvent{Err: event.Err}
				return
			}
			output.WriteString(event.Delta)
			usage := modelUsage{}
			if event.Done {
				if event.Usage != nil {
					usage = *event.Usage
				} else {
					usage = s.usage(providerID, cmd.Model, started, cmd, output.String(), nil, "stop")
				}
				if usage.Provider == "" {
					usage.Provider = providerID
				}
				if usage.Model == "" {
					usage.Model = firstNonEmpty(cmd.Model, defaultModel(p))
				}
				s.recordUsage(usage)
			}
			out <- StreamGenerateEvent{Response: &gatewayv1.StreamGenerateResponse{
				Delta:     event.Delta,
				ToolCalls: toolCallsToProto(event.ToolCalls),
				Done:      event.Done,
				Usage:     usageToProto(usage),
			}}
		}
	}()
	return out, nil
}

func (s *Server) Embed(ctx context.Context, req *gatewayv1.EmbedRequest) (*gatewayv1.EmbedResponse, error) {
	cmd := embedFromProto(req)
	if len(cmd.Input) == 0 {
		return nil, serviceerrors.InvalidArgument("input is required")
	}
	providerID, p, err := s.resolveProvider(req.GetProvider())
	if err != nil {
		return nil, grpcProviderError(err)
	}
	started := time.Now()
	embeddings, err := p.Embed(ctx, cmd)
	if err != nil {
		return nil, grpcProviderError(err)
	}
	usage := modelUsage{
		Provider:        providerID,
		Model:           firstNonEmpty(cmd.Model, defaultModel(p)),
		EmbeddingTokens: estimateTextTokens(cmd.Input...),
		LatencyMillis:   elapsedMillis(started, time.Now()),
		FallbackChain:   []string{providerID},
		RequestID:       requestID("embed"),
		FinishReason:    "stop",
	}
	s.recordUsage(usage)
	return &gatewayv1.EmbedResponse{Embeddings: embeddings, Usage: usageToProto(usage)}, nil
}

func (s *Server) Rerank(ctx context.Context, req *gatewayv1.RerankRequest) (*gatewayv1.RerankResponse, error) {
	if req.GetQuery() == "" {
		return nil, serviceerrors.InvalidArgument("query is required")
	}
	if len(req.GetDocuments()) == 0 {
		return nil, serviceerrors.InvalidArgument("documents are required")
	}
	providerID := firstNonEmpty(req.GetProvider(), "local")
	started := time.Now()
	usage := modelUsage{
		Provider:      providerID,
		Model:         firstNonEmpty(req.GetModel(), "local/rerank"),
		InputTokens:   estimateTextTokens(append([]string{req.GetQuery()}, req.GetDocuments()...)...),
		LatencyMillis: elapsedMillis(started, time.Now()),
		FallbackChain: []string{providerID},
		RequestID:     requestID("rerank"),
		FinishReason:  "stop",
	}
	s.recordUsage(usage)
	return &gatewayv1.RerankResponse{Results: rerankLocal(req.GetQuery(), req.GetDocuments()), Usage: usageToProto(usage)}, nil
}

func (s *Server) CountTokens(_ context.Context, req *gatewayv1.CountTokensRequest) (*gatewayv1.CountTokensResponse, error) {
	cmd := generateFromProto(req.GetProvider(), req.GetModel(), req.GetMessages(), req.GetTools(), nil)
	tokens := estimateMessagesTokens(cmd.Messages, cmd.Tools) + estimateToolTokens(cmd.Tools)
	usage := modelUsage{
		Provider:      firstNonEmpty(req.GetProvider(), "local"),
		Model:         req.GetModel(),
		InputTokens:   tokens,
		RequestID:     requestID("count"),
		FinishReason:  "counted",
		FallbackChain: []string{firstNonEmpty(req.GetProvider(), "local")},
	}
	s.recordUsage(usage)
	return &gatewayv1.CountTokensResponse{
		Tokens: tokens,
		Usage:  usageToProto(usage),
	}, nil
}

func (s *Server) ListModels(ctx context.Context, req *gatewayv1.ListModelsRequest) (*gatewayv1.ListModelsResponse, error) {
	providers := s.ProviderIDs()
	if req.GetProvider() != "" {
		providers = []string{req.GetProvider()}
	}
	out := make([]*gatewayv1.ModelInfo, 0)
	for _, providerID := range providers {
		p, ok := s.provider(providerID)
		if !ok {
			return nil, grpcProviderError(plugin.NewProviderError(plugin.ProviderErrorModelUnavailable, providerID, "", 0, fmt.Errorf("provider unavailable")))
		}
		models, err := p.ListModels(ctx)
		if err != nil {
			return nil, grpcProviderError(err)
		}
		out = append(out, models...)
	}
	return &gatewayv1.ListModelsResponse{Models: out}, nil
}

func (s *Server) ProviderHealth(ctx context.Context, req *gatewayv1.ProviderHealthRequest) (*gatewayv1.ProviderHealthResponse, error) {
	providerID, p, err := s.resolveProvider(req.GetProvider())
	if err != nil {
		return nil, grpcProviderError(err)
	}
	health := p.Health(ctx)
	return &gatewayv1.ProviderHealthResponse{Provider: providerID, Healthy: health.Healthy, Status: health.Status}, nil
}

func (s *Server) UsageSummary(context.Context, *gatewayv1.UsageSummaryRequest) (*gatewayv1.UsageSummaryResponse, error) {
	summary := s.UsageSummarySnapshot()
	out := make([]*gatewayv1.UsageAggregate, 0, len(summary))
	for _, usage := range summary {
		out = append(out, usageAggregateToProto(usage))
	}
	return &gatewayv1.UsageSummaryResponse{Usage: out}, nil
}

func (s *Server) ReloadConfig(_ context.Context, req *gatewayv1.ReloadConfigRequest) (*gatewayv1.ReloadConfigResponse, error) {
	providers := providerConfigsFromProto(req.GetProviders())
	fallbacks := fallbackPoliciesFromProto(req.GetFallbacks())
	ids, err := s.Reload(providers, fallbacks)
	if err != nil {
		return nil, grpcProviderError(err)
	}
	return &gatewayv1.ReloadConfigResponse{
		Reloaded:  true,
		Providers: ids,
		Message:   "gateway provider configuration reloaded",
	}, nil
}

func (s *Server) generate(ctx context.Context, primary string, cmd generateCommand) (string, []toolCall, modelUsage, error) {
	inputTokens := estimateMessagesTokens(cmd.Messages, cmd.Tools)
	attempted := make([]string, 0)
	failures := make([]error, 0)
	chain, providers := s.providerChainSnapshot(primary)
	for _, providerID := range chain {
		attempted = append(attempted, providerID)
		p, ok := providers[providerID]
		if !ok {
			failures = append(failures, plugin.NewProviderError(plugin.ProviderErrorModelUnavailable, providerID, cmd.Model, 0, fmt.Errorf("provider unavailable")))
			continue
		}
		started := time.Now()
		stream, err := p.StreamGenerate(ctx, cmd)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", providerID, err))
			if !canFallbackAfter(err) {
				return "", nil, modelUsage{}, err
			}
			continue
		}
		var text strings.Builder
		var calls []toolCall
		var providerUsage *modelUsage
		for event := range stream {
			if event.Err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", providerID, event.Err))
				if canFallbackAfter(event.Err) {
					continue
				}
				return "", nil, modelUsage{}, event.Err
			}
			text.WriteString(event.Delta)
			calls = append(calls, event.ToolCalls...)
			if event.Usage != nil {
				usageCopy := *event.Usage
				providerUsage = &usageCopy
			}
		}
		body := text.String()
		if providerUsage != nil {
			if providerUsage.Provider == "" {
				providerUsage.Provider = providerID
			}
			if providerUsage.Model == "" {
				providerUsage.Model = firstNonEmpty(cmd.Model, defaultModel(p))
			}
			if len(providerUsage.FallbackChain) == 0 {
				providerUsage.FallbackChain = append([]string(nil), attempted...)
			}
			if providerUsage.RequestID == "" {
				providerUsage.RequestID = requestID("generate")
			}
			if providerUsage.FinishReason == "" {
				providerUsage.FinishReason = "stop"
			}
			return body, calls, *providerUsage, nil
		}
		return body, calls, modelUsage{
			Provider:      providerID,
			Model:         firstNonEmpty(cmd.Model, defaultModel(p)),
			InputTokens:   inputTokens,
			OutputTokens:  estimateTextTokens(body),
			LatencyMillis: elapsedMillis(started, time.Now()),
			FallbackChain: append([]string(nil), attempted...),
			RequestID:     requestID("generate"),
			FinishReason:  "stop",
		}, nil
	}
	return "", nil, modelUsage{}, plugin.NewProviderError(plugin.ProviderErrorExhausted, primary, cmd.Model, 0, errors.Join(failures...))
}

func (s *Server) resolveProvider(requested string) (string, provider, error) {
	chain, providers := s.providerChainSnapshot(requested)
	for _, providerID := range chain {
		p, ok := providers[providerID]
		if ok {
			return providerID, p, nil
		}
	}
	return "", nil, plugin.NewProviderError(plugin.ProviderErrorModelUnavailable, requested, "", 0, fmt.Errorf("provider unavailable"))
}

func (s *Server) providerChainLocked(primary string) []string {
	primary = strings.TrimSpace(primary)
	if primary == "" {
		if _, ok := s.providers["local"]; ok {
			primary = "local"
		} else {
			for id := range s.providers {
				primary = id
				break
			}
		}
	}
	chain := []string{primary}
	for _, fallback := range s.fallbacks[primary] {
		if fallback != "" && !contains(chain, fallback) {
			chain = append(chain, fallback)
		}
	}
	return chain
}

func (s *Server) providerChainSnapshot(primary string) ([]string, map[string]provider) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	chain := s.providerChainLocked(primary)
	providers := make(map[string]provider, len(chain))
	for _, providerID := range chain {
		if p, ok := s.providers[providerID]; ok {
			providers[providerID] = p
		}
	}
	return chain, providers
}

func (s *Server) provider(providerID string) (provider, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.providers[providerID]
	return p, ok
}

func (s *Server) usage(providerID, model string, started time.Time, cmd generateCommand, output string, chain []string, reason string) modelUsage {
	if len(chain) == 0 {
		chain = []string{providerID}
	}
	return modelUsage{
		Provider:      providerID,
		Model:         model,
		InputTokens:   estimateMessagesTokens(cmd.Messages, cmd.Tools),
		OutputTokens:  estimateTextTokens(output),
		LatencyMillis: elapsedMillis(started, time.Now()),
		FallbackChain: append([]string(nil), chain...),
		RequestID:     requestID("stream"),
		FinishReason:  reason,
	}
}

func (s *Server) UsageSummarySnapshot() []UsageAggregate {
	if s == nil || s.recorder == nil {
		return nil
	}
	return s.recorder.snapshot()
}

func (s *Server) Reload(configs []ProviderConfig, fallbacks map[string][]string) ([]string, error) {
	if s == nil {
		return nil, fmt.Errorf("gateway server is not configured")
	}
	s.mu.RLock()
	merged := cloneProviderConfigMap(s.providerConfigs)
	s.mu.RUnlock()
	for _, cfg := range configs {
		cfg.ID = strings.TrimSpace(cfg.ID)
		if cfg.ID == "" {
			return nil, fmt.Errorf("provider id is required")
		}
		existing := merged[cfg.ID]
		if cfg.Kind == "" {
			cfg.Kind = existing.Kind
		}
		if cfg.APIKey == "" {
			cfg.APIKey = existing.APIKey
		}
		if cfg.BaseURL == "" {
			cfg.BaseURL = existing.BaseURL
		}
		if cfg.Model == "" {
			cfg.Model = existing.Model
		}
		merged[cfg.ID] = cfg
	}

	nextConfigs := providerConfigMapValues(merged)
	providers, providerConfigs, err := buildProviders(nextConfigs)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	oldProviders := s.providers
	s.providers = providers
	s.providerConfigs = providerConfigs
	if fallbacks != nil {
		s.fallbacks = cloneFallbacks(fallbacks)
	}
	ids := s.providerIDsLocked()
	s.mu.Unlock()

	if err := closeProviders(oldProviders); err != nil {
		s.logger.Warn("close old gateway providers after reload", "error", err)
	}
	return ids, nil
}

func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	providers := cloneProviderMap(s.providers)
	s.mu.RUnlock()
	return closeProviders(providers)
}

func (s *Server) recordUsage(usage modelUsage) {
	if s == nil || s.recorder == nil {
		return
	}
	s.recorder.record(usage)
}

func newProvider(cfg ProviderConfig) (provider, error) {
	if strings.TrimSpace(cfg.ID) == "" {
		return nil, fmt.Errorf("provider id is required")
	}
	switch strings.TrimSpace(cfg.Kind) {
	case "", "local":
		return newLocalProvider(cfg), nil
	case "bifrost":
		return newBifrostProvider(cfg)
	case "openai-compatible":
		return newOpenAICompatibleProvider(cfg), nil
	case "unsupported":
		return newUnsupportedProvider(cfg), nil
	default:
		return nil, fmt.Errorf("unsupported provider kind %q", cfg.Kind)
	}
}

func buildProviders(configs []ProviderConfig) (map[string]provider, map[string]ProviderConfig, error) {
	providers := make(map[string]provider)
	providerConfigs := make(map[string]ProviderConfig)
	for _, providerCfg := range configs {
		if !providerCfg.Enabled {
			continue
		}
		p, err := newProvider(providerCfg)
		if err != nil {
			return nil, nil, err
		}
		id := p.ID()
		providerCfg.ID = id
		providers[id] = p
		providerConfigs[id] = providerCfg
	}
	if len(providers) == 0 {
		cfg := ProviderConfig{ID: "local", Kind: "local", Model: "local/noop", Enabled: true}
		providers["local"] = newLocalProvider(cfg)
		providerConfigs["local"] = cfg
	}
	return providers, providerConfigs, nil
}

func cloneFallbacks(in map[string][]string) map[string][]string {
	out := make(map[string][]string, len(in))
	for providerID, chain := range in {
		for _, fallback := range chain {
			if fallback != "" && !contains(out[providerID], fallback) {
				out[providerID] = append(out[providerID], fallback)
			}
		}
	}
	return out
}

func cloneProviderMap(in map[string]provider) map[string]provider {
	out := make(map[string]provider, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneProviderConfigMap(in map[string]ProviderConfig) map[string]ProviderConfig {
	out := make(map[string]ProviderConfig, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func providerConfigMapValues(in map[string]ProviderConfig) []ProviderConfig {
	out := make([]ProviderConfig, 0, len(in))
	for _, cfg := range in {
		out = append(out, cfg)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func closeProviders(providers map[string]provider) error {
	var errs []error
	for _, provider := range providers {
		if closer, ok := provider.(closableProvider); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (s *Server) providerIDsLocked() []string {
	out := make([]string, 0, len(s.providers))
	for id := range s.providers {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func contains(values []string, value string) bool {
	for _, existing := range values {
		if existing == value {
			return true
		}
	}
	return false
}

func defaultModel(p provider) string {
	models, err := p.ListModels(context.Background())
	if err != nil || len(models) == 0 {
		return ""
	}
	return models[0].GetId()
}

func requestID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func grpcProviderError(err error) error {
	if err == nil {
		return nil
	}
	var providerErr *plugin.ProviderError
	if errors.As(err, &providerErr) {
		switch providerErr.Category {
		case plugin.ProviderErrorAuth:
			return serviceerrors.Auth(providerErr.Error())
		case plugin.ProviderErrorRateLimit:
			return serviceerrors.RateLimit(providerErr.Error())
		case plugin.ProviderErrorModelUnavailable:
			return serviceerrors.NotFound(providerErr.Error())
		case plugin.ProviderErrorContextOverflow:
			return serviceerrors.ContextOverflow(providerErr.Error())
		case plugin.ProviderErrorInvalidRequest:
			return serviceerrors.InvalidArgument(providerErr.Error())
		default:
			return serviceerrors.Unavailable(providerErr.Error())
		}
	}
	return serviceerrors.Internal(err.Error())
}
