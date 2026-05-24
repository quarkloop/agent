package startup

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
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

	catalog, err := LoadPluginCatalog(nil)
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

func TestLoadPluginCatalogRejectsUnsupportedSnapshotVersion(t *testing.T) {
	snapshot := &clientcontract.RuntimeCatalogResponse{
		PluginCatalog: json.RawMessage(`{"version":999,"plugins":[]}`),
	}

	if _, err := LoadPluginCatalog(snapshot); err == nil {
		t.Fatal("expected unsupported catalog version error")
	}
}

func TestLoadPluginCatalogPrefersNATSSnapshot(t *testing.T) {
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
	catalog, err := LoadPluginCatalog(&clientcontract.RuntimeCatalogResponse{PluginCatalog: payload})
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
	catalog, err := LoadServiceCatalog(&clientcontract.RuntimeCatalogResponse{
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

	RegisterServiceFunctions(a, catalog)

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
		Name:    "gateway",
		Type:    "gateway",
		Address: "127.0.0.1:7306",
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:      "quark.gateway.v1.GatewayService",
			Method:       "StreamGenerate",
			Request:      "quark.gateway.v1.StreamGenerateRequest",
			Response:     "quark.gateway.v1.StreamGenerateResponse",
			FunctionName: "gateway_StreamGenerate",
			Streaming:    true,
		}},
	}})

	RegisterServiceFunctions(a, catalog)

	if len(a.Plugins.GetTools()) != 0 {
		t.Fatalf("streaming service function was registered as a unary runtime tool: %+v", a.Plugins.GetTools())
	}
}

func TestModelProviderFromServiceUsesGatewayDescriptor(t *testing.T) {
	catalog := runtimeservices.NewCatalog([]*servicev1.ServiceDescriptor{{
		Name:    "gateway",
		Type:    "gateway",
		Address: "127.0.0.1:7306",
	}})
	if got := ModelProviderFromService(catalog, "openrouter"); got == nil {
		t.Fatal("expected gateway service provider adapter")
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
				Role: plugin.AgentProfileRoleDelegate,
			},
			SystemPrompt: "devops",
		},
		{
			Name: "quark-main",
			Type: plugin.TypeAgent,
			AgentProfile: &plugin.AgentProfile{
				ID:   "quark-main",
				Name: "Quark Main",
				Role: plugin.AgentProfileRoleMain,
			},
			SystemPrompt: "main",
		},
	}}

	got, err := ResolveAgentPlugin(catalog, "quark-main")
	if err != nil {
		t.Fatalf("resolve agent profile: %v", err)
	}
	if got.AgentProfile == nil || got.AgentProfile.ID != "quark-main" || got.SystemPrompt != "main" {
		t.Fatalf("selected profile = %+v", got)
	}
}

func TestResolveAgentPluginDefaultsToMainProfile(t *testing.T) {
	catalog := &pluginmanager.Catalog{Plugins: []pluginmanager.CatalogPlugin{
		{
			Name:         "quark-system",
			Type:         plugin.TypeAgent,
			AgentProfile: &plugin.AgentProfile{ID: "quark-system", Name: "Quark System", Role: plugin.AgentProfileRoleDelegate},
		},
		{
			Name:         "quark-main",
			Type:         plugin.TypeAgent,
			AgentProfile: &plugin.AgentProfile{ID: "quark-main", Name: "Quark Main", Role: plugin.AgentProfileRoleMain},
		},
	}}

	got, err := ResolveAgentPlugin(catalog, "")
	if err != nil {
		t.Fatalf("resolve agent profile: %v", err)
	}
	if got.AgentProfile == nil || got.AgentProfile.ID != "quark-main" {
		t.Fatalf("default profile = %+v", got)
	}
}

func TestResolveAgentPluginRejectsUnknownRequestedProfile(t *testing.T) {
	catalog := &pluginmanager.Catalog{Plugins: []pluginmanager.CatalogPlugin{{
		Name:         "quark-system",
		Type:         plugin.TypeAgent,
		AgentProfile: &plugin.AgentProfile{ID: "quark-system", Name: "Quark System", Role: plugin.AgentProfileRoleDelegate},
	}}}

	if _, err := ResolveAgentPlugin(catalog, "missing"); err == nil {
		t.Fatal("resolve agent profile unexpectedly succeeded")
	}
}

