//go:build e2e

package utils

import (
	"slices"
	"strings"
	"testing"
)

func TestServiceProcessEnvDoesNotPropagateUndeclaredSecrets(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "sk-or-v1-process-secret")
	t.Setenv("UNDECLARED_SECRET", "must-not-leak")
	t.Setenv("PATH", "/bin")

	env := ServiceProcessEnv(map[string]string{"QUARK_INDEXER_ADDR": "127.0.0.1:7301"})

	if !slices.Contains(env, "PATH=/bin") || !slices.Contains(env, "QUARK_INDEXER_ADDR=127.0.0.1:7301") {
		t.Fatalf("expected base and override env entries, got %v", env)
	}
	if !slices.Contains(env, "QUARK_NATS_AUDIT_PREFIX=audit") || !slices.Contains(env, "QUARK_NATS_TELEMETRY_PREFIX=telemetry") {
		t.Fatalf("service env missing audit publication configuration: %v", env)
	}
	for _, entry := range env {
		if strings.Contains(entry, "sk-or-v1-process-secret") || strings.Contains(entry, "must-not-leak") {
			t.Fatalf("service env leaked secret: %v", env)
		}
	}
}

func TestSupervisorProcessEnvExcludesProviderSecretsAndCarriesOverrides(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "sk-or-v1-process-secret")
	t.Setenv("UNDECLARED_SECRET", "must-not-leak")
	t.Setenv("PATH", "/bin")

	env := SupervisorProcessEnv(map[string]string{"QUARK_SPACES_ROOT": "/tmp/spaces"})

	if !slices.Contains(env, "QUARK_SPACES_ROOT=/tmp/spaces") {
		t.Fatalf("supervisor env missing override: %v", env)
	}
	for _, entry := range env {
		if strings.Contains(entry, "sk-or-v1-process-secret") || strings.Contains(entry, "must-not-leak") {
			t.Fatalf("supervisor env leaked secret: %v", env)
		}
	}
}
