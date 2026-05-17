package modelsvc

import (
	"encoding/json"

	modelv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/model/v1"
)

func generateFromProto(provider, model string, messages []*modelv1.ModelMessage, tools []*modelv1.ToolSchema, options map[string]string) generateCommand {
	return generateCommand{
		Model:    model,
		Messages: messagesFromProto(messages),
		Tools:    toolsFromProto(tools),
		Options:  cloneStringMap(options),
	}
}

func embedFromProto(req *modelv1.EmbedRequest) embedCommand {
	if req == nil {
		return embedCommand{}
	}
	return embedCommand{
		Model:      req.GetModel(),
		Input:      append([]string(nil), req.GetInput()...),
		Dimensions: req.GetDimensions(),
		Options:    cloneStringMap(req.GetOptions()),
	}
}

func messagesFromProto(in []*modelv1.ModelMessage) []message {
	out := make([]message, 0, len(in))
	for _, msg := range in {
		if msg == nil {
			continue
		}
		out = append(out, message{
			Role:       msg.GetRole(),
			Content:    msg.GetContent(),
			ToolCalls:  toolCallsFromProto(msg.GetToolCalls()),
			ToolCallID: msg.GetToolCallId(),
		})
	}
	return out
}

func toolsFromProto(in []*modelv1.ToolSchema) []toolSchema {
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

func toolCallsFromProto(in []*modelv1.ToolCall) []toolCall {
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

func toolCallsToProto(in []toolCall) []*modelv1.ToolCall {
	out := make([]*modelv1.ToolCall, 0, len(in))
	for _, call := range in {
		out = append(out, &modelv1.ToolCall{
			Index:         call.Index,
			Id:            call.ID,
			Type:          call.Type,
			Name:          call.Name,
			ArgumentsJson: call.ArgumentsJSON,
		})
	}
	return out
}

func usageToProto(usage modelUsage) *modelv1.ModelUsage {
	return &modelv1.ModelUsage{
		Provider:        usage.Provider,
		Model:           usage.Model,
		InputTokens:     usage.InputTokens,
		OutputTokens:    usage.OutputTokens,
		EmbeddingTokens: usage.EmbeddingTokens,
		LatencyMillis:   usage.LatencyMillis,
		FallbackChain:   append([]string(nil), usage.FallbackChain...),
		RequestId:       usage.RequestID,
		FinishReason:    usage.FinishReason,
	}
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
