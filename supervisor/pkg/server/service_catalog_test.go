package server

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/plugin"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	spacemodel "github.com/quarkloop/pkg/space"
	"github.com/quarkloop/supervisor/pkg/natshub"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestRuntimePluginCatalogEntryIncludesToolSchemaAndSkill(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("Use the tool carefully.\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	entry, err := runtimePluginCatalogEntryFromInstalled(pluginmanager.InstalledPlugin{
		Path: dir,
		Manifest: &plugin.Manifest{
			Name: "build-release",
			Type: plugin.TypeTool,
			Tool: &plugin.ToolConfig{
				Schema: plugin.ToolSchema{
					Name:        "build-release",
					Description: "build release compatibility",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("catalog entry: %v", err)
	}

	if entry.Name != "build-release" || entry.Type != plugin.TypeTool || entry.Path != dir {
		t.Fatalf("unexpected entry identity: %+v", entry)
	}
	if entry.Schema == nil || entry.Schema.Name != "build-release" {
		t.Fatalf("tool schema missing: %+v", entry)
	}
	if entry.Skill != "Use the tool carefully." {
		t.Fatalf("skill = %q", entry.Skill)
	}
}

func TestRuntimePluginCatalogEntryIncludesAgentProfile(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"PROFILE.yaml": "id: quark-knowledge\nname: Quark Knowledge\n",
		"SYSTEM.md":    "You are Quark Knowledge.\n",
		"SKILL.md":     "Use Knowledge services.\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	manifest := &plugin.Manifest{
		Name: "quark-knowledge",
		Type: plugin.TypeAgent,
		Agent: &plugin.AgentConfig{
			Profile: "PROFILE.yaml",
			System:  "SYSTEM.md",
			Skill:   "SKILL.md",
		},
	}
	entry, err := runtimePluginCatalogEntryFromInstalled(pluginmanager.InstalledPlugin{
		Path:     dir,
		Manifest: manifest,
	})
	if err != nil {
		t.Fatalf("catalog entry: %v", err)
	}
	if entry.AgentProfile == nil || entry.AgentProfile.ID != "quark-knowledge" {
		t.Fatalf("agent profile missing: %+v", entry.AgentProfile)
	}
	if entry.SystemPrompt != "You are Quark Knowledge." {
		t.Fatalf("system prompt = %q", entry.SystemPrompt)
	}
	if entry.Skill != "Use Knowledge services." {
		t.Fatalf("skill = %q", entry.Skill)
	}
}

func TestRuntimePluginCatalogUsesVersionedContract(t *testing.T) {
	catalog := plugin.NewRuntimeCatalog([]runtimePluginCatalogEntry{{
		Name: "quark-knowledge",
		Type: plugin.TypeAgent,
		Path: "/plugins/quark-knowledge",
		AgentProfile: &plugin.AgentProfile{
			ID:   "quark-knowledge",
			Name: "Quark Knowledge",
		},
	}})
	if err := catalog.Validate(); err != nil {
		t.Fatalf("catalog should validate: %v", err)
	}
	if catalog.Version != plugin.RuntimeCatalogVersion {
		t.Fatalf("catalog version = %d", catalog.Version)
	}
}

func TestRuntimeCatalogSnapshotReturnsVersionedPayloads(t *testing.T) {
	srv := serviceTestServer(t)
	writeInstalledServicePlugin(t, srv, "test-space")
	qf := []byte(`quark: "1.0"
meta:
  name: test-space
  version: "0.1.0"
plugins:
  - ref: quark/service-indexer
services:
  - name: indexer
    ref: quark/service-indexer
    mode: local
    address_env: QUARK_INDEXER_ADDR
`)
	if _, err := srv.store.UpdateQuarkfile("test-space", qf); err != nil {
		t.Fatalf("update quarkfile: %v", err)
	}
	address, stop := startRegistryService(t)
	defer stop()
	t.Setenv("QUARK_INDEXER_ADDR", address)

	snapshot, err := srv.RuntimeCatalogSnapshot(t.Context(), "test-space")
	if err != nil {
		t.Fatalf("runtime catalog snapshot: %v", err)
	}
	if snapshot.SpaceID != "test-space" || snapshot.GeneratedAt.IsZero() {
		t.Fatalf("snapshot identity = %#v", snapshot)
	}
	if !strings.Contains(string(snapshot.PluginCatalog), `"version":1`) {
		t.Fatalf("plugin catalog payload = %s", string(snapshot.PluginCatalog))
	}
	services, err := servicekit.UnmarshalRuntimeServiceCatalog(snapshot.ServiceCatalog)
	if err != nil {
		t.Fatalf("unmarshal service catalog: %v", err)
	}
	if len(services) != 1 || services[0].GetName() != "indexer" {
		t.Fatalf("service catalog = %+v", services)
	}
}

func TestApplyServiceFunctionMetadata(t *testing.T) {
	desc := &servicev1.ServiceDescriptor{
		Name: "indexer",
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:  "quark.indexer.v1.IndexerService",
			Method:   "GetContext",
			Request:  "old.Request",
			Response: "old.Response",
		}},
	}
	manifest := &plugin.Manifest{
		Name: "indexer",
		Type: plugin.TypeService,
		Service: &plugin.ServiceConfig{
			Functions: []plugin.ServiceFunctionConfig{{
				Name:        "indexer_GetContext",
				Service:     "quark.indexer.v1.IndexerService",
				Method:      "GetContext",
				Request:     "quark.indexer.v1.QueryRequest",
				Response:    "quark.indexer.v1.ContextResponse",
				Description: "Retrieve context using a query embedding.",
				RiskLevel:   "read",
				Idempotent:  true,
			}},
		},
	}

	if err := applyServiceFunctionMetadata(desc, manifest); err != nil {
		t.Fatalf("apply metadata: %v", err)
	}
	rpc := desc.GetRpcs()[0]
	if rpc.GetRequest() != "quark.indexer.v1.QueryRequest" || rpc.GetResponse() != "quark.indexer.v1.ContextResponse" {
		t.Fatalf("rpc types were not updated: %+v", rpc)
	}
	if rpc.GetDescription() != "Retrieve context using a query embedding." {
		t.Fatalf("description = %q", rpc.GetDescription())
	}
	if rpc.GetOwner() != "indexer" || rpc.GetFunctionName() != "indexer_GetContext" || rpc.GetRiskLevel() != "read" {
		t.Fatalf("function contract metadata missing: %+v", rpc)
	}
	if !rpc.GetIdempotent() || rpc.GetTimeoutMillis() != 30000 {
		t.Fatalf("runtime safety metadata missing: %+v", rpc)
	}
}

