package commands

import (
	"testing"

	"github.com/quarkloop/pkg/plugin"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/runtime/pkg/agent"
	"github.com/quarkloop/runtime/pkg/pluginmanager"
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

func TestResolveAgentPluginSelectsRequestedProfile(t *testing.T) {
	catalog := &pluginmanager.Catalog{Plugins: []pluginmanager.CatalogPlugin{
		{
			Name: "quark-devops",
			Type: plugin.TypeAgent,
			AgentProfile: &plugin.AgentProfile{
				ID:   "quark-devops",
				Name: "Quark DevOps",
			},
			SystemPrompt: "devops",
		},
		{
			Name: "quark-knowledge",
			Type: plugin.TypeAgent,
			AgentProfile: &plugin.AgentProfile{
				ID:   "quark-knowledge",
				Name: "Quark Knowledge",
			},
			SystemPrompt: "knowledge",
		},
	}}

	got, err := resolveAgentPlugin(catalog, "quark-knowledge")
	if err != nil {
		t.Fatalf("resolve agent profile: %v", err)
	}
	if got.AgentProfile == nil || got.AgentProfile.ID != "quark-knowledge" || got.SystemPrompt != "knowledge" {
		t.Fatalf("selected profile = %+v", got)
	}
}

func TestResolveAgentPluginDefaultsDeterministically(t *testing.T) {
	catalog := &pluginmanager.Catalog{Plugins: []pluginmanager.CatalogPlugin{
		{
			Name:         "quark-system",
			Type:         plugin.TypeAgent,
			AgentProfile: &plugin.AgentProfile{ID: "quark-system", Name: "Quark System"},
		},
		{
			Name:         "quark-devops",
			Type:         plugin.TypeAgent,
			AgentProfile: &plugin.AgentProfile{ID: "quark-devops", Name: "Quark DevOps"},
		},
	}}

	got, err := resolveAgentPlugin(catalog, "")
	if err != nil {
		t.Fatalf("resolve agent profile: %v", err)
	}
	if got.AgentProfile == nil || got.AgentProfile.ID != "quark-devops" {
		t.Fatalf("default profile = %+v", got)
	}
}

func TestResolveAgentPluginRejectsUnknownRequestedProfile(t *testing.T) {
	catalog := &pluginmanager.Catalog{Plugins: []pluginmanager.CatalogPlugin{{
		Name:         "quark-system",
		Type:         plugin.TypeAgent,
		AgentProfile: &plugin.AgentProfile{ID: "quark-system", Name: "Quark System"},
	}}}

	if _, err := resolveAgentPlugin(catalog, "missing"); err == nil {
		t.Fatal("resolve agent profile unexpectedly succeeded")
	}
}

func TestResolveModelSelectionPrefersResolvedAgentProfile(t *testing.T) {
	provider, model := resolveModelSelection(
		&plugin.AgentProfile{Model: plugin.AgentProfileModel{Provider: "openrouter", Model: "openai/gpt-5-mini"}},
		"anthropic",
		"claude-sonnet-4",
	)
	if provider != "openrouter" || model != "openai/gpt-5-mini" {
		t.Fatalf("model = %s/%s", provider, model)
	}
}

func TestResolveModelSelectionFallsBackToEnvironment(t *testing.T) {
	provider, model := resolveModelSelection(
		&plugin.AgentProfile{Model: plugin.AgentProfileModel{Provider: "openrouter"}},
		"anthropic",
		"claude-sonnet-4",
	)
	if provider != "anthropic" || model != "claude-sonnet-4" {
		t.Fatalf("model = %s/%s", provider, model)
	}
}
