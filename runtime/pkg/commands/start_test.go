package commands

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/runtime/pkg/agent"
	"github.com/quarkloop/runtime/pkg/permissions"
	"github.com/quarkloop/runtime/pkg/pluginmanager"
	runtimeservices "github.com/quarkloop/runtime/pkg/services"
)

func TestLoadPluginCatalogUsesEmptyCatalogWithoutEnv(t *testing.T) {
	t.Setenv("QUARK_SUPERVISOR_URL", "http://127.0.0.1:7200")
	t.Setenv("QUARK_SPACE", "test-space")
	t.Setenv("QUARK_RUNTIME_PLUGIN_CATALOG", "")

	catalog, err := loadPluginCatalog(nil)
	if err != nil {
		t.Fatalf("load plugin catalog: %v", err)
	}
	if catalog == nil {
		t.Fatal("expected empty supervisor-owned catalog, got nil")
	}
	if catalog.Version != plugin.RuntimeCatalogVersion {
		t.Fatalf("catalog version = %d", catalog.Version)
	}
	if !catalog.Empty() {
		t.Fatalf("expected empty catalog, got %+v", catalog)
	}
}

func TestLoadPluginCatalogRejectsUnsupportedVersion(t *testing.T) {
	t.Setenv("QUARK_RUNTIME_PLUGIN_CATALOG", `{"version":999,"plugins":[]}`)

	if _, err := loadPluginCatalog(nil); err == nil {
		t.Fatal("expected unsupported catalog version error")
	}
}

func TestLoadPluginCatalogPrefersNATSSnapshot(t *testing.T) {
	t.Setenv("QUARK_RUNTIME_PLUGIN_CATALOG", `{"version":999,"plugins":[]}`)
	payload, err := json.Marshal(plugin.NewRuntimeCatalog([]plugin.RuntimeCatalogPlugin{{
		Name: "quark-devops",
		Type: plugin.TypeAgent,
		Path: "/plugins/quark-devops",
		AgentProfile: &plugin.AgentProfile{
			ID:   "quark-devops",
			Name: "Quark DevOps",
		},
	}}))
	if err != nil {
		t.Fatalf("marshal catalog: %v", err)
	}
	catalog, err := loadPluginCatalog(&clientcontract.RuntimeCatalogResponse{PluginCatalog: payload})
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	if len(catalog.Plugins) != 1 || catalog.Plugins[0].Name != "quark-devops" {
		t.Fatalf("catalog = %+v", catalog)
	}
}

func TestLoadServiceCatalogPrefersNATSSnapshot(t *testing.T) {
	servicePayload, err := servicekit.MarshalRuntimeServiceCatalog([]*servicev1.ServiceDescriptor{{
		Name:    "indexer",
		Version: "1.0.0",
		Address: "127.0.0.1:7301",
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:      "quark.indexer.v1.IndexerService",
			Method:       "GetContext",
			Request:      "quark.indexer.v1.QueryRequest",
			Response:     "quark.indexer.v1.ContextResponse",
			Description:  "Retrieve context.",
			Owner:        "indexer",
			FunctionName: "indexer_GetContext",
			RiskLevel:    "read",
		}},
	}})
	if err != nil {
		t.Fatalf("marshal service catalog: %v", err)
	}
	catalog, err := loadServiceCatalog(&clientcontract.RuntimeCatalogResponse{
		ServiceCatalog: servicePayload,
		GeneratedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("load service catalog: %v", err)
	}
	if catalog == nil || len(catalog.ToolSchemas()) != 1 {
		t.Fatalf("service catalog = %+v", catalog)
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

func TestRegisterServiceFunctionsSkipsStreamingRPCs(t *testing.T) {
	a, err := agent.NewAgent(agent.Config{ID: "test-agent"})
	if err != nil {
		t.Fatalf("new agent: %v", err)
	}
	catalog := runtimeservices.NewCatalog([]*servicev1.ServiceDescriptor{{
		Name:    "model",
		Type:    "model",
		Address: "127.0.0.1:7306",
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:      "quark.model.v1.ModelService",
			Method:       "StreamGenerate",
			Request:      "quark.model.v1.StreamGenerateRequest",
			Response:     "quark.model.v1.StreamGenerateResponse",
			FunctionName: "model_StreamGenerate",
			Streaming:    true,
		}},
	}})

	registerServiceFunctions(a, catalog)

	if len(a.Plugins.GetTools()) != 0 {
		t.Fatalf("streaming service function was registered as a unary runtime tool: %+v", a.Plugins.GetTools())
	}
}

func TestModelProviderFromServiceUsesModelDescriptor(t *testing.T) {
	catalog := runtimeservices.NewCatalog([]*servicev1.ServiceDescriptor{{
		Name:    "model",
		Type:    "model",
		Address: "127.0.0.1:7306",
	}})
	if got := modelProviderFromService(catalog, "openrouter"); got == nil {
		t.Fatal("expected model service provider adapter")
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

func TestRuntimeAgentProfileMapsResolvedProfileWithoutAliasing(t *testing.T) {
	targets := []string{"quark-devops"}
	got := runtimeAgentProfile(pluginmanager.CatalogPlugin{
		SystemPrompt: "system",
		AgentProfile: &plugin.AgentProfile{
			ID:          "quark-knowledge",
			Name:        "Quark Knowledge",
			Description: "Knowledge profile",
			Handoff:     plugin.AgentProfileHandoff{CanDelegateTo: targets},
		},
	})
	targets[0] = "mutated"

	if got.ID != "quark-knowledge" || got.Name != "Quark Knowledge" || got.Description != "Knowledge profile" || got.SystemPrompt != "system" {
		t.Fatalf("runtime profile = %+v", got)
	}
	if len(got.HandoffTargets) != 1 || got.HandoffTargets[0] != "quark-devops" {
		t.Fatalf("handoff targets = %+v", got.HandoffTargets)
	}
}

func TestRuntimePermissionPolicyCombinesToolAndServiceFunctions(t *testing.T) {
	got := runtimePermissionPolicy(&plugin.AgentProfile{
		Permissions: plugin.AgentProfilePermission{
			Tools:    []string{"io_Read", "io_Read"},
			Services: []string{"indexer_QueryContext", "embedding_Embed"},
		},
	})
	if got == nil {
		t.Fatal("expected permission policy")
	}
	if !got.RestrictTools {
		t.Fatal("resolved agent profile policy must restrict tools to its allowlist")
	}
	want := []string{"io_Read", "indexer_QueryContext", "embedding_Embed"}
	if len(got.AllowedTools) != len(want) {
		t.Fatalf("allowed tools = %+v, want %+v", got.AllowedTools, want)
	}
	for i, name := range want {
		if got.AllowedTools[i] != name {
			t.Fatalf("allowed tools = %+v, want %+v", got.AllowedTools, want)
		}
	}
}

func TestRuntimePermissionPolicyAllowsFallbackAgentWhenProfileMissing(t *testing.T) {
	if got := runtimePermissionPolicy(nil); got != nil {
		t.Fatalf("fallback profile should not restrict tools, got %+v", got)
	}
}

func TestRuntimePermissionPolicyWithEmptyResolvedPermissionsDeniesAll(t *testing.T) {
	got := runtimePermissionPolicy(&plugin.AgentProfile{})
	if got == nil || !got.RestrictTools || len(got.AllowedTools) != 0 {
		t.Fatalf("expected restricted empty policy, got %+v", got)
	}
	if permissions.NewChecker(got).CanUseTool("io_Read") {
		t.Fatal("empty resolved permission set unexpectedly allowed tool call")
	}
}
