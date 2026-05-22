package servicescmd

import (
	"strings"
	"testing"
	"time"

	"github.com/quarkloop/supervisor/pkg/api"
)

func TestFormatServiceTable(t *testing.T) {
	out := formatServiceTable([]api.ServiceInfo{{
		Name:          "indexer",
		Status:        api.ServiceStatusReady,
		Version:       "1.0.0",
		Endpoint:      "127.0.0.1:7301",
		FunctionCount: 2,
	}})
	for _, want := range []string{"NAME", "indexer", "ready", "127.0.0.1:7301"} {
		if !strings.Contains(out, want) {
			t.Fatalf("table missing %q:\n%s", want, out)
		}
	}
}

func TestFormatServiceInspectIncludesDiagnostics(t *testing.T) {
	started := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	out := formatServiceInspect(api.ServiceInfo{
		Name:        "indexer",
		Status:      api.ServiceStatusUnavailable,
		Description: "Indexer",
		StartedAt:   &started,
		Functions: []api.ServiceFunctionInfo{{
			Name:    "indexer_GetContext",
			Service: "quark.indexer.v1.IndexerService",
			Method:  "GetContext",
		}},
		Diagnostics: []string{"health status is NOT_SERVING"},
	})
	for _, want := range []string{"indexer_GetContext", "Diagnostics", "NOT_SERVING", "2026-05-17T10:00:00Z"} {
		if !strings.Contains(out, want) {
			t.Fatalf("inspect missing %q:\n%s", want, out)
		}
	}
}

func TestServicesCommandExcludesLifecycleCommands(t *testing.T) {
	cmd := NewServicesCommand()
	names := make(map[string]bool)
	for _, child := range cmd.Commands() {
		names[child.Name()] = true
	}
	for _, want := range []string{"list", "status", "inspect", "doctor"} {
		if !names[want] {
			t.Fatalf("services command missing %q", want)
		}
	}
	for _, removed := range []string{"logs", "start", "stop", "restart"} {
		if names[removed] {
			t.Fatalf("services command still exposes supervisor-owned lifecycle command %q", removed)
		}
	}
}

func TestFormatServiceDoctor(t *testing.T) {
	out := formatServiceDoctor(api.ServiceDoctorResponse{
		Services: []api.ServiceInfo{{Name: "indexer", Status: api.ServiceStatusMissing}},
		Issues:   []string{"indexer: missing endpoint"},
	})
	if !strings.Contains(out, "Issues:") || !strings.Contains(out, "missing endpoint") {
		t.Fatalf("doctor output missing issues:\n%s", out)
	}
}
