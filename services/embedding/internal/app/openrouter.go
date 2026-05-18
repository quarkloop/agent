package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type openRouterEmbedder struct {
	apiKey     string
	baseURL    string
	model      string
	dimensions int
	httpClient *http.Client
}

func (e *openRouterEmbedder) Embed(ctx context.Context, input, model string, dimensions int) (embeddingResult, error) {
	if strings.TrimSpace(model) == "" {
		model = e.model
	}
	reqBody := openRouterEmbeddingRequest{
		Model: model,
		Input: []openRouterEmbeddingInput{{
			Content: []openRouterEmbeddingContent{{
				Type: "text",
				Text: input,
			}},
		}},
		EncodingFormat: "float",
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return embeddingResult{}, providerError(CategoryInvalidConfig, e.Provider(), model, 0, fmt.Errorf("marshal request: %w", err))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/embeddings", bytes.NewReader(data))
	if err != nil {
		return embeddingResult{}, providerError(CategoryInvalidConfig, e.Provider(), model, 0, fmt.Errorf("create request: %w", err))
	}
	req.Header.Set("Authorization", "Bearer "+e.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return embeddingResult{}, providerError(CategoryTransport, e.Provider(), model, 0, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return embeddingResult{}, providerError(CategoryTransport, e.Provider(), model, resp.StatusCode, fmt.Errorf("read response: %w", err))
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return embeddingResult{}, providerError(categoryForHTTPStatus(resp.StatusCode), e.Provider(), model, resp.StatusCode, fmt.Errorf("%s", strings.TrimSpace(string(body))))
	}

	var out openRouterEmbeddingResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return embeddingResult{}, providerError(CategoryProviderResponse, e.Provider(), model, resp.StatusCode, fmt.Errorf("decode response: %w", err))
	}
	if out.Error != nil {
		return embeddingResult{}, providerError(categoryForProviderError(out.Error.Message), e.Provider(), model, resp.StatusCode, fmt.Errorf("%s", out.Error.Message))
	}
	if len(out.Data) == 0 || len(out.Data[0].Embedding) == 0 {
		return embeddingResult{}, providerError(CategoryProviderResponse, e.Provider(), model, resp.StatusCode, fmt.Errorf("response did not include an embedding"))
	}
	if dimensions > 0 && dimensions != len(out.Data[0].Embedding) {
		return embeddingResult{}, providerError(CategoryDimensionMismatch, e.Provider(), model, resp.StatusCode, fmt.Errorf("requested %d got %d", dimensions, len(out.Data[0].Embedding)))
	}
	if e.dimensions > 0 && e.dimensions != len(out.Data[0].Embedding) {
		return embeddingResult{}, providerError(CategoryDimensionMismatch, e.Provider(), model, resp.StatusCode, fmt.Errorf("configured %d got %d", e.dimensions, len(out.Data[0].Embedding)))
	}
	responseModel := strings.TrimSpace(out.Model)
	if responseModel == "" {
		responseModel = model
	}
	return embeddingResult{
		Vector:   cloneVector(out.Data[0].Embedding),
		Model:    responseModel,
		Provider: e.Provider(),
	}, nil
}

func (e *openRouterEmbedder) Provider() string { return "openrouter" }
func (e *openRouterEmbedder) Model() string    { return e.model }
func (e *openRouterEmbedder) Dimensions() int  { return e.dimensions }
func (e *openRouterEmbedder) Description() string {
	return "Create an OpenRouter provider-backed embedding vector for text."
}

type openRouterEmbeddingRequest struct {
	Model          string                     `json:"model"`
	Input          []openRouterEmbeddingInput `json:"input"`
	EncodingFormat string                     `json:"encoding_format"`
}

type openRouterEmbeddingInput struct {
	Content []openRouterEmbeddingContent `json:"content"`
}

type openRouterEmbeddingContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type openRouterEmbeddingResponse struct {
	Model string `json:"model"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
	Data []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
}

func categoryForProviderError(message string) ErrorCategory {
	normalized := strings.ToLower(message)
	switch {
	case strings.Contains(normalized, "api key") || strings.Contains(normalized, "auth"):
		return CategoryAuth
	case strings.Contains(normalized, "rate") || strings.Contains(normalized, "quota"):
		return CategoryQuota
	case strings.Contains(normalized, "not found") || strings.Contains(normalized, "unavailable"):
		return CategoryModelUnavailable
	case strings.Contains(normalized, "input length") || strings.Contains(normalized, "maximum allowed token"):
		return CategoryInvalidRequest
	default:
		return CategoryProviderResponse
	}
}
