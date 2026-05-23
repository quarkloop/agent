package secretssvc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	secretsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/secrets/v1"
)

func TestOpenBaoClientResolveRotateAndLeaseLifecycle(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		if got := r.Header.Get("X-Vault-Token"); got != "root" {
			t.Fatalf("token header = %q", got)
		}
		switch r.URL.Path {
		case "/v1/secret/data/openrouter":
			switch r.Method {
			case http.MethodGet:
				_ = json.NewEncoder(w).Encode(map[string]any{
					"lease_id":       "lease-1",
					"lease_duration": 60,
					"renewable":      true,
					"data": map[string]any{
						"data": map[string]any{"api_key": "sk-test"},
					},
				})
			case http.MethodPost:
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"version": 3}})
			default:
				http.Error(w, "bad method", http.StatusMethodNotAllowed)
			}
		case "/v1/sys/leases/renew":
			_ = json.NewEncoder(w).Encode(map[string]any{"lease_id": "lease-1", "lease_duration": 120, "renewable": true})
		case "/v1/sys/leases/revoke":
			w.WriteHeader(http.StatusNoContent)
		case "/v1/auth/token/create":
			_ = json.NewEncoder(w).Encode(map[string]any{"auth": map[string]any{
				"client_token":   "scoped-token",
				"accessor":       "accessor-1",
				"policies":       []string{"gateway"},
				"lease_duration": 30,
				"renewable":      true,
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewOpenBaoClient(OpenBaoConfig{Address: server.URL, Token: "root", Mount: "secret"})
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	ref := SecretRef{Mount: "secret", Path: "openrouter", Field: "api_key"}
	secret, err := client.Resolve(context.Background(), ref, true)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if secret.GetValue() != "sk-test" || secret.GetLease().GetLeaseId() != "lease-1" {
		t.Fatalf("secret = %#v", secret)
	}
	version, err := client.Rotate(context.Background(), ref, "new-key", 2)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if version != 3 {
		t.Fatalf("version = %d", version)
	}
	lease, err := client.RenewLease(context.Background(), "lease-1", 120)
	if err != nil {
		t.Fatalf("renew: %v", err)
	}
	if lease.GetDurationSeconds() != 120 {
		t.Fatalf("lease = %#v", lease)
	}
	if err := client.RevokeLease(context.Background(), "lease-1", true); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	token, err := client.IssueToken(context.Background(), testIssueRequest())
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	if token.GetToken() != "scoped-token" || token.GetAccessor() != "accessor-1" {
		t.Fatalf("token = %#v", token)
	}
	if len(seen) != 5 {
		t.Fatalf("requests = %v", seen)
	}
}

func testIssueRequest() *secretsv1.IssueScopedSecretRequest {
	return &secretsv1.IssueScopedSecretRequest{Scope: "gateway", Policies: []string{"gateway"}, TtlSeconds: 30, Renewable: true}
}