func TestApplyServiceFunctionMetadataRequiresEveryRPC(t *testing.T) {
	desc := &servicev1.ServiceDescriptor{
		Name: "indexer",
		Rpcs: []*servicev1.RpcDescriptor{{
			Service: "quark.indexer.v1.IndexerService",
			Method:  "GetContext",
		}},
	}
	manifest := &plugin.Manifest{
		Name: "indexer",
		Type: plugin.TypeService,
		Service: &plugin.ServiceConfig{
			Functions: []plugin.ServiceFunctionConfig{{
				Name:        "indexer_IndexDocument",
				Service:     "quark.indexer.v1.IndexerService",
				Method:      "IndexDocument",
				Request:     "quark.indexer.v1.IndexRequest",
				Response:    "quark.indexer.v1.IndexStatus",
				Description: "Persist one canonical index record.",
			}},
		},
	}

	if err := applyServiceFunctionMetadata(desc, manifest); err == nil {
		t.Fatal("apply metadata unexpectedly succeeded")
	}
}

func TestServicePluginAddressPrefersQuarkfileServiceBinding(t *testing.T) {
	t.Setenv("QUARK_CONFIGURED_INDEXER_ADDR", "127.0.0.1:9001")
	t.Setenv("QUARK_INDEXER_ADDR", "127.0.0.1:9002")
	manifest := serviceManifest("indexer", "quark.indexer.v1.IndexerService")
	manifest.Service.AddressEnv = "QUARK_INDEXER_ADDR"
	manifest.Service.DefaultAddress = "127.0.0.1:9003"

	got := servicePluginAddress(manifest, spacemodel.ServiceRef{AddressEnv: "QUARK_CONFIGURED_INDEXER_ADDR"})
	if got != "127.0.0.1:9001" {
		t.Fatalf("address from configured env = %q", got)
	}

	got = servicePluginAddress(manifest, spacemodel.ServiceRef{Address: "127.0.0.1:9004"})
	if got != "127.0.0.1:9004" {
		t.Fatalf("address from configured literal = %q", got)
	}

	got = servicePluginAddress(manifest, spacemodel.ServiceRef{})
	if got != "127.0.0.1:9002" {
		t.Fatalf("address from manifest env = %q", got)
	}

	t.Setenv("QUARK_INDEXER_ADDR", "")
	got = servicePluginAddress(manifest, spacemodel.ServiceRef{})
	if got != "127.0.0.1:9003" {
		t.Fatalf("address from default = %q", got)
	}
}

func TestCheckServicePluginReadinessHealthy(t *testing.T) {
	address, stop := startHealthServer(t, "quark.indexer.v1.IndexerService", healthpb.HealthCheckResponse_SERVING)
	defer stop()

	err := checkServicePluginReadiness(context.Background(), address, serviceManifest("indexer", "quark.indexer.v1.IndexerService"))
	if err != nil {
		t.Fatalf("readiness should pass: %v", err)
	}
}

