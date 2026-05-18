package boundary

import (
	"errors"
	"testing"

	"github.com/quarkloop/pkg/plugin"
)

func TestDiagnosticFromProviderAuthError(t *testing.T) {
	diag := DiagnosticFromError(plugin.NewProviderError(plugin.ProviderErrorAuth, "openrouter", "model", 401, errors.New("bad key")), Runtime, "message")
	if diag.Code != "provider.auth" || diag.Severity != "action_required" || diag.Boundary != string(Provider) {
		t.Fatalf("diagnostic = %+v", diag)
	}
	if diag.Hint == "" {
		t.Fatalf("expected diagnostic hint: %+v", diag)
	}
}

func TestDiagnosticFromPolicyDeniedError(t *testing.T) {
	diag := DiagnosticFromError(New(Runtime, PolicyDenied, "tool.indexer_QueryContext", "not allowed"), Runtime, "message")
	if diag.Code != "runtime.policy_denied" || diag.Severity != "action_required" {
		t.Fatalf("diagnostic = %+v", diag)
	}
	if diag.Operation != "tool.indexer_QueryContext" || diag.Hint == "" {
		t.Fatalf("diagnostic missing operation or hint: %+v", diag)
	}
}
