package servicescmd

import (
	"strings"
	"testing"

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
	out := formatServiceInspect(api.ServiceInfo{
		Name:        "indexer",
		Status:      api.ServiceStatusUnavailable,
		Description: "Indexer",
		Functions: []api.ServiceFunctionInfo{{
			Name:    "indexer_GetContext",
			Service: "quark.indexer.v1.IndexerService",
			Method:  "GetContext",
		}},
		Diagnostics: []string{"health status is NOT_SERVING"},
	})
	for _, want := range []string{"indexer_GetContext", "Diagnostics", "NOT_SERVING"} {
		if !strings.Contains(out, want) {
			t.Fatalf("inspect missing %q:\n%s", want, out)
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