func TestResolveAgentPluginRejectsDelegateAsRoot(t *testing.T) {
	catalog := &pluginmanager.Catalog{Plugins: []pluginmanager.CatalogPlugin{{
		Name:         "quark-devops",
		Type:         plugin.TypeAgent,
		AgentProfile: &plugin.AgentProfile{ID: "quark-devops", Name: "Quark DevOps", Role: plugin.AgentProfileRoleDelegate},
	}}}

	if _, err := ResolveAgentPlugin(catalog, "quark-devops"); err == nil {
		t.Fatal("delegate profile unexpectedly resolved as root")
	}
}

func TestResolveAgentPluginRejectsMissingMainProfile(t *testing.T) {
	catalog := &pluginmanager.Catalog{Plugins: []pluginmanager.CatalogPlugin{{
		Name:         "quark-system",
		Type:         plugin.TypeAgent,
		AgentProfile: &plugin.AgentProfile{ID: "quark-system", Name: "Quark System", Role: plugin.AgentProfileRoleDelegate},
	}}}

	if _, err := ResolveAgentPlugin(catalog, ""); err == nil {
		t.Fatal("missing main profile unexpectedly resolved")
	}
}

func TestResolveModelSelectionPrefersResolvedAgentProfile(t *testing.T) {
	provider, model := ResolveModelSelection(
		&plugin.AgentProfile{Model: plugin.AgentProfileModel{Provider: "openrouter", Model: "openai/gpt-5-mini"}},
		"anthropic",
		"claude-sonnet-4",
	)
	if provider != "openrouter" || model != "openai/gpt-5-mini" {
		t.Fatalf("model = %s/%s", provider, model)
	}
}

func TestRuntimeSpacesFromEnvDeduplicatesConfiguredSpaces(t *testing.T) {
	t.Setenv("QUARK_SPACES", "space-a, space-b,space-a")
	t.Setenv("QUARK_SPACE", "space-fallback")
	got := SpacesFromEnv()
	if len(got) != 2 || got[0] != "space-a" || got[1] != "space-b" {
		t.Fatalf("spaces = %#v", got)
	}
}

func TestClaimRuntimeSpacesRejectsCompetingRuntime(t *testing.T) {
	ns := startRuntimeLeaseNATS(t)
	defer ns.Shutdown()
	ctx := context.Background()
	t.Setenv("QUARK_NATS_URL", ns.ClientURL())
	t.Setenv("QUARK_RUNTIME_LEASE_BUCKET", "runtime_claim_test")
	t.Setenv("QUARK_RUNTIME_ID", "runtime-1")
	manager, leases, err := ClaimRuntimeSpaces(ctx, []string{"space-a"})
	if err != nil {
		t.Fatalf("claim first runtime: %v", err)
	}
	defer ReleaseRuntimeSpaces(ctx, leases, manager)

	t.Setenv("QUARK_RUNTIME_ID", "runtime-2")
	if _, _, err := ClaimRuntimeSpaces(ctx, []string{"space-a"}); err == nil {
		t.Fatal("expected competing runtime claim to fail")
	}
}

func startRuntimeLeaseNATS(t *testing.T) *natsserver.Server {
	t.Helper()
	ns, err := natsserver.NewServer(&natsserver.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
		NoLog:     true,
		NoSigs:    true,
	})
	if err != nil {
		t.Fatalf("new nats server: %v", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(5 * time.Second) {
		ns.Shutdown()
		t.Fatal("nats server did not become ready")
	}
	return ns
}

func TestResolveModelSelectionFallsBackToEnvironment(t *testing.T) {
	provider, model := ResolveModelSelection(
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
	got := AgentProfile(pluginmanager.CatalogPlugin{
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
	got := PermissionPolicy(&plugin.AgentProfile{
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
	if got := PermissionPolicy(nil); got != nil {
		t.Fatalf("fallback profile should not restrict tools, got %+v", got)
	}
}

func TestRuntimePermissionPolicyWithEmptyResolvedPermissionsDeniesAll(t *testing.T) {
	got := PermissionPolicy(&plugin.AgentProfile{})
	if got == nil || !got.RestrictTools || len(got.AllowedTools) != 0 {
		t.Fatalf("expected restricted empty policy, got %+v", got)
	}
	if permissions.NewChecker(got).CanUseTool("io_Read") {
		t.Fatal("empty resolved permission set unexpectedly allowed tool call")
	}
}
