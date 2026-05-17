package modelsvc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/quarkloop/pkg/plugin"
	modelv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/model/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	modelv1.UnimplementedModelServiceServer
	providers map[string]provider
	fallbacks map[string][]string
	logger    logger
}

func NewServer(cfg Config) (*Server, error) {
	providers := make(map[string]provider)
	for _, providerCfg := range cfg.Providers {
		if !providerCfg.Enabled {
			continue
		}
		p, err := newProvider(providerCfg)
		if err != nil {
			return nil, err
		}
		providers[p.ID()] = p
	}
	if len(providers) == 0 {
		providers["local"] = newLocalProvider(ProviderConfig{ID: "local", Model: "local/noop", Enabled: true})
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return &Server{
		providers: providers,
		fallbacks: cloneFallbacks(cfg.Fallbacks),
		logger:    cfg.Logger,
	}, nil
}

func (s *Server) ProviderIDs() []string {
	out := make([]string, 0, len(s.providers))
	for id := range s.providers {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (s *Server) Generate(ctx context.Context, req *modelv1.GenerateRequest) (*modelv1.GenerateResponse, error) {
	cmd := generateFromProto(req.GetProvider(), req.GetModel(), req.GetMessages(), req.GetTools(), req.GetOptions())
	if len(cmd.Messages) == 0 {
		return nil, status.Error(codes.InvalidArgument, "messages are required")
	}
	text, calls, usage, err := s.generate(ctx, req.GetProvider(), cmd)
	if err != nil {
		return nil, grpcProviderError(err)
	}
	return &modelv1.GenerateResponse{Text: text, ToolCalls: toolCallsToProto(calls), Usage: usageToProto(usage)}, nil
}

func (s *Server) StreamGenerate(req *modelv1.StreamGenerateRequest, stream modelv1.ModelService_StreamGenerateServer) error {
	cmd := generateFromProto(req.GetProvider(), req.GetModel(), req.GetMessages(), req.GetTools(), req.GetOptions())
	if len(cmd.Messages) == 0 {
		return status.Error(codes.InvalidArgument, "messages are required")
	}
	providerID, p, err := s.resolve(req.GetProvider())
	if err != nil {
		return grpcProviderError(err)
	}
	started := time.Now()
	ch, err := p.StreamGenerate(stream.Context(), cmd)
	if err != nil {
		return grpcProviderError(err)
	}
	var output strings.Builder
	var calls []toolCall
	for event := range ch {
		if event.Err != nil {
			return grpcProviderError(event.Err)
		}
		output.WriteString(event.Delta)
		calls = append(calls, event.ToolCalls...)
		usage := modelUsage{}
		if event.Done {
			usage = s.usage(providerID, cmd.Model, started, cmd, output.String(), nil, "stop")
		}
		if err := stream.Send(&modelv1.StreamGenerateResponse{
			Delta:     event.Delta,
			ToolCalls: toolCallsToProto(event.ToolCalls),
			Done:      event.Done,
			Usage:     usageToProto(usage),
		}); err != nil {
			return err
		}
	}
	_ = calls
	return nil
}

func (s *Server) Embed(ctx context.Context, req *modelv1.EmbedRequest) (*modelv1.EmbedResponse, error) {
	cmd := embedFromProto(req)
	if len(cmd.Input) == 0 {
		return nil, status.Error(codes.InvalidArgument, "input is required")
	}
	providerID, p, err := s.resolve(req.GetProvider())
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
	return &modelv1.EmbedResponse{Embeddings: embeddings, Usage: usageToProto(usage)}, nil
}

func (s *Server) Rerank(ctx context.Context, req *modelv1.RerankRequest) (*modelv1.RerankResponse, error) {
	if req.GetQuery() == "" {
		return nil, status.Error(codes.InvalidArgument, "query is required")
	}
	if len(req.GetDocuments()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "documents are required")
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
	return &modelv1.RerankResponse{Results: rerankLocal(req.GetQuery(), req.GetDocuments()), Usage: usageToProto(usage)}, nil
}

func (s *Server) CountTokens(_ context.Context, req *modelv1.CountTokensRequest) (*modelv1.CountTokensResponse, error) {
	cmd := generateFromProto(req.GetProvider(), req.GetModel(), req.GetMessages(), req.GetTools(), nil)
	tokens := estimateMessagesTokens(cmd.Messages, cmd.Tools) + estimateToolTokens(cmd.Tools)
	return &modelv1.CountTokensResponse{
		Tokens: tokens,
		Usage: &modelv1.ModelUsage{
			Provider:      firstNonEmpty(req.GetProvider(), "local"),
			Model:         req.GetModel(),
			InputTokens:   tokens,
			RequestId:     requestID("count"),
			FinishReason:  "counted",
			FallbackChain: []string{firstNonEmpty(req.GetProvider(), "local")},
		},
	}, nil
}

func (s *Server) ListModels(ctx context.Context, req *modelv1.ListModelsRequest) (*modelv1.ListModelsResponse, error) {
	providers := s.ProviderIDs()
	if req.GetProvider() != "" {
		providers = []string{req.GetProvider()}
	}
	out := make([]*modelv1.ModelInfo, 0)
	for _, providerID := range providers {
		p, ok := s.providers[providerID]
		if !ok {
			return nil, grpcProviderError(plugin.NewProviderError(plugin.ProviderErrorModelUnavailable, providerID, "", 0, fmt.Errorf("provider unavailable")))
		}
		models, err := p.ListModels(ctx)
		if err != nil {
			return nil, grpcProviderError(err)
		}
		out = append(out, models...)
	}
	return &modelv1.ListModelsResponse{Models: out}, nil
}

func (s *Server) ProviderHealth(ctx context.Context, req *modelv1.ProviderHealthRequest) (*modelv1.ProviderHealthResponse, error) {
	providerID, p, err := s.resolve(req.GetProvider())
	if err != nil {
		return nil, grpcProviderError(err)
	}
	health := p.Health(ctx)
	return &modelv1.ProviderHealthResponse{Provider: providerID, Healthy: health.Healthy, Status: health.Status}, nil
}

func (s *Server) generate(ctx context.Context, primary string, cmd generateCommand) (string, []toolCall, modelUsage, error) {
	inputTokens := estimateMessagesTokens(cmd.Messages, cmd.Tools)
	attempted := make([]string, 0)
	failures := make([]error, 0)
	for _, providerID := range s.providerChain(primary) {
		attempted = append(attempted, providerID)
		p, ok := s.providers[providerID]
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
		}
		body := text.String()
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

func (s *Server) resolve(requested string) (string, provider, error) {
	for _, providerID := range s.providerChain(requested) {
		p, ok := s.providers[providerID]
		if ok {
			return providerID, p, nil
		}
	}
	return "", nil, plugin.NewProviderError(plugin.ProviderErrorModelUnavailable, requested, "", 0, fmt.Errorf("provider unavailable"))
}

func (s *Server) providerChain(primary string) []string {
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

func newProvider(cfg ProviderConfig) (provider, error) {
	if strings.TrimSpace(cfg.ID) == "" {
		return nil, fmt.Errorf("provider id is required")
	}
	switch strings.TrimSpace(cfg.Kind) {
	case "", "local":
		return newLocalProvider(cfg), nil
	case "openai-compatible":
		return newOpenAICompatibleProvider(cfg), nil
	case "unsupported":
		return newUnsupportedProvider(cfg), nil
	default:
		return nil, fmt.Errorf("unsupported provider kind %q", cfg.Kind)
	}
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
			return status.Error(codes.Unauthenticated, providerErr.Error())
		case plugin.ProviderErrorRateLimit:
			return status.Error(codes.ResourceExhausted, providerErr.Error())
		case plugin.ProviderErrorModelUnavailable:
			return status.Error(codes.NotFound, providerErr.Error())
		case plugin.ProviderErrorContextOverflow:
			return status.Error(codes.OutOfRange, providerErr.Error())
		case plugin.ProviderErrorInvalidRequest:
			return status.Error(codes.InvalidArgument, providerErr.Error())
		default:
			return status.Error(codes.Unavailable, providerErr.Error())
		}
	}
	return status.Error(codes.Internal, err.Error())
}
