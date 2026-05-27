//go:build e2e

package utils

import (
	"testing"

	"github.com/quarkloop/pkg/natskit"
)

func TestServiceReadinessFunctionsResolveCanonicalSubjects(t *testing.T) {
	expected := map[string]string{
		"io":       "svc.io.v1.stat",
		"core":     "svc.core.v1.check_health",
		"gateway":  "svc.gateway.v1.provider_health",
		"indexer":  "svc.indexer.v1.query_context",
		"document": "svc.document.v1.detect_type",
		"runstate": "svc.runstate.v1.list_runs",
		"citation": "svc.citation.v1.score_coverage",
		"harness":  "svc.harness.v1.get_context_report",
		"devops":   "svc.devops.v1.repo_status",
		"system":   "svc.system.v1.snapshot",
		"workflow": "svc.workflow.v1.list",
		"secrets":  "svc.secrets.v1.audit_access",
	}
	for service, want := range expected {
		function, ok := serviceReadinessFunctions[service]
		if !ok {
			t.Fatalf("service readiness function missing for %q", service)
		}
		operation, err := natskit.ServiceOperationFromFunctionName(service, function)
		if err != nil {
			t.Fatalf("resolve %s/%s: %v", service, function, err)
		}
		if operation.Subject != want {
			t.Fatalf("service readiness subject for %q = %q, want %q", service, operation.Subject, want)
		}
	}
}
