package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/quarkloop/pkg/natskit"
	gatewayv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/gateway/v1"
	"github.com/quarkloop/services/gateway/internal/gatewaysvc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const defaultGatewayQueue = "q.gateway.v1"

type gatewayUnary func(context.Context, proto.Message) (proto.Message, *gatewayv1.ModelUsage, error)

func startGatewayHost(ctx context.Context, cfg natskit.Config, queue string, server *gatewaysvc.Server) (*natskit.Host, error) {
	if queue == "" {
		queue = defaultGatewayQueue
	}
	host, err := natskit.NewHost(ctx, cfg, queue)
	if err != nil {
		return nil, err
	}
	unary := []struct {
		function string
		request  func() proto.Message
		invoke   gatewayUnary
	}{
		{"generate", func() proto.Message { return &gatewayv1.GenerateRequest{} }, func(ctx context.Context, msg proto.Message) (proto.Message, *gatewayv1.ModelUsage, error) {
			resp, err := server.Generate(ctx, msg.(*gatewayv1.GenerateRequest))
			return resp, resp.GetUsage(), err
		}},
		{"embed", func() proto.Message { return &gatewayv1.EmbedRequest{} }, func(ctx context.Context, msg proto.Message) (proto.Message, *gatewayv1.ModelUsage, error) {
			resp, err := server.Embed(ctx, msg.(*gatewayv1.EmbedRequest))
			return resp, resp.GetUsage(), err
		}},
		{"rerank", func() proto.Message { return &gatewayv1.RerankRequest{} }, func(ctx context.Context, msg proto.Message) (proto.Message, *gatewayv1.ModelUsage, error) {
			resp, err := server.Rerank(ctx, msg.(*gatewayv1.RerankRequest))
			return resp, resp.GetUsage(), err
		}},
		{"count_tokens", func() proto.Message { return &gatewayv1.CountTokensRequest{} }, func(ctx context.Context, msg proto.Message) (proto.Message, *gatewayv1.ModelUsage, error) {
			resp, err := server.CountTokens(ctx, msg.(*gatewayv1.CountTokensRequest))
			return resp, resp.GetUsage(), err
		}},
		{"list_models", func() proto.Message { return &gatewayv1.ListModelsRequest{} }, func(ctx context.Context, msg proto.Message) (proto.Message, *gatewayv1.ModelUsage, error) {
			resp, err := server.ListModels(ctx, msg.(*gatewayv1.ListModelsRequest))
			return resp, nil, err
		}},
		{"provider_health", func() proto.Message { return &gatewayv1.ProviderHealthRequest{} }, func(ctx context.Context, msg proto.Message) (proto.Message, *gatewayv1.ModelUsage, error) {
			resp, err := server.ProviderHealth(ctx, msg.(*gatewayv1.ProviderHealthRequest))
			return resp, nil, err
		}},
		{"usage_summary", func() proto.Message { return &gatewayv1.UsageSummaryRequest{} }, func(ctx context.Context, msg proto.Message) (proto.Message, *gatewayv1.ModelUsage, error) {
			resp, err := server.UsageSummary(ctx, msg.(*gatewayv1.UsageSummaryRequest))
			return resp, nil, err
		}},
		{"reload_config", func() proto.Message { return &gatewayv1.ReloadConfigRequest{} }, func(ctx context.Context, msg proto.Message) (proto.Message, *gatewayv1.ModelUsage, error) {
			resp, err := server.ReloadConfig(ctx, msg.(*gatewayv1.ReloadConfigRequest))
			return resp, nil, err
		}},
	}
	for _, item := range unary {
		operation, err := natskit.ServiceOperation("gateway", item.function)
		if err != nil {
			host.Close()
			return nil, err
		}
		item := item
		if err := host.RegisterUnary(operation, timeout(cfg), func(ctx context.Context, req natskit.RequestEnvelope) (natskit.ResponseEnvelope, error) {
			input := item.request()
			if err := protojson.Unmarshal(req.Payload, input); err != nil {
				return natskit.ResponseEnvelope{}, err
			}
			output, usage, err := item.invoke(ctx, input)
			if err != nil {
				return natskit.ResponseEnvelope{}, err
			}
			payload, err := protojson.MarshalOptions{UseProtoNames: false}.Marshal(output)
			if err != nil {
				return natskit.ResponseEnvelope{}, err
			}
			resp := natskit.OKResponse(req.ServiceCallID, payload)
			resp.Usage = gatewayUsage(usage)
			return resp, nil
		}); err != nil {
			host.Close()
			return nil, err
		}
	}
	streamOperation, _ := natskit.ServiceOperation("gateway", "stream_generate")
	if err := host.RegisterStream(streamOperation, timeout(cfg), func(ctx context.Context, req natskit.RequestEnvelope, publish func(natskit.ResponseEnvelope) error) (natskit.ResponseEnvelope, error) {
		var input gatewayv1.StreamGenerateRequest
		if err := protojson.Unmarshal(req.Payload, &input); err != nil {
			return natskit.ResponseEnvelope{}, err
		}
		events, err := server.StreamGenerateEvents(ctx, &input)
		if err != nil {
			return natskit.ResponseEnvelope{}, err
		}
		for event := range events {
			if event.Err != nil {
				return natskit.ResponseEnvelope{}, event.Err
			}
			payload, err := protojson.MarshalOptions{UseProtoNames: false}.Marshal(event.Response)
			if err != nil {
				return natskit.ResponseEnvelope{}, err
			}
			resp := natskit.OKResponse(req.ServiceCallID, payload)
			resp.Usage = gatewayUsage(event.Response.GetUsage())
			if event.Response.GetDone() {
				return resp, nil
			}
			if err := publish(resp); err != nil {
				return natskit.ResponseEnvelope{}, err
			}
		}
		return natskit.ResponseEnvelope{}, fmt.Errorf("gateway stream closed without completion")
	}); err != nil {
		host.Close()
		return nil, err
	}
	if err := host.Ready(ctx); err != nil {
		host.Close()
		return nil, err
	}
	return host, nil
}

func gatewayUsage(usage *gatewayv1.ModelUsage) *natskit.Usage {
	if usage == nil || usage.GetProvider() == "" {
		return nil
	}
	additional, _ := json.Marshal(map[string]any{
		"latency_millis": usage.GetLatencyMillis(),
		"cost_estimate":  usage.GetCostEstimate(),
		"fallback_chain": usage.GetFallbackChain(),
		"finish_reason":  usage.GetFinishReason(),
	})
	return &natskit.Usage{
		Provider:       usage.GetProvider(),
		Model:          usage.GetModel(),
		RequestID:      usage.GetRequestId(),
		InputTokens:    usage.GetInputTokens(),
		OutputTokens:   usage.GetOutputTokens(),
		TotalTokens:    usage.GetInputTokens() + usage.GetOutputTokens() + usage.GetEmbeddingTokens(),
		AdditionalJSON: additional,
	}
}

func timeout(cfg natskit.Config) time.Duration {
	if cfg.Timeout > 0 {
		return cfg.Timeout
	}
	return natskit.DefaultTimeout
}
