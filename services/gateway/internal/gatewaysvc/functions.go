package gatewaysvc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/quarkloop/pkg/plugin"
	gatewayv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/gateway/v1"
	"github.com/quarkloop/pkg/serviceapi/serviceerrors"
)

func (s *Server) Generate(ctx context.Context, req *gatewayv1.GenerateRequest) (*gatewayv1.GenerateResponse, error) {
	cmd := generateFromProto(req.GetProvider(), req.GetModel(), req.GetMessages(), req.GetTools(), req.GetOptions())
	if len(cmd.Messages) == 0 {
		return nil, serviceerrors.InvalidArgument("messages are required")
	}
	text, calls, usage, err := s.generate(ctx, req.GetProvider(), cmd)
	if err != nil {
		return nil, providerServiceError(err)
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
		return nil, providerServiceError(err)
	}
	if err := s.reserveExternalRequest(providerID, cmd.Model, "stream_generate"); err != nil {
		return nil, err
	}
	started := time.Now()
	ch, err := p.StreamGenerate(ctx, cmd)
	if err != nil {
		return nil, providerServiceError(err)
	}
	out := make(chan StreamGenerateEvent, 64)
	go func() {
		defer close(out)
		var output strings.Builder
		for event := range ch {
			if event.Err != nil {
				out <- StreamGenerateEvent{Err: providerServiceError(event.Err)}
				return
			}
			output.WriteString(event.Delta)
			usage := modelUsage{}
			var responseUsage *gatewayv1.ModelUsage
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
				responseUsage = usageToProto(usage)
			}
			out <- StreamGenerateEvent{Response: &gatewayv1.StreamGenerateResponse{
				Delta:     event.Delta,
				ToolCalls: toolCallsToProto(event.ToolCalls),
				Done:      event.Done,
				Usage:     responseUsage,
			}}
		}
	}()
	return out, nil
}

func (s *Server) Embed(ctx context.Context, req *gatewayv1.EmbedRequest) (*gatewayv1.EmbedResponse, error) {
	cmd := embedFromProto(req)
	if err := validateResolvedEmbeddingInputs(cmd.Inputs); err != nil {
		return nil, serviceerrors.InvalidArgument(err.Error())
	}
	provider := strings.TrimSpace(req.GetProvider())
	if provider == "" {
		provider = s.embeddingProvider
	}
	if provider == "" {
		return nil, serviceerrors.InvalidArgument("embedding provider is required")
	}
	providerID, p, err := s.resolveProvider(provider)
	if err != nil {
		return nil, providerServiceError(err)
	}
	if err := s.reserveExternalRequest(providerID, cmd.Model, "embed"); err != nil {
		return nil, err
	}
	started := time.Now()
	embeddings, err := p.Embed(ctx, cmd)
	if err != nil {
		return nil, providerServiceError(err)
	}
	usage := modelUsage{
		Provider:        providerID,
		Model:           embeddingUsageModel(cmd, embeddings),
		EmbeddingTokens: estimateTextTokens(embeddingTextForUsage(cmd.Inputs)...),
		LatencyMillis:   elapsedMillis(started, time.Now()),
		FallbackChain:   []string{providerID},
		RequestID:       requestID("embed"),
		FinishReason:    "stop",
	}
	s.recordUsage(usage)
	return &gatewayv1.EmbedResponse{Embeddings: embeddings, Usage: usageToProto(usage)}, nil
}

func (s *Server) Rerank(_ context.Context, req *gatewayv1.RerankRequest) (*gatewayv1.RerankResponse, error) {
	if req.GetQuery() == "" {
		return nil, serviceerrors.InvalidArgument("query is required")
	}
	if len(req.GetDocuments()) == 0 {
		return nil, serviceerrors.InvalidArgument("documents are required")
	}
	return nil, serviceerrors.Unavailable("rerank requires a configured Gateway provider adapter")
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
	return &gatewayv1.CountTokensResponse{Tokens: tokens, Usage: usageToProto(usage)}, nil
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
			return nil, providerServiceError(plugin.NewProviderError(plugin.ProviderErrorModelUnavailable, providerID, "", 0, fmt.Errorf("provider unavailable")))
		}
		models, err := p.ListModels(ctx)
		if err != nil {
			return nil, providerServiceError(err)
		}
		out = append(out, models...)
	}
	return &gatewayv1.ListModelsResponse{Models: out}, nil
}

func (s *Server) ProviderHealth(ctx context.Context, req *gatewayv1.ProviderHealthRequest) (*gatewayv1.ProviderHealthResponse, error) {
	providerID, p, err := s.resolveProvider(req.GetProvider())
	if err != nil {
		return nil, providerServiceError(err)
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
	ids, err := s.Reload(providerConfigsFromProto(req.GetProviders()), fallbackPoliciesFromProto(req.GetFallbacks()))
	if err != nil {
		return nil, providerServiceError(err)
	}
	return &gatewayv1.ReloadConfigResponse{
		Reloaded:  true,
		Providers: ids,
		Message:   "gateway provider configuration reloaded",
	}, nil
}
