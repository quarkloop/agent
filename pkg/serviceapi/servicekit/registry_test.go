package servicekit_test

import (
	"context"
	"strings"
	"testing"

	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
)

func TestRegistryRejectsInvalidDescriptor(t *testing.T) {
	registry := servicekit.NewRegistry()
	err := registry.Register(&servicev1.ServiceDescriptor{
		Name:    "indexer",
		Version: "1.0.0",
		Address: "127.0.0.1:7301",
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:  "quark.indexer.v1.IndexerService",
			Method:   "GetContext",
			Request:  "quark.indexer.v1.QueryRequest",
			Response: "quark.indexer.v1.ContextResponse",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "missing description") {
		t.Fatalf("expected missing description validation error, got %v", err)
	}
}

func TestRegistryListServicesIsDeterministicAndCopied(t *testing.T) {
	registry := servicekit.NewRegistry()
	for _, desc := range []*servicev1.ServiceDescriptor{
		testDescriptor("space"),
		testDescriptor("indexer"),
	} {
		if err := registry.Register(desc); err != nil {
			t.Fatalf("register %s: %v", desc.GetName(), err)
		}
	}

	resp, err := registry.ListServices(context.Background(), nil)
	if err != nil {
		t.Fatalf("list services: %v", err)
	}
	if len(resp.GetServices()) != 2 || resp.GetServices()[0].GetName() != "indexer" || resp.GetServices()[1].GetName() != "space" {
		t.Fatalf("services not sorted: %+v", resp.GetServices())
	}

	resp.GetServices()[0].Name = "mutated"
	again, err := registry.ListServices(context.Background(), nil)
	if err != nil {
		t.Fatalf("list services again: %v", err)
	}
	if again.GetServices()[0].GetName() != "indexer" {
		t.Fatalf("registry returned mutable descriptor reference: %+v", again.GetServices())
	}
}

func testDescriptor(name string) *servicev1.ServiceDescriptor {
	return &servicev1.ServiceDescriptor{
		Name:    name,
		Type:    name,
		Version: "1.0.0",
		Address: "127.0.0.1:7301",
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:     "quark." + name + ".v1.Service",
			Method:      "Get",
			Request:     "quark." + name + ".v1.GetRequest",
			Response:    "quark." + name + ".v1.GetResponse",
			Description: "Read " + name + " state.",
		}},
	}
}
