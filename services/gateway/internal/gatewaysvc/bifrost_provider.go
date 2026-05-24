package gatewaysvc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	bifrost "github.com/maximhq/bifrost/core"
	"github.com/maximhq/bifrost/core/schemas"
	"github.com/quarkloop/pkg/plugin"
	gatewayv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/gateway/v1"
)

type bifrostProvider struct {
	id             string
	model          string
	embeddingModel string
	provider       schemas.ModelProvider
	client         *bifrost.Bifrost
	account        *bifrostAccount
}

type bifrostAccount struct {
	provider schemas.ModelProvider
	keys     []schemas.Key
	config   schemas.ProviderConfig
}

func newBifrostProvider(cfg ProviderConfig) (provider, error) {
	providerKey, err := bifrostProviderKey(cfg.ID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.APIKey) == "" && providerKey != schemas.Ollama {
		return nil, plugin.NewProviderError(plugin.ProviderErrorAuth, cfg.ID, cfg.Model, 0, fmt.Errorf("api key is required"))
	}
	network := schemas.DefaultNetworkConfig
	if baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"); baseURL != "" {
		network.BaseURL = bifrostBaseURL(providerKey, baseURL)
	}
	providerCfg := schemas.ProviderConfig{
		NetworkConfig: network,
	}
	providerCfg.CheckAndSetDefaults()
	key := schemas.Key{
		ID:     strings.TrimSpace(cfg.ID) + "-primary",
		Name:   strings.TrimSpace(cfg.ID),
		Value:  *schemas.NewEnvVar(strings.TrimSpace(cfg.APIKey)),
		Models: schemas.WhiteList{"*"},
		Weight: 1,
	}
	account := &bifrostAccount{provider: providerKey, keys: []schemas.Key{key}, config: providerCfg}
	client, err := bifrost.Init(context.Background(), schemas.BifrostConfig{Account: account})
	if err != nil {
		return nil, fmt.Errorf("initialize bifrost provider %s: %w", cfg.ID, err)
	}
	return &bifrostProvider{
		id:             strings.TrimSpace(cfg.ID),
		model:          strings.TrimSpace(cfg.Model),
		embeddingModel: strings.TrimSpace(cfg.EmbeddingModel),
		provider:       providerKey,
		client:         client,
		account:        account,
	}, nil
}

func (p *bifrostProvider) ID() string { return p.id }

func (p *bifrostProvider) ListModels(ctx context.Context) ([]*gatewayv1.ModelInfo, error) {
	if p.client == nil {
		return nil, plugin.NewProviderError(plugin.ProviderErrorTransport, p.id, p.model, 0, fmt.Errorf("bifrost client is not initialized"))
	}
	resp, bifrostErr := p.client.ListModelsRequest(bifrostContext(ctx), &schemas.BifrostListModelsRequest{Provider: p.provider})
	if bifrostErr != nil {
		return nil, p.providerError(bifrostErr, p.model)
	}
	out := make([]*gatewayv1.ModelInfo, 0, len(resp.Data))
	for _, model := range resp.Data {
		name := model.ID
		if model.Name != nil && strings.TrimSpace(*model.Name) != "" {
			name = *model.Name
		}
		var contextWindow int32
		if model.ContextLength != nil {
			contextWindow = int32(*model.ContextLength)
		}
		out = append(out, &gatewayv1.ModelInfo{
			Id:            model.ID,
			Provider:      p.id,
			Name:          name,
			ContextWindow: contextWindow,
			DefaultModel:  model.ID == p.model,
		})
	}
	if len(out) == 0 && p.model != "" {
		out = append(out, &gatewayv1.ModelInfo{Id: p.model, Provider: p.id, Name: p.model, DefaultModel: true})
	}
	return out, nil
}

func (p *bifrostProvider) StreamGenerate(ctx context.Context, cmd generateCommand) (<-chan streamEvent, error) {
	if p.client == nil {
		return nil, plugin.NewProviderError(plugin.ProviderErrorTransport, p.id, cmd.Model, 0, fmt.Errorf("bifrost client is not initialized"))
	}
	includeUsage := true
	params := &schemas.ChatParameters{
		StreamOptions: &schemas.ChatStreamOptions{IncludeUsage: &includeUsage},
		Tools:         bifrostTools(cmd.Tools),
	}
	if maxOutputTokens, ok := maxOutputTokensOption(cmd.Options); ok {
		params.MaxCompletionTokens = schemas.Ptr(maxOutputTokens)
	}
	req := &schemas.BifrostChatRequest{
		Provider: p.provider,
		Model:    firstNonEmpty(cmd.Model, p.model),
		Input:    bifrostMessages(cmd.Messages),
		Params:   params,
	}
	stream, bifrostErr := p.client.ChatCompletionStreamRequest(bifrostContext(ctx), req)
	if bifrostErr != nil {
		return nil, p.providerError(bifrostErr, req.Model)
	}
	out := make(chan streamEvent, 64)
	go func() {
		defer close(out)
		done := false
		var lastUsage *modelUsage
		for chunk := range stream {
			if chunk == nil {
				continue
			}
			if chunk.BifrostError != nil {
				out <- streamEvent{Err: p.providerError(chunk.BifrostError, req.Model)}
				return
			}
			resp := chunk.BifrostChatResponse
			if resp == nil {
				continue
			}
			if resp.Usage != nil {
				usage := p.usageFromBifrost(resp.Usage, req.Model, resp.ID, "stop")
				lastUsage = &usage
			}
			if len(resp.Choices) == 0 {
				continue
			}
			choice := resp.Choices[0]
			event := streamEvent{}
			if choice.ChatStreamResponseChoice != nil && choice.ChatStreamResponseChoice.Delta != nil {
				delta := choice.ChatStreamResponseChoice.Delta
				if delta.Content != nil {
					event.Delta = *delta.Content
				}
				event.ToolCalls = toolCallsFromBifrost(delta.ToolCalls)
			}
			if choice.FinishReason != nil {
				event.Done = true
				done = true
				if lastUsage != nil {
					event.Usage = lastUsage
				}
			}
			out <- event
		}
		if !done {
			out <- streamEvent{Done: true, Usage: lastUsage}
		}
	}()
	return out, nil
}

