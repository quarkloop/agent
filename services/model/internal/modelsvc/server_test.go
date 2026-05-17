package modelsvc

import (
	"context"
	"strings"
	"testing"

	modelv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/model/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestGenerateUsesOrderedFallbackAndReturnsUsage(t *testing.T) {
	srv, err := NewServer(Config{
		Providers: []ProviderConfig{{ID: "local", Kind: "local", Model: "local/noop", Enabled: true}},
		Fallbacks: map[string][]string{"missing": []string{"local"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Generate(context.Background(), &modelv1.GenerateRequest{
		Provider: "missing",
		Model:    "local/noop",
		Messages: []*modelv1.ModelMessage{{Role: "user", Content: "summarize this"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(resp.GetText(), "summarize this") {
		t.Fatalf("unexpected text: %q", resp.GetText())
	}
	if resp.GetUsage().GetProvider() != "local" {
		t.Fatalf("usage provider = %q", resp.GetUsage().GetProvider())
	}
	if got := resp.GetUsage().GetFallbackChain(); len(got) != 2 || got[0] != "missing" || got[1] != "local" {
		t.Fatalf("fallback chain = %+v", got)
	}
}

func TestEmbedReturnsDeterministicVectorsAndUsage(t *testing.T) {
	srv, err := NewServer(Config{
		Providers: []ProviderConfig{{ID: "local", Kind: "local", Model: "local/embed", Enabled: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Embed(context.Background(), &modelv1.EmbedRequest{
		Provider:   "local",
		Model:      "local/embed",
		Input:      []string{"alpha", "beta"},
		Dimensions: 8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.GetEmbeddings()) != 2 {
		t.Fatalf("embeddings = %d, want 2", len(resp.GetEmbeddings()))
	}
	for _, embedding := range resp.GetEmbeddings() {
		if len(embedding.GetVector()) != 8 || embedding.GetProvider() != "local" || embedding.GetContentHash() == "" {
			t.Fatalf("bad embedding: %#v", embedding)
		}
	}
	if resp.GetUsage().GetEmbeddingTokens() == 0 || resp.GetUsage().GetRequestId() == "" {
		t.Fatalf("missing embedding usage: %#v", resp.GetUsage())
	}
}

func TestRerankCountTokensAndProviderHealth(t *testing.T) {
	srv, err := NewServer(Config{
		Providers: []ProviderConfig{{ID: "local", Kind: "local", Model: "local/noop", Enabled: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rerank, err := srv.Rerank(context.Background(), &modelv1.RerankRequest{
		Provider:  "local",
		Query:     "transformer attention",
		Documents: []string{"receipt total", "attention transformer paper"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rerank.GetResults()[0].GetIndex() != 1 {
		t.Fatalf("rerank results = %+v", rerank.GetResults())
	}
	count, err := srv.CountTokens(context.Background(), &modelv1.CountTokensRequest{
		Provider: "local",
		Messages: []*modelv1.ModelMessage{{Role: "user", Content: "hello world"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if count.GetTokens() == 0 || count.GetUsage().GetInputTokens() == 0 {
		t.Fatalf("token count = %#v", count)
	}
	health, err := srv.ProviderHealth(context.Background(), &modelv1.ProviderHealthRequest{Provider: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if !health.GetHealthy() || health.GetStatus() == "" {
		t.Fatalf("health = %#v", health)
	}
}

func TestUnsupportedProviderMapsToStructuredError(t *testing.T) {
	srv, err := NewServer(Config{
		Providers: []ProviderConfig{{ID: "anthropic", Kind: "unsupported", Model: "claude", Enabled: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = srv.Generate(context.Background(), &modelv1.GenerateRequest{
		Provider: "anthropic",
		Model:    "claude",
		Messages: []*modelv1.ModelMessage{{Role: "user", Content: "hi"}},
	})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("code = %s, err = %v", status.Code(err), err)
	}
}
