package modelsvc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/quarkloop/pkg/plugin"
	modelv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/model/v1"
)

type openAICompatibleProvider struct {
	id      string
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

func newOpenAICompatibleProvider(cfg ProviderConfig) provider {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	return &openAICompatibleProvider{
		id:      strings.TrimSpace(cfg.ID),
		baseURL: normalizeOpenAICompatibleBaseURL(cfg.ID, baseURL),
		apiKey:  strings.TrimSpace(cfg.APIKey),
		model:   strings.TrimSpace(cfg.Model),
		client:  http.DefaultClient,
	}
}

func (p *openAICompatibleProvider) ID() string { return p.id }

func (p *openAICompatibleProvider) ListModels(context.Context) ([]*modelv1.ModelInfo, error) {
	model := p.model
	if model == "" {
		model = "default"
	}
	return []*modelv1.ModelInfo{{
		Id:            model,
		Provider:      p.id,
		Name:          model,
		ContextWindow: 128000,
		DefaultModel:  true,
	}}, nil
}

func (p *openAICompatibleProvider) StreamGenerate(ctx context.Context, cmd generateCommand) (<-chan streamEvent, error) {
	if p.apiKey == "" {
		return nil, plugin.NewProviderError(plugin.ProviderErrorAuth, p.id, cmd.Model, 0, fmt.Errorf("api key is required"))
	}
	if p.baseURL == "" {
		return nil, plugin.NewProviderError(plugin.ProviderErrorInvalidRequest, p.id, cmd.Model, 0, fmt.Errorf("base URL is required"))
	}
	reqBody := openAIChatRequest{
		Model:    firstNonEmpty(cmd.Model, p.model),
		Messages: openAIMessages(cmd.Messages),
		Tools:    openAITools(cmd.Tools),
		Stream:   true,
	}
	if maxOutputTokens, ok := maxOutputTokensOption(cmd.Options); ok {
		reqBody.MaxTokens = &maxOutputTokens
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, plugin.NewProviderError(plugin.ProviderErrorInvalidRequest, p.id, cmd.Model, 0, fmt.Errorf("marshal request: %w", err))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, plugin.NewProviderError(plugin.ProviderErrorInvalidRequest, p.id, cmd.Model, 0, fmt.Errorf("create request: %w", err))
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, plugin.NewProviderError(plugin.ProviderErrorTransport, p.id, cmd.Model, 0, fmt.Errorf("send request: %w", err))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, p.httpError(resp, cmd.Model)
	}
	ch := make(chan streamEvent, 64)
	go readOpenAIStream(resp.Body, ch)
	return ch, nil
}

func (p *openAICompatibleProvider) Embed(ctx context.Context, cmd embedCommand) ([]*modelv1.Embedding, error) {
	if p.apiKey == "" {
		return nil, plugin.NewProviderError(plugin.ProviderErrorAuth, p.id, cmd.Model, 0, fmt.Errorf("api key is required"))
	}
	body, err := json.Marshal(map[string]any{
		"model":           firstNonEmpty(cmd.Model, p.model),
		"input":           append([]string(nil), cmd.Input...),
		"encoding_format": "float",
	})
	if err != nil {
		return nil, plugin.NewProviderError(plugin.ProviderErrorInvalidRequest, p.id, cmd.Model, 0, fmt.Errorf("marshal embeddings request: %w", err))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, plugin.NewProviderError(plugin.ProviderErrorInvalidRequest, p.id, cmd.Model, 0, fmt.Errorf("create embeddings request: %w", err))
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, plugin.NewProviderError(plugin.ProviderErrorTransport, p.id, cmd.Model, 0, fmt.Errorf("send embeddings request: %w", err))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, p.httpError(resp, cmd.Model)
	}
	defer resp.Body.Close()
	var parsed openAIEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, plugin.NewProviderError(plugin.ProviderErrorResponse, p.id, cmd.Model, resp.StatusCode, fmt.Errorf("decode embeddings response: %w", err))
	}
	out := make([]*modelv1.Embedding, 0, len(parsed.Data))
	for i, item := range parsed.Data {
		input := ""
		if i < len(cmd.Input) {
			input = cmd.Input[i]
		}
		out = append(out, &modelv1.Embedding{
			Vector:      append([]float32(nil), item.Embedding...),
			Provider:    p.id,
			Model:       firstNonEmpty(cmd.Model, p.model),
			Dimensions:  int32(len(item.Embedding)),
			ContentHash: contentHash(input),
		})
	}
	return out, nil
}

