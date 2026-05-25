package gatewaysvc

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/quarkloop/pkg/boundary"
	gatewayv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/gateway/v1"
)

func TestGenerateUsesOrderedFallbackAndReturnsUsage(t *testing.T) {
	srv := newConfiguredGatewayServer(t, map[string][]string{"missing": {"fixture"}})
	resp, err := srv.Generate(context.Background(), &gatewayv1.GenerateRequest{
		Provider: "missing",
		Model:    "fixture/chat",
		Messages: []*gatewayv1.ModelMessage{gatewayTextMessage("user", "summarize this")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.GetText(), "summarize this") {
		t.Fatalf("unexpected text: %q", resp.GetText())
	}
	if resp.GetUsage().GetProvider() != "fixture" {
		t.Fatalf("usage provider = %q", resp.GetUsage().GetProvider())
	}
	if got := resp.GetUsage().GetFallbackChain(); len(got) != 2 || got[0] != "missing" || got[1] != "fixture" {
		t.Fatalf("fallback chain = %+v", got)
	}
}

func TestEmbedUsesConfiguredProviderModelAndReturnsUsage(t *testing.T) {
	srv := newConfiguredGatewayServer(t, nil)
	resp, err := srv.Embed(context.Background(), &gatewayv1.EmbedRequest{
		Inputs: gatewayTextInputs("alpha", "beta"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetEmbeddings()) != 2 {
		t.Fatalf("embeddings = %d, want 2", len(resp.GetEmbeddings()))
	}
	for _, embedding := range resp.GetEmbeddings() {
		if len(embedding.GetVector()) != 3 || embedding.GetProvider() != "fixture" || embedding.GetModel() != "fixture/embed" || embedding.GetContentHash() == "" {
			t.Fatalf("bad embedding: %#v", embedding)
		}
	}
	if resp.GetUsage().GetEmbeddingTokens() == 0 || resp.GetUsage().GetRequestId() == "" {
		t.Fatalf("missing embedding usage: %#v", resp.GetUsage())
	}
}

func TestRerankCountTokensAndProviderHealth(t *testing.T) {
	srv := newConfiguredGatewayServer(t, nil)
	_, err := srv.Rerank(context.Background(), &gatewayv1.RerankRequest{
		Provider:  "fixture",
		Query:     "transformer attention",
		Documents: []string{"receipt total", "attention transformer paper"},
	})
	if !boundary.IsCategory(err, boundary.Unavailable) {
		t.Fatalf("rerank error = %v, want unavailable adapter diagnostic", err)
	}
	count, err := srv.CountTokens(context.Background(), &gatewayv1.CountTokensRequest{
		Provider: "fixture",
		Messages: []*gatewayv1.ModelMessage{gatewayTextMessage("user", "hello world")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if count.GetTokens() == 0 || count.GetUsage().GetInputTokens() == 0 {
		t.Fatalf("token count = %#v", count)
	}
	health, err := srv.ProviderHealth(context.Background(), &gatewayv1.ProviderHealthRequest{Provider: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if !health.GetHealthy() || health.GetStatus() == "" {
		t.Fatalf("health = %#v", health)
	}
}

func TestUsageSummaryAndReloadConfig(t *testing.T) {
	srv := newConfiguredGatewayServer(t, nil)
	if _, err := srv.CountTokens(context.Background(), &gatewayv1.CountTokensRequest{
		Provider: "fixture",
		Model:    "fixture/chat",
		Messages: []*gatewayv1.ModelMessage{gatewayTextMessage("user", "hello usage")},
	}); err != nil {
		t.Fatal(err)
	}
	summary, err := srv.UsageSummary(context.Background(), &gatewayv1.UsageSummaryRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(summary.GetUsage()) != 1 || summary.GetUsage()[0].GetProvider() != "fixture" || summary.GetUsage()[0].GetRequests() != 1 {
		t.Fatalf("usage summary = %+v", summary.GetUsage())
	}
	reload, err := srv.ReloadConfig(context.Background(), &gatewayv1.ReloadConfigRequest{
		Providers: []*gatewayv1.GatewayProviderConfig{{
			Id:             "fixture",
			Kind:           "openai-compatible",
			Model:          "fixture/reloaded",
			EmbeddingModel: "fixture/embed",
			Enabled:        true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reload.GetReloaded() || len(reload.GetProviders()) != 1 || reload.GetProviders()[0] != "fixture" {
		t.Fatalf("reload = %+v", reload)
	}
	models, err := srv.ListModels(context.Background(), &gatewayv1.ListModelsRequest{Provider: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if len(models.GetModels()) != 1 || models.GetModels()[0].GetId() != "fixture/reloaded" {
		t.Fatalf("models = %+v", models.GetModels())
	}
}

func TestUnsupportedProviderMapsToStructuredError(t *testing.T) {
	srv, err := NewServer(Config{
		Providers: []ProviderConfig{{ID: "anthropic", Kind: "unsupported", Model: "claude", Enabled: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = srv.Generate(context.Background(), &gatewayv1.GenerateRequest{
		Provider: "anthropic",
		Model:    "claude",
		Messages: []*gatewayv1.ModelMessage{gatewayTextMessage("user", "hi")},
	})
	if !boundary.IsCategory(err, boundary.Unavailable) {
		t.Fatalf("error = %v, want unavailable", err)
	}
}

func TestMissingProviderMapsToNotFoundDiagnostic(t *testing.T) {
	srv := newConfiguredGatewayServer(t, nil)
	_, err := srv.ProviderHealth(context.Background(), &gatewayv1.ProviderHealthRequest{Provider: "missing"})
	if !boundary.IsCategory(err, boundary.NotFound) {
		t.Fatalf("error = %v, want not found", err)
	}
}

func TestBifrostProviderInitializesAndClosesWithoutNetworkCall(t *testing.T) {
	p, err := newProvider(ProviderConfig{
		ID:      "openrouter",
		Kind:    "bifrost",
		APIKey:  "test-key",
		BaseURL: "https://openrouter.ai/api/v1",
		Model:   "openai/test",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("new bifrost provider: %v", err)
	}
	health := p.Health(context.Background())
	if !health.Healthy {
		t.Fatalf("health = %+v", health)
	}
	closer, ok := p.(closableProvider)
	if !ok {
		t.Fatal("bifrost provider does not close cleanly")
	}
	if err := closer.Close(); err != nil {
		t.Fatalf("close bifrost provider: %v", err)
	}
}

func newConfiguredGatewayServer(t *testing.T, fallbacks map[string][]string) *Server {
	t.Helper()
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat/completions":
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"summarize this\"}}]}\n\n")
			fmt.Fprint(w, "data: [DONE]\n\n")
		case "/embeddings":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"embedding":[0.1,0.2,0.3]},{"embedding":[0.4,0.5,0.6]}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(endpoint.Close)
	srv, err := NewServer(Config{
		Providers: []ProviderConfig{{
			ID:             "fixture",
			Kind:           "openai-compatible",
			APIKey:         "test-key",
			BaseURL:        endpoint.URL,
			Model:          "fixture/chat",
			EmbeddingModel: "fixture/embed",
			Enabled:        true,
		}},
		Fallbacks:         fallbacks,
		EmbeddingProvider: "fixture",
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return srv
}

func gatewayTextMessage(role, text string) *gatewayv1.ModelMessage {
	return &gatewayv1.ModelMessage{
		Role: role,
		Content: []*gatewayv1.ContentPart{{
			Kind: gatewayv1.ContentKind_CONTENT_KIND_TEXT,
			Text: text,
		}},
	}
}

func gatewayTextInputs(values ...string) []*gatewayv1.MultimodalInput {
	out := make([]*gatewayv1.MultimodalInput, 0, len(values))
	for _, value := range values {
		out = append(out, &gatewayv1.MultimodalInput{Content: []*gatewayv1.ContentPart{{
			Kind: gatewayv1.ContentKind_CONTENT_KIND_TEXT,
			Text: value,
		}}})
	}
	return out
}
