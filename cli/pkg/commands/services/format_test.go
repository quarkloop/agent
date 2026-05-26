package servicescmd

import (
	"strings"
	"testing"

	"github.com/quarkloop/pkg/serviceapi/clientcontract"
)

func TestFormatServiceTable(t *testing.T) {
	out := formatServiceTable([]clientcontract.ServiceInfo{{
		Name:          "indexer",
		Status:        clientcontract.ServiceStatusReady,
		Version:       "1.0.0",
		SubjectPrefix: "svc.indexer.v1",
		FunctionCount: 2,
	}})
	for _, want := range []string{"NAME", "SUBJECT PREFIX", "indexer", "ready", "svc.indexer.v1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("table missing %q:\n%s", want, out)
		}
	}
}

func TestFormatServiceInspectIncludesDiagnostics(t *testing.T) {
	out := formatServiceInspect(clientcontract.ServiceInfo{
		Name:          "indexer",
		Status:        clientcontract.ServiceStatusUnavailable,
		Description:   "Indexer",
		SubjectPrefix: "svc.indexer.v1",
		Functions: []clientcontract.ServiceFunctionInfo{{
			Name:    "indexer_QueryContext",
			Service: "quark.indexer.v1.IndexerService",
			Method:  "QueryContext",
		}},
		Diagnostics: []string{"health status is NOT_SERVING"},
	})
	for _, want := range []string{"indexer_QueryContext", "Diagnostics", "NOT_SERVING", "svc.indexer.v1.*"} {
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
	out := formatServiceDoctor(clientcontract.ServiceDoctorResponse{
		Services: []clientcontract.ServiceInfo{{Name: "indexer", Status: clientcontract.ServiceStatusMissing}},
		Issues:   []string{"indexer: missing endpoint"},
	})
	if !strings.Contains(out, "Issues:") || !strings.Contains(out, "missing endpoint") {
		t.Fatalf("doctor output missing issues:\n%s", out)
	}
}
