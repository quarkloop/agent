//go:build e2e

package utils

import "testing"

func TestCfgForTestDefaultsToOpenRouterE2EModel(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "test-key")
	t.Setenv("OPENROUTER_E2E_MODEL", "")
	t.Setenv("OPENROUTER_MODEL", "")
	t.Setenv("ANTHROPIC_DEFAULT_SONNET_MODEL", "")

	cfg, ok := CfgForTest(t, "OPENROUTER_API_KEY")
	if !ok {
		t.Fatal("expected provider config")
	}
	if cfg.Provider != "openrouter" {
		t.Fatalf("provider = %q, want openrouter", cfg.Provider)
	}
	if cfg.Model != "openai/gpt-4o-mini" {
		t.Fatalf("model = %q, want openai/gpt-4o-mini", cfg.Model)
	}
}

func TestIsRateLimitTextRecognizesProviderQuotaAndCreditFailures(t *testing.T) {
	for _, msg := range []string{
		"status=402: this request requires more credits",
		"quota exhausted",
		"HTTP 429 rate limit",
	} {
		if !IsRateLimitText(msg) {
			t.Fatalf("IsRateLimitText(%q) = false", msg)
		}
	}
}
