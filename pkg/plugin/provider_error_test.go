package plugin

import (
	"errors"
	"testing"
)

func TestProviderErrorCategoryForHTTPStatus(t *testing.T) {
	tests := map[int]ProviderErrorCategory{
		401: ProviderErrorAuth,
		403: ProviderErrorAuth,
		404: ProviderErrorModelUnavailable,
		410: ProviderErrorModelUnavailable,
		413: ProviderErrorContextOverflow,
		429: ProviderErrorRateLimit,
		500: ProviderErrorTransport,
	}
	for status, want := range tests {
		if got := ProviderErrorCategoryForHTTPStatus(status); got != want {
			t.Fatalf("status %d category = %s, want %s", status, got, want)
		}
	}
}

func TestProviderErrorUnwrapsCause(t *testing.T) {
	cause := errors.New("quota exhausted")
	err := NewProviderError(ProviderErrorRateLimit, "openrouter", "model-a", 429, cause)
	if !errors.Is(err, cause) {
		t.Fatalf("provider error should unwrap cause")
	}
}
