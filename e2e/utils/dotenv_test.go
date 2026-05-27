//go:build e2e

package utils

import "testing"

func TestRequireProviderConfigDefaultsToOpenRouterE2EModel(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("OPENROUTER_E2E_MODEL", "")
	t.Setenv("OPENROUTER_MODEL", "")

	cfg := RequireProviderConfig(t)
	if cfg.Provider != "openrouter" {
		t.Fatalf("provider = %q, want openrouter", cfg.Provider)
	}
	if cfg.Model != defaultE2EModel {
		t.Fatalf("model = %q, want %s", cfg.Model, defaultE2EModel)
	}
}

func TestAllowedE2EModelsAreLimitedToFinalGatePolicy(t *testing.T) {
	for _, model := range []string{
		"openrouter/owl-alpha",
		"nvidia/nemotron-3-super-120b-a12b:free",
		"deepseek/deepseek-v4-flash:free",
	} {
		if !allowedE2EModel(model) {
			t.Fatalf("configured E2E model %q was rejected", model)
		}
	}
	if allowedE2EModel("openrouter/not-approved") {
		t.Fatal("unapproved E2E model was accepted")
	}
}
