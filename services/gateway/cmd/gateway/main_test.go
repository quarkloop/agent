package main

import (
	"testing"
	"time"
)

func TestProviderConfigsFromEnvUsesOpenRouterCompatibleAdapterByDefault(t *testing.T) {
	setProviderEnv(t)
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("OPENROUTER_EMBEDDING_MODEL", "openrouter/embed-test")

	cfg := findProviderConfig(t, "openrouter")
	if cfg.Kind != "openai-compatible" {
		t.Fatalf("openrouter kind = %q", cfg.Kind)
	}
	if cfg.BaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("openrouter base url = %q", cfg.BaseURL)
	}
	if cfg.EmbeddingModel != "openrouter/embed-test" {
		t.Fatalf("openrouter embedding model = %q", cfg.EmbeddingModel)
	}
}

func TestProviderConfigsFromEnvCanSelectBifrostOpenRouterAdapter(t *testing.T) {
	setProviderEnv(t)
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("QUARK_OPENROUTER_PROVIDER_KIND", "bifrost")

	cfg := findProviderConfig(t, "openrouter")
	if cfg.Kind != "bifrost" {
		t.Fatalf("openrouter kind = %q", cfg.Kind)
	}
	if cfg.BaseURL != "https://openrouter.ai/api" {
		t.Fatalf("openrouter base url = %q", cfg.BaseURL)
	}
}

func TestDurationEnvOrDefaultParsesPositiveDuration(t *testing.T) {
	t.Setenv("QUARK_GATEWAY_TIMEOUT", "2m")

	got := durationEnvOrDefault("QUARK_GATEWAY_TIMEOUT", time.Second)
	if got != 2*time.Minute {
		t.Fatalf("duration = %s", got)
	}
}

func TestInt64EnvOrDefaultAcceptsNonNegativeLimit(t *testing.T) {
	t.Setenv("QUARK_GATEWAY_MAX_EXTERNAL_REQUESTS", "7")
	if got := int64EnvOrDefault("QUARK_GATEWAY_MAX_EXTERNAL_REQUESTS", 0); got != 7 {
		t.Fatalf("external request limit = %d", got)
	}
	t.Setenv("QUARK_GATEWAY_MAX_EXTERNAL_REQUESTS", "-1")
	if got := int64EnvOrDefault("QUARK_GATEWAY_MAX_EXTERNAL_REQUESTS", 0); got != 0 {
		t.Fatalf("negative external request limit = %d, want fallback", got)
	}
}

func setProviderEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"OPENROUTER_API_KEY",
		"OPENROUTER_BASE_URL",
		"OPENROUTER_MODEL",
		"OPENROUTER_EMBEDDING_MODEL",
		"QUARK_OPENROUTER_PROVIDER_KIND",
		"OPENAI_API_KEY",
		"ANTHROPIC_API_KEY",
	} {
		t.Setenv(key, "")
	}
}

func findProviderConfig(t *testing.T, id string) ProviderConfigView {
	t.Helper()
	for _, cfg := range providerConfigsFromEnv() {
		if cfg.ID == id {
			return ProviderConfigView{ID: cfg.ID, Kind: cfg.Kind, BaseURL: cfg.BaseURL, EmbeddingModel: cfg.EmbeddingModel}
		}
	}
	t.Fatalf("provider %q not found", id)
	return ProviderConfigView{}
}

type ProviderConfigView struct {
	ID             string
	Kind           string
	BaseURL        string
	EmbeddingModel string
}