func (p *bifrostProvider) Embed(ctx context.Context, cmd embedCommand) ([]*gatewayv1.Embedding, error) {
	if p.client == nil {
		return nil, plugin.NewProviderError(plugin.ProviderErrorTransport, p.id, cmd.Model, 0, fmt.Errorf("bifrost client is not initialized"))
	}
	model := firstNonEmpty(cmd.Model, p.embeddingModel)
	if model == "" {
		return nil, plugin.NewProviderError(plugin.ProviderErrorInvalidRequest, p.id, "", 0, fmt.Errorf("embedding model is required"))
	}
	format := "float"
	params := &schemas.EmbeddingParameters{EncodingFormat: &format}
	if cmd.Dimensions > 0 {
		dims := int(cmd.Dimensions)
		params.Dimensions = &dims
	}
	resp, bifrostErr := p.client.EmbeddingRequest(bifrostContext(ctx), &schemas.BifrostEmbeddingRequest{
		Provider: p.provider,
		Model:    model,
		Input:    &schemas.EmbeddingInput{Texts: append([]string(nil), cmd.Input...)},
		Params:   params,
	})
	if bifrostErr != nil {
		return nil, p.providerError(bifrostErr, model)
	}
	out := make([]*gatewayv1.Embedding, 0, len(resp.Data))
	for i, item := range resp.Data {
		vector := make([]float32, 0, len(item.Embedding.EmbeddingArray))
		for _, value := range item.Embedding.EmbeddingArray {
			vector = append(vector, float32(value))
		}
		if len(vector) == 0 {
			return nil, plugin.NewProviderError(plugin.ProviderErrorResponse, p.id, cmd.Model, 0, fmt.Errorf("provider returned non-float embedding format"))
		}
		input := ""
		if i < len(cmd.Input) {
			input = cmd.Input[i]
		}
		out = append(out, &gatewayv1.Embedding{
			Vector:      vector,
			Provider:    p.id,
			Model:       firstNonEmpty(resp.Model, model),
			Dimensions:  int32(len(vector)),
			ContentHash: contentHash(input),
		})
	}
	return out, nil
}

func (p *bifrostProvider) Health(context.Context) providerHealth {
	if p == nil || p.client == nil {
		return providerHealth{Healthy: false, Status: "bifrost client is not initialized"}
	}
	if strings.TrimSpace(p.account.keys[0].Value.GetValue()) == "" && p.provider != schemas.Ollama {
		return providerHealth{Healthy: false, Status: "missing api key"}
	}
	return providerHealth{Healthy: true, Status: "configured"}
}

func (p *bifrostProvider) Close() error {
	if p == nil || p.client == nil {
		return nil
	}
	p.client.Shutdown()
	return nil
}

func (p *bifrostProvider) providerError(err *schemas.BifrostError, model string) error {
	if err == nil {
		return nil
	}
	statusCode := 0
	if err.StatusCode != nil {
		statusCode = *err.StatusCode
	}
	category := plugin.ProviderErrorCategoryForHTTPStatus(statusCode)
	if statusCode == 0 {
		category = plugin.ProviderErrorTransport
	}
	return plugin.NewProviderError(category, p.id, model, statusCode, fmt.Errorf("%s", err.GetErrorString()))
}

func (p *bifrostProvider) usageFromBifrost(usage *schemas.BifrostLLMUsage, model, requestID, finish string) modelUsage {
	if usage == nil {
		return modelUsage{}
	}
	cost := 0.0
	if usage.Cost != nil {
		cost = usage.Cost.TotalCost
	}
	return modelUsage{
		Provider:      p.id,
		Model:         firstNonEmpty(model, p.model),
		InputTokens:   int64(usage.PromptTokens),
		OutputTokens:  int64(usage.CompletionTokens),
		CostEstimate:  cost,
		FallbackChain: []string{p.id},
		RequestID:     requestID,
		FinishReason:  firstNonEmpty(finish, "stop"),
	}
}

