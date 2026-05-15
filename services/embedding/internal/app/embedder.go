package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type embedder interface {
	Embed(ctx context.Context, input, model string, dimensions int) (embeddingResult, error)
	Provider() string
	Model() string
	Dimensions() int
	Description() string
}

type embeddingResult struct {
	Vector   []float32
	Model    string
	Provider string
}

type fallbackEmbedder struct {
	embedders []embedder
}

func (e fallbackEmbedder) Embed(ctx context.Context, input, model string, dimensions int) (embeddingResult, error) {
	failures := make([]error, 0, len(e.embedders))
	for _, candidate := range e.embedders {
		result, err := candidate.Embed(ctx, input, modelForCandidate(model, candidate), dimensionsForCandidate(dimensions, candidate))
		if err == nil {
			return result, nil
		}
		failures = append(failures, err)
		if !fallbackAllowed(err) {
			return embeddingResult{}, err
		}
	}
	return embeddingResult{}, providerError(CategoryProvidersExhausted, e.Provider(), model, 0, errors.Join(failures...))
}

func (e fallbackEmbedder) Provider() string {
	if len(e.embedders) == 0 {
		return ""
	}
	return e.embedders[0].Provider()
}

func (e fallbackEmbedder) Model() string {
	if len(e.embedders) == 0 {
		return ""
	}
	return e.embedders[0].Model()
}

func (e fallbackEmbedder) Dimensions() int {
	if len(e.embedders) == 0 {
		return 0
	}
	return e.embedders[0].Dimensions()
}

func (e fallbackEmbedder) Description() string {
	providers := make([]string, 0, len(e.embedders))
	for _, candidate := range e.embedders {
		providers = append(providers, candidate.Provider())
	}
	return "Create an embedding vector with ordered provider fallback: " + strings.Join(providers, " -> ") + "."
}

func modelForCandidate(requestModel string, candidate embedder) string {
	if strings.TrimSpace(requestModel) == "" {
		return ""
	}
	if candidate.Provider() == "local" {
		return ""
	}
	return requestModel
}

func dimensionsForCandidate(requestDimensions int, candidate embedder) int {
	if requestDimensions > 0 {
		return requestDimensions
	}
	return candidate.Dimensions()
}

func newEmbedder(cfg Config) (embedder, error) {
	specs := normalizeProviderSpecs(ProviderSpec{
		Provider:   cfg.Provider,
		Model:      cfg.Model,
		Dimensions: cfg.Dimensions,
	}, cfg.Fallbacks)
	embedders := make([]embedder, 0, len(specs))
	for _, spec := range specs {
		embedder, err := newProviderEmbedder(cfg, spec)
		if err != nil {
			return nil, err
		}
		embedders = append(embedders, embedder)
	}
	if len(embedders) == 1 {
		return embedders[0], nil
	}
	return fallbackEmbedder{embedders: embedders}, nil
}

func newProviderEmbedder(cfg Config, spec ProviderSpec) (embedder, error) {
	provider := normalizeProvider(spec.Provider)
	switch provider {
	case "local":
		dimensions := spec.Dimensions
		if dimensions <= 0 {
			dimensions = defaultDimensions
		}
		model := strings.TrimSpace(spec.Model)
		if model == "" {
			model = "local-hash-v1"
		}
		return localEmbedder{model: model, dimensions: dimensions}, nil
	case "openrouter":
		if strings.TrimSpace(cfg.OpenRouterAPIKey) == "" {
			return nil, providerError(CategoryAuth, provider, spec.Model, 0, fmt.Errorf("OpenRouter API key is required"))
		}
		model := strings.TrimSpace(spec.Model)
		if model == "" {
			return nil, providerError(CategoryInvalidConfig, provider, "", 0, fmt.Errorf("OpenRouter embedding model is required"))
		}
		baseURL := strings.TrimRight(strings.TrimSpace(cfg.OpenRouterBaseURL), "/")
		if baseURL == "" {
			baseURL = defaultOpenRouterBaseURL
		}
		httpClient := cfg.HTTPClient
		if httpClient == nil {
			httpClient = &http.Client{}
		}
		return &openRouterEmbedder{
			apiKey:     cfg.OpenRouterAPIKey,
			baseURL:    baseURL,
			model:      model,
			dimensions: spec.Dimensions,
			httpClient: httpClient,
		}, nil
	default:
		return nil, providerError(CategoryInvalidConfig, provider, spec.Model, 0, fmt.Errorf("unsupported embedding provider %q", spec.Provider))
	}
}
