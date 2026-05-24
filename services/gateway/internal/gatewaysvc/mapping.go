package gatewaysvc

import (
	"encoding/json"

	gatewayv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/gateway/v1"
)

func generateFromProto(provider, model string, messages []*gatewayv1.ModelMessage, tools []*gatewayv1.ToolSchema, options map[string]string) generateCommand {
	return generateCommand{
		Model:    model,
		Messages: messagesFromProto(messages),
		Tools:    toolsFromProto(tools),
		Options:  cloneStringMap(options),
	}
}

func embedFromProto(req *gatewayv1.EmbedRequest) embedCommand {
	if req == nil {
		return embedCommand{}
	}
	return embedCommand{
		Model:      req.GetModel(),
		Inputs:     multimodalInputsFromProto(req.GetInputs()),
		Dimensions: req.GetDimensions(),
		Options:    cloneStringMap(req.GetOptions()),
	}
}

func messagesFromProto(in []*gatewayv1.ModelMessage) []message {
	out := make([]message, 0, len(in))
	for _, msg := range in {
		if msg == nil {
			continue
		}
		out = append(out, message{
			Role:       msg.GetRole(),
			Content:    contentPartsFromProto(msg.GetContent()),
			ToolCalls:  toolCallsFromProto(msg.GetToolCalls()),
			ToolCallID: msg.GetToolCallId(),
		})
	}
	return out
}

func multimodalInputsFromProto(in []*gatewayv1.MultimodalInput) []multimodalInput {
	out := make([]multimodalInput, 0, len(in))
	for _, input := range in {
		if input == nil {
			continue
		}
		out = append(out, multimodalInput{
			Content:  contentPartsFromProto(input.GetContent()),
			Metadata: cloneStringMap(input.GetMetadata()),
		})
	}
	return out
}

func contentPartsFromProto(in []*gatewayv1.ContentPart) []contentPart {
	out := make([]contentPart, 0, len(in))
	for _, part := range in {
		if part == nil {
			continue
		}
		out = append(out, contentPart{
			Kind:      contentKindFromProto(part.GetKind()),
			Text:      part.GetText(),
			ImageURL:  part.GetImageUrl(),
			ImageData: append([]byte(nil), part.GetImageData()...),
			MIMEType:  part.GetMimeType(),
			Ref:       part.GetRef(),
			Metadata:  cloneStringMap(part.GetMetadata()),
		})
	}
	return out
}

func contentKindFromProto(kind gatewayv1.ContentKind) contentKind {
	switch kind {
	case gatewayv1.ContentKind_CONTENT_KIND_TEXT:
		return contentText
	case gatewayv1.ContentKind_CONTENT_KIND_IMAGE_URL:
		return contentImageURL
	case gatewayv1.ContentKind_CONTENT_KIND_IMAGE_DATA:
		return contentImageData
	case gatewayv1.ContentKind_CONTENT_KIND_CONTENT_REF:
		return contentContentRef
	case gatewayv1.ContentKind_CONTENT_KIND_IMAGE_REF:
		return contentImageRef
	case gatewayv1.ContentKind_CONTENT_KIND_PAGE_REF:
		return contentPageRef
	case gatewayv1.ContentKind_CONTENT_KIND_ARTIFACT_REF:
		return contentArtifactRef
	case gatewayv1.ContentKind_CONTENT_KIND_FILE_REF:
		return contentFileRef
	default:
		return 0
	}
}

func toolsFromProto(in []*gatewayv1.ToolSchema) []toolSchema {
	out := make([]toolSchema, 0, len(in))
	for _, tool := range in {
		if tool == nil {
			continue
		}
		out = append(out, toolSchema{
			Name:           tool.GetName(),
			Description:    tool.GetDescription(),
			ParametersJSON: tool.GetParametersJson(),
		})
	}
	return out
}

func toolCallsFromProto(in []*gatewayv1.ToolCall) []toolCall {
	out := make([]toolCall, 0, len(in))
	for _, call := range in {
		if call == nil {
			continue
		}
		out = append(out, toolCall{
			Index:         call.GetIndex(),
			ID:            call.GetId(),
			Type:          call.GetType(),
			Name:          call.GetName(),
			ArgumentsJSON: call.GetArgumentsJson(),
		})
	}
	return out
}

func toolCallsToProto(in []toolCall) []*gatewayv1.ToolCall {
	out := make([]*gatewayv1.ToolCall, 0, len(in))
	for _, call := range in {
		out = append(out, &gatewayv1.ToolCall{
			Index:         call.Index,
			Id:            call.ID,
			Type:          call.Type,
			Name:          call.Name,
			ArgumentsJson: call.ArgumentsJSON,
		})
	}
	return out
}

func usageToProto(usage modelUsage) *gatewayv1.ModelUsage {
	return &gatewayv1.ModelUsage{
		Provider:        usage.Provider,
		Model:           usage.Model,
		InputTokens:     usage.InputTokens,
		OutputTokens:    usage.OutputTokens,
		EmbeddingTokens: usage.EmbeddingTokens,
		LatencyMillis:   usage.LatencyMillis,
		CostEstimate:    usage.CostEstimate,
		FallbackChain:   append([]string(nil), usage.FallbackChain...),
		RequestId:       usage.RequestID,
		FinishReason:    usage.FinishReason,
	}
}

func usageAggregateToProto(usage UsageAggregate) *gatewayv1.UsageAggregate {
	return &gatewayv1.UsageAggregate{
		Provider:        usage.Provider,
		Model:           usage.Model,
		Requests:        usage.Requests,
		InputTokens:     usage.InputTokens,
		OutputTokens:    usage.OutputTokens,
		EmbeddingTokens: usage.EmbeddingTokens,
		TotalTokens:     usage.TotalTokens,
		LatencyMillis:   usage.LatencyMillis,
		CostEstimate:    usage.CostEstimate,
		FallbackChain:   append([]string(nil), usage.FallbackChain...),
	}
}

func providerConfigsFromProto(in []*gatewayv1.GatewayProviderConfig) []ProviderConfig {
	out := make([]ProviderConfig, 0, len(in))
	for _, cfg := range in {
		if cfg == nil {
			continue
		}
		out = append(out, ProviderConfig{
			ID:             cfg.GetId(),
			Kind:           cfg.GetKind(),
			BaseURL:        cfg.GetBaseUrl(),
			Model:          cfg.GetModel(),
			EmbeddingModel: cfg.GetEmbeddingModel(),
			Enabled:        cfg.GetEnabled(),
		})
	}
	return out
}

func fallbackPoliciesFromProto(in []*gatewayv1.GatewayFallbackPolicy) map[string][]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string][]string, len(in))
	for _, policy := range in {
		if policy == nil || policy.GetProvider() == "" {
			continue
		}
		out[policy.GetProvider()] = append([]string(nil), policy.GetFallbacks()...)
	}
	return out
}

func parametersMap(tool toolSchema) map[string]any {
	if tool.ParametersJSON == "" {
		return nil
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(tool.ParametersJSON), &out); err != nil {
		return nil
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
