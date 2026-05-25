package app

import (
	"encoding/json"

	"github.com/quarkloop/pkg/natskit"
	gatewayv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/gateway/v1"
	"google.golang.org/protobuf/proto"
)

// gatewayUsageFromResponse exports accounting fields only. Model content,
// request payloads, and provider credentials are never envelope metadata.
func gatewayUsageFromResponse(response proto.Message) *natskit.Usage {
	var usage *gatewayv1.ModelUsage
	switch typed := response.(type) {
	case *gatewayv1.GenerateResponse:
		usage = typed.GetUsage()
	case *gatewayv1.StreamGenerateResponse:
		usage = typed.GetUsage()
	case *gatewayv1.EmbedResponse:
		usage = typed.GetUsage()
	case *gatewayv1.RerankResponse:
		usage = typed.GetUsage()
	case *gatewayv1.CountTokensResponse:
		usage = typed.GetUsage()
	}
	if usage == nil || usage.GetProvider() == "" {
		return nil
	}
	additional, _ := json.Marshal(map[string]any{
		"latency_millis": usage.GetLatencyMillis(),
		"cost_estimate":  usage.GetCostEstimate(),
		"fallback_chain": append([]string(nil), usage.GetFallbackChain()...),
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
