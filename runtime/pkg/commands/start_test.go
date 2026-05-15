package commands

import (
	"testing"

	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/runtime/pkg/agent"
	runtimeservices "github.com/quarkloop/runtime/pkg/services"
)

func TestLoadPluginCatalogUsesEmptyCatalogWithoutEnv(t *testing.T) {
	t.Setenv("QUARK_SUPERVISOR_URL", "http://127.0.0.1:7200")
	t.Setenv("QUARK_SPACE", "test-space")
	t.Setenv("QUARK_RUNTIME_PLUGIN_CATALOG", "")

	catalog, err := loadPluginCatalog()
	if err != nil {
		t.Fatalf("load plugin catalog: %v", err)
	}
	if catalog == nil {
		t.Fatal("expected empty supervisor-owned catalog, got nil")
	}
	if !catalog.Empty() {
		t.Fatalf("expected empty catalog, got %+v", catalog)
	}
}

func TestRegisterServiceFunctionsUsesRuntimeToolPath(t *testing.T) {
	a, err := agent.NewAgent(agent.Config{ID: "test-agent"})
	if err != nil {
		t.Fatalf("new agent: %v", err)
	}
	catalog := runtimeservices.NewCatalog([]*servicev1.ServiceDescriptor{{
		Name:    "indexer",
		Address: "127.0.0.1:7301",
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:     "quark.indexer.v1.IndexerService",
			Method:      "GetContext",
			Request:     "quark.indexer.v1.QueryRequest",
			Response:    "quark.indexer.v1.ContextResponse",
			Description: "Retrieve context.",
		}},
	}})

	registerServiceFunctions(a, catalog)

	tools := a.Plugins.GetTools()
	if len(tools) != 1 || tools[0].Name != "indexer_GetContext" {
		t.Fatalf("runtime tools = %+v", tools)
	}
	if !a.Plugins.IsLoaded("indexer_GetContext") {
		t.Fatalf("service function was not registered as a runtime tool")
	}
}