func (p *openAICompatibleProvider) Health(context.Context) providerHealth {
	if p.apiKey == "" {
		return providerHealth{Healthy: false, Status: "missing api key"}
	}
	if p.baseURL == "" {
		return providerHealth{Healthy: false, Status: "missing base URL"}
	}
	return providerHealth{Healthy: true, Status: "configured"}
}

func (p *openAICompatibleProvider) httpError(resp *http.Response, model string) error {
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	providerErr := plugin.NewProviderError(plugin.ProviderErrorCategoryForHTTPStatus(resp.StatusCode), p.id, model, resp.StatusCode, fmt.Errorf("%s", strings.TrimSpace(string(data))))
	providerErr.ResetAt = resp.Header.Get("X-RateLimit-Reset")
	return providerErr
}

func readOpenAIStream(body io.ReadCloser, ch chan<- streamEvent) {
	defer close(ch)
	defer body.Close()
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			ch <- streamEvent{Done: true}
			return
		}
		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			ch <- streamEvent{Err: err}
			return
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		ch <- streamEvent{
			Delta:     chunk.Choices[0].Delta.Content,
			ToolCalls: toolCallsFromOpenAI(chunk.Choices[0].Delta.ToolCalls),
		}
	}
	if err := scanner.Err(); err != nil {
		ch <- streamEvent{Err: err}
	}
}

type openAIChatRequest struct {
	Model     string          `json:"model"`
	Messages  []openAIMessage `json:"messages"`
	Tools     []openAITool    `json:"tools,omitempty"`
	Stream    bool            `json:"stream"`
	MaxTokens *int            `json:"max_tokens,omitempty"`
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAITool struct {
	Type     string             `json:"type"`
	Function openAIToolFunction `json:"function"`
}

type openAIToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type openAIToolCall struct {
	Index    int32              `json:"index"`
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type,omitempty"`
	Function openAIToolCallFunc `json:"function"`
}

type openAIToolCallFunc struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type openAIStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content   string           `json:"content"`
			ToolCalls []openAIToolCall `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
}

type openAIEmbeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

func openAIMessages(messages []message) []openAIMessage {
	out := make([]openAIMessage, 0, len(messages))
	for _, msg := range messages {
		converted := openAIMessage{Role: msg.Role, Content: msg.Content, ToolCallID: msg.ToolCallID}
		for _, call := range msg.ToolCalls {
			converted.ToolCalls = append(converted.ToolCalls, openAIToolCall{
				Index: call.Index,
				ID:    call.ID,
				Type:  call.Type,
				Function: openAIToolCallFunc{
					Name:      call.Name,
					Arguments: call.ArgumentsJSON,
				},
			})
		}
		out = append(out, converted)
	}
	return out
}

func openAITools(tools []toolSchema) []openAITool {
	out := make([]openAITool, 0, len(tools))
	for _, tool := range tools {
		out = append(out, openAITool{
			Type: "function",
			Function: openAIToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  parametersMap(tool),
			},
		})
	}
	return out
}

func normalizeOpenAICompatibleBaseURL(providerID, baseURL string) string {
	if strings.EqualFold(strings.TrimSpace(providerID), "openrouter") && strings.HasSuffix(baseURL, "/api") {
		return baseURL + "/v1"
	}
	return baseURL
}

func toolCallsFromOpenAI(calls []openAIToolCall) []toolCall {
	out := make([]toolCall, 0, len(calls))
	for _, call := range calls {
		out = append(out, toolCall{
			Index:         call.Index,
			ID:            call.ID,
			Type:          call.Type,
			Name:          call.Function.Name,
			ArgumentsJSON: call.Function.Arguments,
		})
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