func TestCheckServicePluginReadinessUnhealthy(t *testing.T) {
	address, stop := startHealthServer(t, "quark.indexer.v1.IndexerService", healthpb.HealthCheckResponse_NOT_SERVING)
	defer stop()

	err := checkServicePluginReadiness(context.Background(), address, serviceManifest("indexer", "quark.indexer.v1.IndexerService"))
	if err == nil || !strings.Contains(err.Error(), "NOT_SERVING") {
		t.Fatalf("expected unhealthy service error, got: %v", err)
	}
}

func TestValidateServicePluginDescriptorsRejectsMissingRPC(t *testing.T) {
	desc := &servicev1.ServiceDescriptor{
		Name:    "indexer",
		Version: "1.0.0",
		Address: "127.0.0.1:7301",
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:       "quark.indexer.v1.IndexerService",
			Method:        "GetContext",
			Request:       "quark.indexer.v1.QueryRequest",
			Response:      "quark.indexer.v1.ContextResponse",
			Description:   "Retrieve context.",
			Owner:         "indexer",
			FunctionName:  "indexer_GetContext",
			RiskLevel:     "read",
			TimeoutMillis: 30000,
		}},
	}
	manifest := serviceManifest("indexer", "quark.indexer.v1.IndexerService")
	manifest.Service.Functions = append(manifest.Service.Functions, plugin.ServiceFunctionConfig{
		Name:        "indexer_IndexDocument",
		Service:     "quark.indexer.v1.IndexerService",
		Method:      "IndexDocument",
		Request:     "quark.indexer.v1.IndexRequest",
		Response:    "quark.indexer.v1.IndexStatus",
		Description: "Persist index records.",
	})

	err := validateServicePluginDescriptors([]*servicev1.ServiceDescriptor{desc}, manifest)
	if err == nil || !strings.Contains(err.Error(), "missing RPC descriptor") {
		t.Fatalf("expected descriptor mismatch, got: %v", err)
	}
}

func TestValidateServicePluginDescriptorsRejectsVersionMismatch(t *testing.T) {
	desc := &servicev1.ServiceDescriptor{
		Name:    "indexer",
		Version: "2.0.0",
		Address: "127.0.0.1:7301",
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:     "quark.indexer.v1.IndexerService",
			Method:      "GetContext",
			Request:     "quark.indexer.v1.QueryRequest",
			Response:    "quark.indexer.v1.ContextResponse",
			Description: "Retrieve context.",
		}},
	}

	err := validateServicePluginDescriptors([]*servicev1.ServiceDescriptor{desc}, serviceManifest("indexer", "quark.indexer.v1.IndexerService"))
	if err == nil || !strings.Contains(err.Error(), "unsupported version") {
		t.Fatalf("expected version error, got: %v", err)
	}
}

func TestResolveServicePluginCatalogIgnoresUnboundInstalledServicePlugins(t *testing.T) {
	srv := serviceTestServer(t)
	writeInstalledServicePlugin(t, srv, "test-space")
	writeInstalledServicePluginNamed(t, srv, "test-space", servicePluginFixture{
		Name:         "build-release",
		AddressEnv:   "QUARK_BUILD_RELEASE_ADDR",
		ProtoService: "quark.buildrelease.v1.BuildReleaseService",
		FunctionName: "build_release_DryRun",
	})
	qf := []byte(`quark: "1.0"
meta:
  name: test-space
  version: "0.1.0"
plugins:
  - ref: quark/service-indexer
  - ref: quark/service-build-release
services:
  - name: indexer
    ref: quark/service-indexer
    mode: local
    address_env: QUARK_INDEXER_ADDR
`)
	if _, err := srv.store.UpdateQuarkfile("test-space", qf); err != nil {
		t.Fatalf("update quarkfile: %v", err)
	}
	address, stop := startRegistryService(t)
	defer stop()
	t.Setenv("QUARK_INDEXER_ADDR", address)

	descriptors, err := srv.resolveServicePluginCatalog(t.Context(), "test-space")
	if err != nil {
		t.Fatalf("resolve service catalog: %v", err)
	}
	if len(descriptors) != 1 {
		t.Fatalf("descriptors = %+v", descriptors)
	}
	if descriptors[0].GetName() != "indexer" {
		t.Fatalf("descriptor name = %q", descriptors[0].GetName())
	}
}

