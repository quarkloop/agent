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
		Messages: []message{{Role: "user", Content: "hello"}},
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
