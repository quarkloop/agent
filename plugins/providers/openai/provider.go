//go:build plugin

// Package main implements the OpenAI provider plugin.
package main

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
)

// OpenAIProvider implements the ProviderPlugin interface for OpenAI.
type OpenAIProvider struct {
	manifest *plugin.Manifest
	apiKey   string
	baseURL  string
	client   *http.Client
}

func (p *OpenAIProvider) Name() string            { return p.manifest.Name }
func (p *OpenAIProvider) Version() string         { return p.manifest.Version }
func (p *OpenAIProvider) Type() plugin.PluginType { return p.manifest.Type }
func (p *OpenAIProvider) ProviderID() string      { return p.manifest.Name }

func (p *OpenAIProvider) Initialize(ctx context.Context, config map[string]any) error {
	manifest, err := plugin.ParseManifest(plugin.ManifestPathFromConfig(config))
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}
	p.manifest = manifest
	p.client = &http.Client{}
	if p.manifest.Provider != nil && p.manifest.Provider.APIBase != "" {
		p.baseURL = p.manifest.Provider.APIBase
	} else {
		p.baseURL = "https://api.openai.com/v1"
	}
	return nil
}

func (p *OpenAIProvider) Shutdown(ctx context.Context) error {
	return nil
}

func (p *OpenAIProvider) Configure(config plugin.ProviderConfig) error {
	p.apiKey = config.APIKey
	if config.BaseURL != "" {
		p.baseURL = config.BaseURL
	}
	return nil
}

func (p *OpenAIProvider) ListModels(ctx context.Context) ([]plugin.ModelInfo, error) {
	if p.manifest.Provider != nil && len(p.manifest.Provider.Models) > 0 {
		return p.manifest.Provider.Models, nil
	}
	return []plugin.ModelInfo{
		{ID: "gpt-4o", Name: "GPT-4o", ContextWindow: 128000, Default: true},
	}, nil
}

// ChatCompletionStream sends a streaming chat completion request to OpenAI.
func (p *OpenAIProvider) ChatCompletionStream(ctx context.Context, req *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
	oaiReq := &openAIRequest{
		Model:    req.Model,
		Messages: convertMessages(req.Messages),
		Stream:   true,
	}

	if len(req.Tools) > 0 {
		oaiReq.Tools = convertTools(req.Tools)
	}

	body, err := json.Marshal(oaiReq)
	if err != nil {
		return nil, plugin.NewProviderError(plugin.ProviderErrorInvalidRequest, p.ProviderID(), req.Model, 0, fmt.Errorf("marshal request: %w", err))
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, plugin.NewProviderError(plugin.ProviderErrorInvalidRequest, p.ProviderID(), req.Model, 0, fmt.Errorf("create request: %w", err))
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, plugin.NewProviderError(plugin.ProviderErrorTransport, p.ProviderID(), req.Model, 0, fmt.Errorf("send request: %w", err))
	}

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		providerErr := plugin.NewProviderError(plugin.ProviderErrorCategoryForHTTPStatus(resp.StatusCode), p.ProviderID(), req.Model, resp.StatusCode, fmt.Errorf("%s", strings.TrimSpace(string(errBody))))
		providerErr.ResetAt = resp.Header.Get("X-RateLimit-Reset")
		return nil, providerErr
	}

	ch := make(chan plugin.StreamEvent, 64)
	go p.readStream(resp.Body, ch)
	return ch, nil
}

// readStream parses the SSE stream into StreamEvents.
func (p *OpenAIProvider) readStream(body io.ReadCloser, ch chan<- plugin.StreamEvent) {
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
			ch <- plugin.StreamEvent{Done: true}
			return
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			ch <- plugin.StreamEvent{Err: fmt.Errorf("parse chunk: %w", err)}
			return
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		delta := chunk.Choices[0].Delta
		ch <- plugin.StreamEvent{
			Delta:     delta.Content,
			ToolCalls: convertToolCalls(delta.ToolCalls),
		}
	}

	if err := scanner.Err(); err != nil {
		ch <- plugin.StreamEvent{Err: fmt.Errorf("read stream: %w", err)}
	}
}

// ParseToolCalls - OpenAI uses native tool calling, so this is a no-op.
func (p *OpenAIProvider) ParseToolCalls(content string) ([]plugin.ToolCall, string) {
	return nil, content
}

// --- Request/Response types ---

type openAIRequest struct {
	Model    string       `json:"model"`
	Messages []openAIMsg  `json:"messages"`
	Tools    []openAITool `json:"tools,omitempty"`
	Stream   bool         `json:"stream"`
}

type openAIMsg struct {
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAITool struct {
	Type     string         `json:"type"`
	Function openAIToolFunc `json:"function"`
}

type openAIToolFunc struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type openAIToolCall struct {
	Index    int                `json:"index"`
	ID       string             `json:"id,omitempty"`
	Type     string             `json:"type,omitempty"`
	Function openAIToolCallFunc `json:"function"`
}

type openAIToolCallFunc struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

type streamChunk struct {
	Choices []streamChoice `json:"choices"`
}

type streamChoice struct {
	Delta streamDelta `json:"delta"`
}

type streamDelta struct {
	Content   string           `json:"content"`
	ToolCalls []openAIToolCall `json:"tool_calls"`
}

// --- Conversion helpers ---

func convertMessages(msgs []plugin.Message) []openAIMsg {
	out := make([]openAIMsg, len(msgs))
	for i, m := range msgs {
		out[i] = openAIMsg{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		if len(m.ToolCalls) > 0 {
			out[i].ToolCalls = make([]openAIToolCall, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				out[i].ToolCalls[j] = openAIToolCall{
					Index: tc.Index,
					ID:    tc.ID,
					Type:  tc.Type,
					Function: openAIToolCallFunc{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}
	}
	return out
}

func convertTools(schemas []plugin.ToolSchema) []openAITool {
	out := make([]openAITool, len(schemas))
	for i, s := range schemas {
		out[i] = openAITool{
			Type: "function",
			Function: openAIToolFunc{
				Name:        s.Name,
				Description: s.Description,
				Parameters:  s.Parameters,
			},
		}
	}
	return out
}

func convertToolCalls(tcs []openAIToolCall) []plugin.ToolCall {
	out := make([]plugin.ToolCall, len(tcs))
	for i, tc := range tcs {
		out[i] = plugin.ToolCall{
			Index: tc.Index,
			ID:    tc.ID,
			Type:  tc.Type,
			Function: plugin.ToolCallFunction{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		}
	}
	return out
}
