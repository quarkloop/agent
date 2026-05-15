package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocalEmbedderUsesDeterministicMetadata(t *testing.T) {
	emb, err := newEmbedder(Config{Provider: "local"})
	if err != nil {
		t.Fatalf("new embedder: %v", err)
	}
	result, err := emb.Embed(context.Background(), "hello world", "", 0)
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if result.Provider != "local" || result.Model != "local-hash-v1" || len(result.Vector) != defaultDimensions {
		t.Fatalf("unexpected local result: provider=%s model=%s dimensions=%d", result.Provider, result.Model, len(result.Vector))
	}
}

func TestOpenRouterEmbedderCallsEmbeddingsEndpoint(t *testing.T) {
	var sawAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Fatalf("path = %s, want /embeddings", r.URL.Path)
		}
		sawAuth = r.Header.Get("Authorization") == "Bearer test-key"
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"model": "private/openrouter/test-model",
			"data": [{"embedding": [0.1, 0.2, 0.3]}]
		}`))
	}))
	defer srv.Close()

	emb, err := newEmbedder(Config{
		Provider:          "openrouter",
		Model:             "test-model",
		Dimensions:        3,
		OpenRouterAPIKey:  "test-key",
		OpenRouterBaseURL: srv.URL,
		HTTPClient:        srv.Client(),
	})
	if err != nil {
		t.Fatalf("new embedder: %v", err)
	}
	result, err := emb.Embed(context.Background(), "hello", "", 0)
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	if !sawAuth {
		t.Fatal("OpenRouter authorization header was not sent")
	}
	if result.Provider != "openrouter" || result.Model != "private/openrouter/test-model" || len(result.Vector) != 3 {
		t.Fatalf("unexpected OpenRouter result: %+v", result)
	}
}

func TestOpenRouterEmbedderRejectsDimensionMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": [{"embedding": [0.1, 0.2]}]}`))
	}))
	defer srv.Close()

	emb, err := newEmbedder(Config{
		Provider:          "openrouter",
		Model:             "test-model",
		Dimensions:        3,
		OpenRouterAPIKey:  "test-key",
		OpenRouterBaseURL: srv.URL,
		HTTPClient:        srv.Client(),
	})
	if err != nil {
		t.Fatalf("new embedder: %v", err)
	}
	if _, err := emb.Embed(context.Background(), "hello", "", 0); !isProviderCategory(err, CategoryDimensionMismatch) {
		t.Fatalf("expected dimension mismatch error, got %v", err)
	}
}

func TestEmbeddingFallbackUsesLocalAfterOpenRouterQuota(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "quota exhausted", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	emb, err := newEmbedder(Config{
		Provider:          "openrouter",
		Model:             "test-model",
		Dimensions:        4,
		Fallbacks:         []ProviderSpec{{Provider: "local", Dimensions: 4}},
		OpenRouterAPIKey:  "test-key",
		OpenRouterBaseURL: srv.URL,
		HTTPClient:        srv.Client(),
	})
	if err != nil {
		t.Fatalf("new embedder: %v", err)
	}
	result, err := emb.Embed(context.Background(), "hello fallback", "", 0)
	if err != nil {
		t.Fatalf("fallback embed: %v", err)
	}
	if result.Provider != "local" || result.Model != "local-hash-v1" || len(result.Vector) != 4 {
		t.Fatalf("fallback result = %+v", result)
	}
}

func TestEmbeddingFallbackDoesNotHideDimensionMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": [{"embedding": [0.1, 0.2]}]}`))
	}))
	defer srv.Close()

	emb, err := newEmbedder(Config{
		Provider:          "openrouter",
		Model:             "test-model",
		Dimensions:        4,
		Fallbacks:         []ProviderSpec{{Provider: "local", Dimensions: 4}},
		OpenRouterAPIKey:  "test-key",
		OpenRouterBaseURL: srv.URL,
		HTTPClient:        srv.Client(),
	})
	if err != nil {
		t.Fatalf("new embedder: %v", err)
	}
	if _, err := emb.Embed(context.Background(), "hello fallback", "", 0); !isProviderCategory(err, CategoryDimensionMismatch) {
		t.Fatalf("expected terminal dimension mismatch, got %v", err)
	}
}

func TestOpenRouterAuthErrorIsCategorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	emb, err := newEmbedder(Config{
		Provider:          "openrouter",
		Model:             "test-model",
		OpenRouterAPIKey:  "test-key",
		OpenRouterBaseURL: srv.URL,
		HTTPClient:        srv.Client(),
	})
	if err != nil {
		t.Fatalf("new embedder: %v", err)
	}
	if _, err := emb.Embed(context.Background(), "hello", "", 0); !isProviderCategory(err, CategoryAuth) {
		t.Fatalf("expected auth error, got %v", err)
	}
}

func TestParseProviderSpecs(t *testing.T) {
	specs, err := ParseProviderSpecs("openrouter|nvidia/llama-nemotron-embed-vl-1b-v2:free|2048,local||32")
	if err != nil {
		t.Fatalf("parse provider specs: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("specs = %+v", specs)
	}
	if specs[0].Provider != "openrouter" || specs[0].Model != "nvidia/llama-nemotron-embed-vl-1b-v2:free" || specs[0].Dimensions != 2048 {
		t.Fatalf("openrouter spec = %+v", specs[0])
	}
	if specs[1].Provider != "local" || specs[1].Model != "" || specs[1].Dimensions != 32 {
		t.Fatalf("local spec = %+v", specs[1])
	}
}

func TestOpenRouterMissingAPIKeyIsStartupAuthError(t *testing.T) {
	_, err := newEmbedder(Config{Provider: "openrouter", Model: "test-model"})
	if !isProviderCategory(err, CategoryAuth) {
		t.Fatalf("expected startup auth error, got %v", err)
	}
}

func isProviderCategory(err error, category ErrorCategory) bool {
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		return false
	}
	if providerErr.Category == category {
		return true
	}
	if providerErr.Category == CategoryProvidersExhausted && providerErr.Err != nil {
		return isProviderCategory(providerErr.Err, category)
	}
	unwrapped := errors.Unwrap(providerErr)
	if unwrapped == nil {
		return false
	}
	return isProviderCategory(unwrapped, category)
}
