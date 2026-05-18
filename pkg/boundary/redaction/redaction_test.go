package redaction

import (
	"strings"
	"testing"
)

func TestRedactStringRemovesEnvSecretsAndCredentialPatterns(t *testing.T) {
	secret := "sk-or-v1-secret-value"
	t.Setenv("OPENROUTER_API_KEY", secret)

	input := `{"authorization":"Bearer ` + secret + `","nested":{"api_key":"sk-or-v1-another-secret"}}`
	got := RedactString(input)

	if strings.Contains(got, secret) || strings.Contains(got, "another-secret") {
		t.Fatalf("redaction leaked secret: %s", got)
	}
	for _, want := range []string{`Bearer [redacted]`, `api_key":"[redacted]`} {
		if !strings.Contains(got, want) {
			t.Fatalf("redaction missing %q in %s", want, got)
		}
	}
}

func TestLooksSensitiveKey(t *testing.T) {
	for _, key := range []string{"OPENROUTER_API_KEY", "authorization", "client_secret", "PASSWORD"} {
		if !LooksSensitiveKey(key) {
			t.Fatalf("expected sensitive key %q", key)
		}
	}
	for _, key := range []string{"PATH", "HOME", "QUARK_SPACE"} {
		if LooksSensitiveKey(key) {
			t.Fatalf("expected non-sensitive key %q", key)
		}
	}
}
