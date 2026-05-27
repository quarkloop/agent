//go:build e2e

package utils

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDumpNATSCLIDiagnosticsWritesInspectableArtifacts(t *testing.T) {
	script := filepath.Join(t.TempDir(), "nats")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'diagnostic-output\\n'\n"), 0o755); err != nil {
		t.Fatalf("write fake nats CLI: %v", err)
	}
	t.Setenv("NATS_CLI", script)
	dir := t.TempDir()
	DumpNATSCLIDiagnostics(t, NATSEndpoints{ClientURL: "nats://test.invalid:4222"}, "unit", nil, dir)
	data, err := os.ReadFile(filepath.Join(dir, "nats-unit-account-info.txt"))
	if err != nil {
		t.Fatalf("read NATS diagnostic artifact: %v", err)
	}
	if !strings.Contains(string(data), "diagnostic-output") {
		t.Fatalf("NATS diagnostic artifact missing probe output: %s", data)
	}
}

func TestServiceSubscriptionProbesFollowActiveCanonicalFunctions(t *testing.T) {
	probes := serviceSubscriptionProbes(natsCLIIdentity{}, []string{"devops", "gateway"})
	got := make(map[string]string, len(probes))
	for _, probe := range probes {
		for i, arg := range probe.Args {
			if arg == "--filter-subject" && i+1 < len(probe.Args) {
				got[probe.Label] = probe.Args[i+1]
			}
		}
	}
	if got["svc-devops-subscriptions"] != "svc.devops.v1.repo_status" {
		t.Fatalf("devops probe = %q, want canonical repo status subject", got["svc-devops-subscriptions"])
	}
	if got["svc-gateway-subscriptions"] != "svc.gateway.v1.provider_health" ||
		got["svc-gateway-embedding-subscriptions"] != "svc.gateway.v1.embed" {
		t.Fatalf("gateway probes = %+v", got)
	}
	if _, ok := got["svc-indexer-subscriptions"]; ok {
		t.Fatalf("inactive indexer subject was probed: %+v", got)
	}
}
