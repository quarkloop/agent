//go:build e2e

package utils

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/quarkloop/pkg/natskit"
)

var serviceReadinessFunctions = map[string]string{
	"io":       "stat",
	"core":     "check_health",
	"gateway":  "provider_health",
	"indexer":  "query_context",
	"document": "detect_type",
	"runstate": "list_runs",
	"citation": "score_coverage",
	"harness":  "get_context_report",
	"devops":   "status",
	"system":   "snapshot",
	"workflow": "list",
	"secrets":  "audit_access",
}

// waitForServiceResponders ensures Compose containers have registered their
// NATS functions before runtime receives a catalog containing those services.
// The harmless empty read probes may return a domain validation error; a
// NATSKit response is sufficient to establish responder readiness.
func waitForServiceResponders(t *testing.T, env *E2EEnv, services []string, timeout time.Duration) {
	t.Helper()
	conn := connectControlNATS(t, env.NATS)
	defer conn.Close()
	for _, service := range uniqueNonEmpty(services) {
		function, ok := serviceReadinessFunctions[service]
		if !ok {
			t.Fatalf("no NATS readiness function declared for Compose service %q", service)
		}
		waitForServiceResponder(t, conn, env.Space, service, function, timeout)
	}
}

func waitForServiceResponder(t *testing.T, conn *natskit.Client, spaceID, service, function string, timeout time.Duration) {
	t.Helper()
	operation, err := natskit.ServiceOperation(service, function)
	if err != nil {
		t.Fatalf("service readiness operation %s/%s: %v", service, function, err)
	}
	payload, _ := json.Marshal(struct{}{})
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		request, err := natskit.NewRequest(natskit.NewServiceCallID(), spaceID, natskit.ActorRuntime, payload)
		if err != nil {
			t.Fatalf("service readiness request %s: %v", operation.Subject, err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		response, err := conn.Call(ctx, operation, request)
		cancel()
		if err == nil {
			Logf(t, "service-ready service=%s function=%s status=%s", service, function, response.Status)
			return
		}
		lastErr = err
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("service responder not ready subject=%s: %v", operation.Subject, lastErr)
}
