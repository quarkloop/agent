package servicekit_test

import (
	"strings"
	"testing"

	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
)

func TestRuntimeServiceCatalogRoundTrip(t *testing.T) {
	descriptors := []*servicev1.ServiceDescriptor{{
		Name:    "indexer",
		Type:    "indexer",
		Version: "1.0.0",
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:       "quark.indexer.v1.IndexerService",
			Method:        "QueryContext",
			Request:       "quark.indexer.v1.QueryRequest",
			Response:      "quark.indexer.v1.ContextResponse",
			Description:   "Retrieve context.",
			Owner:         "indexer",
			FunctionName:  "indexer_QueryContext",
			Subject:       "svc.indexer.v1.query_context",
			RiskLevel:     "read",
			Idempotent:    true,
			TimeoutMillis: 30000,
		}},
		Skills: []*servicev1.SkillDescriptor{{
			Name:     "service-indexer",
			Version:  "1.0.0",
			Markdown: "Use indexer.",
		}},
	}}

	payload, err := servicekit.MarshalRuntimeServiceCatalog(descriptors)
	if err != nil {
		t.Fatalf("marshal catalog: %v", err)
	}
	if !strings.Contains(string(payload), `"version":1`) {
		t.Fatalf("version missing from payload: %s", payload)
	}

	got, err := servicekit.UnmarshalRuntimeServiceCatalog(payload)
	if err != nil {
		t.Fatalf("unmarshal catalog: %v", err)
	}
	if len(got) != 1 || got[0].GetName() != "indexer" || got[0].GetRpcs()[0].GetMethod() != "QueryContext" {
		t.Fatalf("decoded descriptors = %+v", got)
	}
}

func TestRuntimeServiceCatalogRejectsUnsupportedVersion(t *testing.T) {
	_, err := servicekit.UnmarshalRuntimeServiceCatalog([]byte(`{"version":999,"services":[]}`))
	if err == nil || !strings.Contains(err.Error(), "unsupported runtime service catalog version") {
		t.Fatalf("expected unsupported version error, got: %v", err)
	}
}

func TestRuntimeServiceCatalogValidatesDescriptors(t *testing.T) {
	_, err := servicekit.MarshalRuntimeServiceCatalog([]*servicev1.ServiceDescriptor{{
		Name:    "indexer",
		Version: "1.0.0",
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:  "quark.indexer.v1.IndexerService",
			Method:   "QueryContext",
			Request:  "quark.indexer.v1.QueryRequest",
			Response: "quark.indexer.v1.ContextResponse",
		}},
	}})
	if err == nil || !strings.Contains(err.Error(), "missing description") {
		t.Fatalf("expected descriptor validation error, got: %v", err)
	}
}

func TestRuntimeServiceCatalogValidatesResolvedFunctionMetadata(t *testing.T) {
	_, err := servicekit.MarshalRuntimeServiceCatalog([]*servicev1.ServiceDescriptor{{
		Name:    "indexer",
		Version: "1.0.0",
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:     "quark.indexer.v1.IndexerService",
			Method:      "QueryContext",
			Request:     "quark.indexer.v1.QueryRequest",
			Response:    "quark.indexer.v1.ContextResponse",
			Description: "Retrieve context.",
			Owner:       "indexer",
			RiskLevel:   "read",
			Subject:     "svc.indexer.v1.query_context",
		}},
	}})
	if err == nil || !strings.Contains(err.Error(), "missing function name") {
		t.Fatalf("expected missing function name validation error, got: %v", err)
	}
}

func TestRuntimeServiceCatalogRequiresCanonicalSubject(t *testing.T) {
	_, err := servicekit.MarshalRuntimeServiceCatalog([]*servicev1.ServiceDescriptor{{
		Name:    "indexer",
		Version: "1.0.0",
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:      "quark.indexer.v1.IndexerService",
			Method:       "QueryContext",
			Request:      "quark.indexer.v1.QueryRequest",
			Response:     "quark.indexer.v1.ContextResponse",
			Description:  "Retrieve context.",
			Owner:        "indexer",
			FunctionName: "indexer_QueryContext",
			RiskLevel:    "read",
		}},
	}})
	if err == nil || !strings.Contains(err.Error(), "missing NATS subject") {
		t.Fatalf("expected missing subject error, got: %v", err)
	}
}