func (a *bifrostAccount) GetConfiguredProviders() ([]schemas.ModelProvider, error) {
	return []schemas.ModelProvider{a.provider}, nil
}

func (a *bifrostAccount) GetKeysForProvider(context.Context, schemas.ModelProvider) ([]schemas.Key, error) {
	out := make([]schemas.Key, len(a.keys))
	copy(out, a.keys)
	return out, nil
}

func (a *bifrostAccount) GetConfigForProvider(provider schemas.ModelProvider) (*schemas.ProviderConfig, error) {
	if provider != a.provider {
		return nil, fmt.Errorf("provider %q is not configured", provider)
	}
	cfg := a.config
	cfg.NetworkConfig.ExtraHeaders = cloneMap(cfg.NetworkConfig.ExtraHeaders)
	cfg.NetworkConfig.BetaHeaderOverrides = cloneMap(cfg.NetworkConfig.BetaHeaderOverrides)
	return &cfg, nil
}

func bifrostContext(ctx context.Context) *schemas.BifrostContext {
	return schemas.NewBifrostContext(ctx, time.Time{})
}

func bifrostProviderKey(id string) (schemas.ModelProvider, error) {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "openai":
		return schemas.OpenAI, nil
	case "openrouter":
		return schemas.OpenRouter, nil
	case "anthropic":
		return schemas.Anthropic, nil
	case "ollama":
		return schemas.Ollama, nil
	case "cohere":
		return schemas.Cohere, nil
	case "vllm":
		return schemas.VLLM, nil
	default:
		return "", fmt.Errorf("unsupported bifrost provider %q", id)
	}
}

func bifrostBaseURL(provider schemas.ModelProvider, baseURL string) string {
	if provider == schemas.OpenAI || provider == schemas.OpenRouter {
		return strings.TrimSuffix(baseURL, "/v1")
	}
	return baseURL
}

func bifrostMessages(messages []message) []schemas.ChatMessage {
	out := make([]schemas.ChatMessage, 0, len(messages))
	for _, msg := range messages {
		content := msg.Content
		bmsg := schemas.ChatMessage{
			Role:    schemas.ChatMessageRole(msg.Role),
			Content: &schemas.ChatMessageContent{ContentStr: &content},
		}
		if msg.ToolCallID != "" {
			toolCallID := msg.ToolCallID
			bmsg.ChatToolMessage = &schemas.ChatToolMessage{ToolCallID: &toolCallID}
		}
		if len(msg.ToolCalls) > 0 {
			bmsg.ChatAssistantMessage = &schemas.ChatAssistantMessage{ToolCalls: bifrostToolCalls(msg.ToolCalls)}
		}
		out = append(out, bmsg)
	}
	return out
}

func bifrostTools(tools []toolSchema) []schemas.ChatTool {
	out := make([]schemas.ChatTool, 0, len(tools))
	for _, tool := range tools {
		description := tool.Description
		var params schemas.ToolFunctionParameters
		var paramsPtr *schemas.ToolFunctionParameters
		if strings.TrimSpace(tool.ParametersJSON) != "" {
			if err := json.Unmarshal([]byte(tool.ParametersJSON), &params); err == nil {
				paramsPtr = &params
			}
		}
		out = append(out, schemas.ChatTool{
			Type: schemas.ChatToolTypeFunction,
			Function: &schemas.ChatToolFunction{
				Name:        tool.Name,
				Description: &description,
				Parameters:  paramsPtr,
			},
		})
	}
	return out
}

func bifrostToolCalls(calls []toolCall) []schemas.ChatAssistantMessageToolCall {
	out := make([]schemas.ChatAssistantMessageToolCall, 0, len(calls))
	for _, call := range calls {
		id := call.ID
		callType := firstNonEmpty(call.Type, "function")
		name := call.Name
		out = append(out, schemas.ChatAssistantMessageToolCall{
			Index: uint16(call.Index),
			ID:    &id,
			Type:  &callType,
			Function: schemas.ChatAssistantMessageToolCallFunction{
				Name:      &name,
				Arguments: call.ArgumentsJSON,
			},
		})
	}
	return out
}

func toolCallsFromBifrost(calls []schemas.ChatAssistantMessageToolCall) []toolCall {
	out := make([]toolCall, 0, len(calls))
	for _, call := range calls {
		item := toolCall{
			Index:         int32(call.Index),
			ArgumentsJSON: call.Function.Arguments,
		}
		if call.ID != nil {
			item.ID = *call.ID
		}
		if call.Type != nil {
			item.Type = *call.Type
		}
		if call.Function.Name != nil {
			item.Name = *call.Function.Name
		}
		out = append(out, item)
	}
	return out
}

func cloneMap[T any](in map[string]T) map[string]T {
	if in == nil {
		return nil
	}
	out := make(map[string]T, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
