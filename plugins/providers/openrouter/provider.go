//go:build plugin

// Package main implements the OpenRouter provider plugin.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/quarkloop/pkg/plugin"
)

// OpenRouterProvider implements the ProviderPlugin interface for OpenRouter.
type OpenRouterProvider struct {
	manifest *plugin.Manifest
	apiKey   string
	baseURL  string
	client   *http.Client
}

func (p *OpenRouterProvider) Name() string            { return p.manifest.Name }
func (p *OpenRouterProvider) Version() string         { return p.manifest.Version }
func (p *OpenRouterProvider) Type() plugin.PluginType { return p.manifest.Type }
func (p *OpenRouterProvider) ProviderID() string      { return p.manifest.Name }

func (p *OpenRouterProvider) Initialize(ctx context.Context, config map[string]any) error {
	manifest, err := plugin.ParseManifest(plugin.ManifestPathFromConfig(config))
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}
	p.manifest = manifest
	p.client = &http.Client{}
	if p.manifest.Provider != nil && p.manifest.Provider.APIBase != "" {
		p.baseURL = p.manifest.Provider.APIBase
	} else {
		p.baseURL = "https://openrouter.ai/api/v1"
	}
	return nil
}

func (p *OpenRouterProvider) Shutdown(ctx context.Context) error {
	return nil
}

func (p *OpenRouterProvider) Configure(config plugin.ProviderConfig) error {
	p.apiKey = config.APIKey
	if config.BaseURL != "" {
		p.baseURL = config.BaseURL
	}
	return nil
}

func (p *OpenRouterProvider) ListModels(ctx context.Context) ([]plugin.ModelInfo, error) {
	if p.manifest.Provider != nil && len(p.manifest.Provider.Models) > 0 {
		return p.manifest.Provider.Models, nil
	}
	return []plugin.ModelInfo{
		{ID: "openai/gpt-4o", Name: "GPT-4o", ContextWindow: 128000, Default: true},
	}, nil
}

// ChatCompletionStream sends a streaming chat completion request to OpenRouter.
func (p *OpenRouterProvider) ChatCompletionStream(ctx context.Context, req *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
	orReq := &openRouterRequest{
		Model:    req.Model,
		Messages: convertMessages(req.Messages),
		Stream:   true,
	}

	if len(req.Tools) > 0 {
		orReq.Tools = convertTools(req.Tools)
	}

	body, err := json.Marshal(orReq)
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
func (p *OpenRouterProvider) readStream(body io.ReadCloser, ch chan<- plugin.StreamEvent) {
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

// ParseToolCalls extracts <tool_call> JSON blocks from the content.
func (p *OpenRouterProvider) ParseToolCalls(content string) ([]plugin.ToolCall, string) {
	re := regexp.MustCompile(`(?s)<tool_call>(.*?)</tool_call>`)
	matches := re.FindAllStringSubmatch(content, -1)

	var calls []plugin.ToolCall
	for i, m := range matches {
		if len(m) < 2 {
			continue
		}

		var fn struct {
			Name      string `json:"name"`
			Arguments any    `json:"arguments"`
		}

		if err := json.Unmarshal([]byte(m[1]), &fn); err != nil {
			continue
		}

		argBytes, _ := json.Marshal(fn.Arguments)
		calls = append(calls, plugin.ToolCall{
			Index: i,
			ID:    fmt.Sprintf("call_%d_%d", time.Now().UnixNano(), i),
			Type:  "function",
			Function: plugin.ToolCallFunction{
				Name:      fn.Name,
				Arguments: string(argBytes),
			},
		})
	}

	cleanedContent := re.ReplaceAllString(content, "")
	return calls, cleanedContent
}

// --- Request/Response types ---

type openRouterRequest struct {
	Model    string           `json:"model"`
	Messages []openRouterMsg  `json:"messages"`
	Tools    []openRouterTool `json:"tools,omitempty"`
	Stream   bool             `json:"stream"`
}

type openRouterMsg struct {
	Role       string               `json:"role"`
	Content    string               `json:"content"`
	ToolCalls  []openRouterToolCall `json:"tool_calls,omitempty"`
	ToolCallID string               `json:"tool_call_id,omitempty"`
}

type openRouterTool struct {
	Type     string             `json:"type"`
	Function openRouterToolFunc `json:"function"`
}

type openRouterToolFunc struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type openRouterToolCall struct {
	Index    int                    `json:"index"`
	ID       string                 `json:"id,omitempty"`
	Type     string                 `json:"type,omitempty"`
	Function openRouterToolCallFunc `json:"function"`
}

type openRouterToolCallFunc struct {
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
	Content   string               `json:"content"`
	ToolCalls []openRouterToolCall `json:"tool_calls"`
}

// --- Conversion helpers ---

func convertMessages(msgs []plugin.Message) []openRouterMsg {
	out := make([]openRouterMsg, len(msgs))
	for i, m := range msgs {
		out[i] = openRouterMsg{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
		}
		if len(m.ToolCalls) > 0 {
			out[i].ToolCalls = make([]openRouterToolCall, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				out[i].ToolCalls[j] = openRouterToolCall{
					Index: tc.Index,
					ID:    tc.ID,
					Type:  tc.Type,
					Function: openRouterToolCallFunc{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			}
		}
	}
	return out
}

func convertTools(schemas []plugin.ToolSchema) []openRouterTool {
	out := make([]openRouterTool, len(schemas))
	for i, s := range schemas {
		out[i] = openRouterTool{
			Type: "function",
			Function: openRouterToolFunc{
				Name:        s.Name,
				Description: s.Description,
				Parameters:  s.Parameters,
			},
		}
	}
	return out
}

func convertToolCalls(tcs []openRouterToolCall) []plugin.ToolCall {
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