func TestImportServiceFunctionRoutesMakesControlServiceReachableFromRuntime(t *testing.T) {
	cfg := natshub.DefaultConfig(filepath.Join(t.TempDir(), "nats"))
	cfg.Client.Port = 0
	cfg.WebSocket.Enabled = false
	cfg.Monitoring.Enabled = false
	cfg.NoLog = true
	hub, err := natshub.New(cfg)
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}
	if err := hub.Start(t.Context()); err != nil {
		t.Fatalf("start hub: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = hub.Stop(ctx)
	})
	space, err := hub.ProvisionSpace("docs")
	if err != nil {
		t.Fatalf("provision space: %v", err)
	}

	controlCred, err := hub.ControlCredential()
	if err != nil {
		t.Fatalf("control credential: %v", err)
	}
	controlConn := connectNATS(t, hub.Endpoints().ClientURL, controlCred.Username, controlCred.Password)
	defer controlConn.Close()
	sub, err := controlConn.Subscribe("svc.gateway.v1.generate", func(msg *natsgo.Msg) {
		if err := msg.Respond([]byte("ok")); err != nil {
			t.Errorf("respond: %v", err)
		}
	})
	if err != nil {
		t.Fatalf("subscribe gateway subject: %v", err)
	}
	defer sub.Unsubscribe()
	if err := controlConn.FlushTimeout(time.Second); err != nil {
		t.Fatalf("flush service subscription: %v", err)
	}

	srv := &Server{natsHub: hub}
	err = srv.importServiceFunctionRoutes("docs", []*servicev1.ServiceDescriptor{{
		Name: "gateway",
		Rpcs: []*servicev1.RpcDescriptor{{
			Owner:  "gateway",
			Method: "Generate",
		}},
	}})
	if err != nil {
		t.Fatalf("import routes: %v", err)
	}

	runtimeConn := connectNATS(t, hub.Endpoints().ClientURL, space.Runtime.Username, space.Runtime.Password)
	defer runtimeConn.Close()
	msg, err := runtimeConn.Request("svc.gateway.v1.generate", []byte("payload"), time.Second)
	if err != nil {
		t.Fatalf("runtime request imported gateway function: %v", err)
	}
	if string(msg.Data) != "ok" {
		t.Fatalf("reply = %q", string(msg.Data))
	}
}

func connectNATS(t *testing.T, url, username, password string) *natsgo.Conn {
	t.Helper()
	conn, err := natsgo.Connect(url, natsgo.UserInfo(username, password), natsgo.Timeout(time.Second))
	if err != nil {
		t.Fatalf("connect nats: %v", err)
	}
	if err := conn.FlushTimeout(time.Second); err != nil {
		conn.Close()
		t.Fatalf("flush nats: %v", err)
	}
	return conn
}

func startHealthServer(t *testing.T, service string, status healthpb.HealthCheckResponse_ServingStatus) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := grpc.NewServer()
	healthServer := health.NewServer()
	healthServer.SetServingStatus(service, status)
	healthpb.RegisterHealthServer(server, healthServer)
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(ln)
	}()
	return ln.Addr().String(), func() {
		server.Stop()
		_ = ln.Close()
		<-errCh
	}
}

type servicePluginFixture struct {
	Name         string
	AddressEnv   string
	ProtoService string
	FunctionName string
}

func writeInstalledServicePluginNamed(t *testing.T, srv *Server, space string, fixture servicePluginFixture) {
	t.Helper()
	mgr, err := srv.store.Plugins(space)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(mgr.PluginsDir(), "services", fixture.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`name: %s
version: "1.0.0"
type: service
mode: api
description: %s service
service:
  address_env: %s
  health:
    protocol: grpc_health_v1
    service: %s
    timeout: 2s
  readiness:
    required: true
    min_version: "1.0.0"
  skill: SKILL.md
  readme: README.md
  proto_services:
    - %s
  functions:
    - name: %s
      service: %s
      method: GetContext
      request: quark.indexer.v1.QueryRequest
      response: quark.indexer.v1.ContextResponse
      description: Retrieve context.
      risk_level: read
      idempotent: true
`, fixture.Name, fixture.Name, fixture.AddressEnv, fixture.ProtoService, fixture.ProtoService, fixture.FunctionName, fixture.ProtoService)
	if err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# service-"+fixture.Name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func serviceManifest(name, protoService string) *plugin.Manifest {
	return &plugin.Manifest{
		Name:    name,
		Version: "1.0.0",
		Type:    plugin.TypeService,
		Service: &plugin.ServiceConfig{
			Health: plugin.ServiceHealthConfig{
				Protocol: "grpc_health_v1",
				Service:  protoService,
				Timeout:  "2s",
			},
			Readiness: plugin.ServiceReadinessConfig{
				Required:   true,
				MinVersion: "1.0.0",
			},
			ProtoServices: []string{protoService},
			Functions: []plugin.ServiceFunctionConfig{{
				Name:        "indexer_GetContext",
				Service:     protoService,
				Method:      "GetContext",
				Request:     "quark.indexer.v1.QueryRequest",
				Response:    "quark.indexer.v1.ContextResponse",
				Description: "Retrieve context.",
			}},
		},
	}
}
