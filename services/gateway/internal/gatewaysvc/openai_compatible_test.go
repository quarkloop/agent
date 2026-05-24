package gatewaysvc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAICompatibleProviderSendsMaxTokensOption(t *testing.T) {
	requests := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- payload
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider := newOpenAICompatibleProvider(ProviderConfig{
		ID:      "compatible",
		APIKey:  "test-key",
		BaseURL: server.URL,
		Model:   "test/model",
	})
	stream, err := provider.StreamGenerate(context.Background(), generateCommand{
		Messages: []message{{Role: "user", Content: []contentPart{{Kind: contentText, Text: "hello"}}}},
		Options:  map[string]string{optionMaxOutputTokens: "257"},
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	for event := range stream {
		if event.Err != nil {
			t.Fatalf("stream event: %v", event.Err)
		}
	}

	request := <-requests
	if got := request["max_tokens"]; got != float64(257) {
		t.Fatalf("max_tokens = %v", got)
	}
}

func TestOpenRouterProviderMapsMultimodalEmbeddingInput(t *testing.T) {
	requests := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- payload
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]}]}`))
	}))
	defer server.Close()

	provider := newOpenAICompatibleProvider(ProviderConfig{
		ID:             "openrouter",
		APIKey:         "test-key",
		BaseURL:        server.URL,
		EmbeddingModel: "nvidia/embed-vl",
	})
	_, err := provider.Embed(context.Background(), embedCommand{Inputs: []multimodalInput{{Content: []contentPart{
		{Kind: contentText, Text: "read the chart"},
		{Kind: contentImageURL, ImageURL: "https://example.test/chart.png"},
	}}}})
	if err != nil {
		t.Fatalf("embed: %v", err)
	}

	payload := <-requests
	inputs := payload["input"].([]any)
	content := inputs[0].(map[string]any)["content"].([]any)
	if len(content) != 2 || content[1].(map[string]any)["type"] != "image_url" {
		t.Fatalf("multimodal content payload = %+v", content)
	}
}

func TestOpenAICompatibleProviderRejectsMultimodalEmbeddingWithoutAdvertisedSupport(t *testing.T) {
	provider := newOpenAICompatibleProvider(ProviderConfig{
		ID:             "openai",
		APIKey:         "test-key",
		BaseURL:        "https://example.test",
		EmbeddingModel: "text/embed",
	})
	_, err := provider.Embed(context.Background(), embedCommand{Inputs: []multimodalInput{{Content: []contentPart{{
		Kind: contentImageURL, ImageURL: "https://example.test/image.png",
	}}}}})
	if err == nil {
		t.Fatal("expected unsupported multimodal provider error")
	}
}
