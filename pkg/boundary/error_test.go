package boundary

import (
	"errors"
	"testing"

	"github.com/quarkloop/pkg/plugin"
)

func TestFromErrorMapsProviderError(t *testing.T) {
	err := plugin.NewProviderError(plugin.ProviderErrorRateLimit, "openrouter", "model-a", 429, errors.New("quota"))
	boundaryErr := FromError(Runtime, "chat", err)
	if boundaryErr.Boundary != Provider || boundaryErr.Category != RateLimit || boundaryErr.StatusCode != 429 {
		t.Fatalf("boundary error = %+v", boundaryErr)
	}
}

func TestFromHTTPStatusMapsCategories(t *testing.T) {
	if got := FromHTTPStatus(Supervisor, "GET /missing", 404, "missing").Category; got != NotFound {
		t.Fatalf("404 category = %s, want %s", got, NotFound)
	}
	if got := FromHTTPStatus(Service, "call", 429, "limited").Category; got != RateLimit {
		t.Fatalf("429 category = %s, want %s", got, RateLimit)
	}
}

func TestStreamPayloadReducesErrorToCategory(t *testing.T) {
	payload := StreamPayload(plugin.NewProviderError(plugin.ProviderErrorAuth, "openai", "gpt", 401, errors.New("nope")), Runtime, "message")
	if payload["boundary"] != string(Provider) || payload["category"] != string(Auth) {
		t.Fatalf("payload = %+v", payload)
	}
}
