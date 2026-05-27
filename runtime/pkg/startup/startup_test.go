package startup

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/quarkloop/pkg/natskit"
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
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:      "quark.indexer.v1.IndexerService",
			Method:       "QueryContext",
			Request:      "quark.indexer.v1.QueryRequest",
			Response:     "quark.indexer.v1.ContextResponse",
			Description:  "Retrieve context.",
			Owner:        "indexer",
			FunctionName: "indexer_QueryContext",
			Subject:      "svc.indexer.v1.query_context",
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
		Name: "indexer",
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:     "quark.indexer.v1.IndexerService",
			Method:      "QueryContext",
			Request:     "quark.indexer.v1.QueryRequest",
			Response:    "quark.indexer.v1.ContextResponse",
			Subject:     "svc.indexer.v1.query_context",
			Description: "Retrieve context.",
		}},
	}})

	RegisterServiceFunctions(a, catalog)

	tools := a.Plugins.GetTools()
	if len(tools) != 1 || tools[0].Name != "indexer_QueryContext" {
		t.Fatalf("runtime tools = %+v", tools)
	}
	if !a.Plugins.IsLoaded("indexer_QueryContext") {
		t.Fatalf("service function was not registered as a runtime tool")
	}
}

func TestServicePromptMaterialsBindSkillsToDescriptorFunctionNames(t *testing.T) {
	catalog := runtimeservices.NewCatalog([]*servicev1.ServiceDescriptor{{
		Name:   "devops",
		Skills: []*servicev1.SkillDescriptor{{Markdown: "Use DevOps evidence."}},
		Rpcs: []*servicev1.RpcDescriptor{{
			FunctionName: "build_DetectProject",
		}},
	}})

	got := ServicePromptMaterials(catalog)
	if len(got) != 1 || len(got[0].ApplicableTools) != 1 || got[0].ApplicableTools[0] != "build_DetectProject" {
		t.Fatalf("service prompt material bindings = %+v", got)
	}
}

func TestRegisterServiceFunctionsSkipsStreamingRPCs(t *testing.T) {
	a, err := agent.NewAgent(agent.Config{ID: "test-agent"})
	if err != nil {
		t.Fatalf("new agent: %v", err)
	}
	catalog := runtimeservices.NewCatalog([]*servicev1.ServiceDescriptor{{
		Name: "gateway",
		Type: "gateway",
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:      "quark.gateway.v1.GatewayService",
			Method:       "StreamGenerate",
			Request:      "quark.gateway.v1.StreamGenerateRequest",
			Response:     "quark.gateway.v1.StreamGenerateResponse",
			FunctionName: "gateway_StreamGenerate",
			Streaming:    true,
			Subject:      "svc.gateway.v1.stream_generate",
		}},
	}})

	RegisterServiceFunctions(a, catalog)

	if len(a.Plugins.GetTools()) != 0 {
		t.Fatalf("streaming service function was registered as a unary runtime tool: %+v", a.Plugins.GetTools())
	}
}

func TestModelProviderFromServiceUsesGatewayDescriptor(t *testing.T) {
	catalog := runtimeservices.NewCatalog([]*servicev1.ServiceDescriptor{{
		Name: "gateway",
		Type: "gateway",
	}})
	if got := ModelProviderFromService(catalog, "openrouter"); got == nil {
		t.Fatal("expected gateway service provider adapter")
	}
}

type selectedModelFixture struct {
	models []plugin.ModelEntry
}

func (p selectedModelFixture) ListModels(context.Context) ([]plugin.ModelEntry, error) {
	return append([]plugin.ModelEntry(nil), p.models...), nil
}

func (selectedModelFixture) ChatCompletionStream(context.Context, *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
	return nil, nil
}

func (selectedModelFixture) ParseToolCalls(string) ([]plugin.ToolCall, string) {
	return nil, ""
}

func TestResolveSelectedGatewayModelRequiresSelectedContextMetadata(t *testing.T) {
	got, err := ResolveSelectedGatewayModel(context.Background(), selectedModelFixture{models: []plugin.ModelEntry{
		{ID: "other/model", Provider: "openrouter", ContextWindow: 1000},
		{ID: "selected/model", Provider: "openrouter", ContextWindow: 128000},
	}}, "selected/model")
	if err != nil {
		t.Fatalf("resolve selected model: %v", err)
	}
	if len(got) != 1 || got[0].ID != "selected/model" || got[0].ContextWindow != 128000 || !got[0].Default {
		t.Fatalf("selected models = %+v", got)
	}
}

func TestResolveSelectedGatewayModelRejectsUnknownContextWindow(t *testing.T) {
	_, err := ResolveSelectedGatewayModel(context.Background(), selectedModelFixture{models: []plugin.ModelEntry{{
		ID: "selected/model", Provider: "openrouter",
	}}}, "selected/model")
	if err == nil {
		t.Fatal("model with unknown context window unexpectedly resolved")
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

func TestSpecialistSkillMaterialsExposeOnlyInstalledDelegateGuidance(t *testing.T) {
	main := pluginmanager.CatalogPlugin{
		Name: "quark-main", Type: plugin.TypeAgent,
		AgentProfile: &plugin.AgentProfile{
			ID: "quark-main", Role: plugin.AgentProfileRoleMain,
			Handoff: plugin.AgentProfileHandoff{CanDelegateTo: []string{"quark-knowledge", "quark-system"}},
		},
	}
	catalog := &pluginmanager.Catalog{Plugins: []pluginmanager.CatalogPlugin{
		main,
		{Name: "quark-knowledge", Type: plugin.TypeAgent, Skill: "knowledge guidance",
			AgentProfile: &plugin.AgentProfile{ID: "quark-knowledge", Role: plugin.AgentProfileRoleDelegate}},
		{Name: "quark-devops", Type: plugin.TypeAgent, Skill: "unrelated guidance",
			AgentProfile: &plugin.AgentProfile{ID: "quark-devops", Role: plugin.AgentProfileRoleDelegate}},
	}}
	got := SpecialistSkillMaterials(catalog, main)
	if len(got) != 1 || got[0].SourceID != "plugin.agent.quark-knowledge.skill" || got[0].Content != "knowledge guidance" {
		t.Fatalf("specialist materials = %+v", got)
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
	provisionRuntimeLeaseBucket(t, ctx, ns.ClientURL(), "runtime_claim_test")
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

func provisionRuntimeLeaseBucket(t *testing.T, ctx context.Context, url, bucket string) {
	t.Helper()
	client, err := natskit.Connect(ctx, natskit.Config{URL: url, Name: "startup-test-setup"})
	if err != nil {
		t.Fatalf("connect provisioning client: %v", err)
	}
	defer client.Close()
	if _, err := client.EnsureKeyValue(natskit.KeyValueConfig{Bucket: bucket, TTL: time.Minute, History: 1}); err != nil {
		t.Fatalf("provision runtime lease bucket: %v", err)
	}
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
			Services: []string{"indexer_QueryContext", "gateway_Embed"},
		},
	})
	if got == nil {
		t.Fatal("expected permission policy")
	}
	if !got.RestrictTools {
		t.Fatal("resolved agent profile policy must restrict tools to its allowlist")
	}
	want := []string{"io_Read", "indexer_QueryContext", "gateway_Embed"}
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
