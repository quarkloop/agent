//go:build e2e

package utils

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/quarkloop/supervisor/pkg/api"
	supclient "github.com/quarkloop/supervisor/pkg/client"
)

// E2EEnv is the live supervisor+agent pair driven by an e2e test.
type E2EEnv struct {
	Root                string
	SpacesDir           string
	Space               string
	SupURL              string
	Sup                 *supclient.Client
	NATS                NATSEndpoints
	Agent               api.RuntimeInfo
	AgentURL            string
	HTTPC               *http.Client
	Provider            string
	Model               string
	Embedding           EmbeddingOptions
	Services            []ServicePlugin
	Agents              []string
	ExtraServicePlugins []string

	ServiceAddresses map[string]string
}

type NATSEndpoints struct {
	ClientURL     string
	WebSocketURL  string
	MonitoringURL string
	StateDir      string
}

func (e *E2EEnv) ServiceAddress(name string) string {
	if e == nil || e.ServiceAddresses == nil {
		return ""
	}
	return e.ServiceAddresses[name]
}

// RuntimeSetup is the read-only setup state exposed to a StartOptions hook
// before the runtime process is launched.
type RuntimeSetup struct {
	Root       string
	SpacesDir  string
	Space      string
	SupURL     string
	NATS       NATSEndpoints
	WorkingDir string
}

// installSpacePlugins populates the space's plugins directory with the
// plugin manifests and their pre-built artifacts (tool binaries and provider
// .so files). The agent's api-mode loader detects the co-located binary
// and runs it directly; there is no runtime `go build`.
//
// Pre-built artifacts come from BuildAllOnce (tool binaries) and the
// repo-shipped provider .so (produced by `make build-providers`).
func installSpacePlugins(t *testing.T, env *E2EEnv, bins BuiltBinaries, includeKnowledgeServices bool) {
	t.Helper()
	pluginsDir := filepath.Join(env.SpacesDir, env.Space, "plugins")
	srcRoot := filepath.Join(QuarkRoot(t), "plugins")

	// installTool lays out a tool plugin exactly the way production installs
	// do: manifest + the binary + (optionally) the lib-mode plugin.so. The
	// agent's pluginmanager prefers lib mode when the .so is present and
	// falls back to api mode otherwise, so shipping both proves both
	// code paths work.
	installAgent := func(name string) {
		src := filepath.Join(srcRoot, "agents", name)
		dst := filepath.Join(pluginsDir, "agents", name)
		if err := os.MkdirAll(dst, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dst, err)
		}
		for _, file := range []string{"manifest.yaml", "PROFILE.yaml", "SYSTEM.md", "SKILL.md"} {
			copyFile(t, filepath.Join(src, file), filepath.Join(dst, file), 0o644)
		}
	}
	for _, agent := range env.Agents {
		installAgent(agent)
	}

	installService := func(name string) {
		src := filepath.Join(srcRoot, "services", name)
		dst := filepath.Join(pluginsDir, "services", name)
		if err := os.MkdirAll(dst, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dst, err)
		}
		copyFile(t, filepath.Join(src, "manifest.yaml"), filepath.Join(dst, "manifest.yaml"), 0o644)
		copyFile(t, filepath.Join(src, "SKILL.md"), filepath.Join(dst, "SKILL.md"), 0o644)
		copyFile(t, filepath.Join(src, "README.md"), filepath.Join(dst, "README.md"), 0o644)
	}
	installService("io")
	if includeKnowledgeServices {
		installService("core")
		installService("model")
		installService("indexer")
		installService("document")
		installService("ingestion")
		installService("citation")
		embeddingPlugin := env.Embedding.Plugin
		if embeddingPlugin == "" {
			embeddingPlugin = "embedding"
		}
		installService(embeddingPlugin)
	}
	for _, service := range env.Services {
		installService(service.withDefaults().Plugin)
	}
	for _, service := range env.ExtraServicePlugins {
		installService(service)
	}

	providerSrc := filepath.Join(srcRoot, "providers", "openrouter")
	providerLib := bins.OpenRouterLib
	if providerLib == "" {
		providerLib = filepath.Join(providerSrc, "plugin.so")
	}
	if _, err := os.Stat(providerLib); err == nil {
		dst := filepath.Join(pluginsDir, "providers", "openrouter")
		if err := os.MkdirAll(dst, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dst, err)
		}
		copyFile(t, filepath.Join(providerSrc, "manifest.yaml"), filepath.Join(dst, "manifest.yaml"), 0o644)
		copyFile(t, providerLib, filepath.Join(dst, "plugin.so"), 0o755)
	}
}

func copyFile(t *testing.T, src, dst string, mode os.FileMode) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, mode); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
}

// quarkfileFor returns the raw bytes of a minimal Quarkfile for a space.
func quarkfileFor(name, provider, model string, embedding EmbeddingOptions, services []ServicePlugin, extraServicePlugins []string, agents []string, agentServices map[string][]string, includeKnowledgeServices bool) []byte {
	env := ""
	if provider != "noop" {
		env = `  env:
    - OPENROUTER_API_KEY
`
	}
	embedding = embedding.withDefaults()
	pluginRefs := `  - ref: quark/service-io
`
	serviceBlocks := `  - name: io
    ref: quark/service-io
    mode: local
    address_env: QUARK_IO_ADDR
`
	embeddingBlock := ""
	agentBlocks := ""
	for _, agent := range agents {
		pluginRefs += fmt.Sprintf("  - ref: quark/agent-%s\n", agent)
		agentBlocks += fmt.Sprintf(`  - profile: %s
    enabled: true
`, agent)
		if allowed, ok := agentServices[agent]; ok {
			agentBlocks += "    services:\n"
			for _, service := range allowed {
				agentBlocks += fmt.Sprintf("      - %s\n", service)
			}
		}
	}
	agentsSection := ""
	if agentBlocks != "" {
		agentsSection = "agents:\n" + agentBlocks
	}
	if includeKnowledgeServices {
		pluginRefs += fmt.Sprintf(`  - ref: quark/service-core
  - ref: quark/service-model
  - ref: quark/service-indexer
  - ref: quark/service-document
  - ref: quark/service-ingestion
  - ref: quark/service-citation
  - ref: quark/service-%s
`, embedding.Plugin)
		serviceBlocks += fmt.Sprintf(`  - name: core
    ref: quark/service-core
    mode: local
    address_env: QUARK_CORE_ADDR
  - name: model
    ref: quark/service-model
    mode: local
    address_env: QUARK_MODEL_SERVICE_ADDR
  - name: indexer
    ref: quark/service-indexer
    mode: local
    address_env: QUARK_INDEXER_ADDR
  - name: document
    ref: quark/service-document
    mode: local
    address_env: QUARK_DOCUMENT_ADDR
  - name: ingestion
    ref: quark/service-ingestion
    mode: local
    address_env: QUARK_INGESTION_ADDR
  - name: citation
    ref: quark/service-citation
    mode: local
    address_env: QUARK_CITATION_ADDR
  - name: embedding
    ref: quark/service-%s
    mode: %s
    address_env: QUARK_EMBEDDING_ADDR
`, embedding.Plugin, embedding.Mode)
		embeddingBlock = fmt.Sprintf(`embedding:
  service: embedding
  provider: %s
  model: %s
  dimensions: %d
`, embedding.Provider, embedding.Model, embedding.Dimensions)
	}
	for _, service := range services {
		service = service.withDefaults()
		pluginRefs += fmt.Sprintf("  - ref: quark/service-%s\n", service.Plugin)
		serviceBlocks += fmt.Sprintf(`  - name: %s
    ref: quark/service-%s
    mode: %s
    address_env: %s
`, service.Name, service.Plugin, service.Mode, service.AddressEnv)
	}
	for _, plugin := range extraServicePlugins {
		pluginRefs += fmt.Sprintf("  - ref: quark/service-%s\n", plugin)
	}
	qf := fmt.Sprintf(`quark: "1.0"
meta:
  name: %s
  version: "0.1.0"
model:
  provider: %s
  name: %s
%s
plugins:
%s
%s
services:
%s
%s`, name, provider, model, env, pluginRefs, agentsSection, serviceBlocks, embeddingBlock)
	return []byte(qf)
}

// ServicePlugin declares an additional service plugin for an e2e space.
type ServicePlugin struct {
	Name       string
	Plugin     string
	Mode       string
	AddressEnv string
}

func (s ServicePlugin) WithDefaults() ServicePlugin {
	return s.withDefaults()
}

func (s ServicePlugin) withDefaults() ServicePlugin {
	if s.Plugin == "" {
		s.Plugin = s.Name
	}
	if s.Mode == "" {
		s.Mode = "local"
	}
	if s.AddressEnv == "" && s.Name != "" {
		s.AddressEnv = "QUARK_" + strings.ToUpper(strings.ReplaceAll(s.Name, "-", "_")) + "_ADDR"
	}
	return s
}

// EmbeddingOptions selects which embedding service plugin/profile the e2e
// space declares. The service process must be started by the test hook.
type EmbeddingOptions struct {
	Plugin     string
	Mode       string
	Provider   string
	Model      string
	Dimensions int
}

// WithDefaults returns a fully populated embedding profile for callers outside
// the utils package that need to start the matching service process.
func (o EmbeddingOptions) WithDefaults() EmbeddingOptions {
	return o.withDefaults()
}

func (o EmbeddingOptions) withDefaults() EmbeddingOptions {
	if o.Plugin == "" {
		o.Plugin = "embedding"
	}
	if o.Mode == "" {
		o.Mode = "local"
	}
	if o.Provider == "" {
		o.Provider = "local"
	}
	if o.Model == "" {
		o.Model = "local-hash-v1"
	}
	if o.Dimensions == 0 {
		o.Dimensions = 32
	}
	return o
}

// startSupervisor launches a supervisor subprocess with an isolated spaces
// root and returns the client, base URL, spaces dir, and process handle. The
// handle lets tests wait for log markers from the supervisor.
func startSupervisor(t *testing.T, bins BuiltBinaries, extraEnv map[string]string) (*supclient.Client, string, string, NATSEndpoints, *StartedProcess) {
	t.Helper()

	spacesDir := filepath.Join(t.TempDir(), "spaces")
	if err := os.MkdirAll(spacesDir, 0o755); err != nil {
		t.Fatalf("mkdir spaces: %v", err)
	}
	port := ReservePort(t)
	natsClientPort := ReservePort(t)
	natsWebSocketPort := ReservePort(t)
	natsMonitorPort := ReservePort(t)
	natsStateDir := filepath.Join(t.TempDir(), "nats")

	overrides := map[string]string{
		"QUARK_SPACES_ROOT": spacesDir,
	}
	for k, v := range extraEnv {
		overrides[k] = v
	}
	env := SupervisorProcessEnv(overrides)
	proc := StartProcess(t, "supervisor", bins.Supervisor, []string{
		"start",
		"--port", fmt.Sprint(port),
		"--nats-state-dir", natsStateDir,
		"--nats-client-port", fmt.Sprint(natsClientPort),
		"--nats-websocket-port", fmt.Sprint(natsWebSocketPort),
		"--nats-monitor-port", fmt.Sprint(natsMonitorPort),
	}, env)

	supURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	natsEndpoints := NATSEndpoints{
		ClientURL:     fmt.Sprintf("nats://127.0.0.1:%d", natsClientPort),
		WebSocketURL:  fmt.Sprintf("ws://127.0.0.1:%d", natsWebSocketPort),
		MonitoringURL: fmt.Sprintf("http://127.0.0.1:%d", natsMonitorPort),
		StateDir:      natsStateDir,
	}
	// Supervisor exposes GET /v1/spaces for liveness.
	WaitForURL(t, supURL+"/v1/spaces", 10*time.Second)

	sup := supclient.New(supclient.WithBaseURL(supURL))
	return sup, supURL, spacesDir, natsEndpoints, proc
}

// StartOptions tunes the fixture StartE2E builds. Zero-valued options yield
// the default behaviour (lib mode for tools when .so is available, binary
// otherwise).
type StartOptions struct {
	// ForceBinaryTools, when true, omits the tool plugin.so files from the
	// installed space so the agent's pluginmanager must fall back to
	// api-mode loading. Used to test binary fallback end-to-end.
	ForceBinaryTools bool
	// DisableServiceDiscovery keeps legacy provider/tool E2Es focused on plugin
	// behavior instead of adding generated service functions from the runtime
	// service catalog.
	DisableServiceDiscovery bool
	// DisableKnowledgeServices omits the default indexer and embedding service
	// declarations for non-Knowledge e2e flows.
	DisableKnowledgeServices bool
	// SupervisorEnv is appended to the supervisor process environment.
	SupervisorEnv map[string]string
	// WorkingDir is the space working directory registered with the supervisor.
	// When empty, StartE2E creates an isolated temp directory.
	WorkingDir string
	// Embedding declares the embedding service plugin/profile that the test
	// space should expose to the runtime catalog.
	Embedding EmbeddingOptions
	// Services declares additional service plugins that should be installed in
	// the e2e space and exposed to runtime via supervisor discovery.
	Services []ServicePlugin
	// ExtraServicePlugins installs service plugins without declaring a running
	// service binding in the Quarkfile.
	ExtraServicePlugins []string
	// Agents declares agent profile plugins that should be installed and
	// enabled through the Quarkfile. When empty, tests use the runtime fallback
	// profile so legacy tool/provider E2Es stay focused.
	Agents []string
	// AgentServicePermissions narrows an installed agent profile to the named
	// service functions through the Quarkfile override layer.
	AgentServicePermissions map[string][]string
	// BeforeRuntime runs after the space and plugins are ready, but before the
	// runtime child is started. Use it to start external services whose
	// addresses were already supplied through SupervisorEnv.
	BeforeRuntime func(t *testing.T, setup RuntimeSetup, bins BuiltBinaries)
}

// StartE2E boots a supervisor, registers a space, installs plugins, and
// launches an agent. Tests use the returned env to create sessions and
// interact with the agent.
func StartE2E(t *testing.T, withProvider bool, opts ...StartOptions) *E2EEnv {
	t.Helper()

	var opt StartOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	cfg, ok := CfgForTest(t, "OPENROUTER_API_KEY")
	if withProvider && !ok {
		t.Skip("no provider configured (set OPENROUTER_API_KEY)")
	}

	bins := BuildAllOnce(t)
	embedding := opt.Embedding.withDefaults()

	supervisorEnv := make(map[string]string, len(opt.SupervisorEnv)+1)
	for k, v := range opt.SupervisorEnv {
		supervisorEnv[k] = v
	}
	if opt.DisableServiceDiscovery {
		supervisorEnv["QUARK_DISABLE_SERVICE_DISCOVERY"] = "true"
	}
	sup, supURL, spacesDir, natsEndpoints, proc := startSupervisor(t, bins, supervisorEnv)

	spaceName := fmt.Sprintf("e2e-%d", time.Now().UnixNano())
	provider := "openrouter"
	model := "noop/noop"
	if withProvider {
		provider = cfg.Provider
		model = cfg.Model
	}

	workingDir := opt.WorkingDir
	if workingDir == "" {
		workingDir = t.TempDir()
	}
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		t.Fatalf("mkdir working dir: %v", err)
	}
	createCtx, createCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer createCancel()
	if _, err := sup.CreateSpace(createCtx, spaceName, quarkfileFor(spaceName, provider, model, embedding, opt.Services, opt.ExtraServicePlugins, opt.Agents, opt.AgentServicePermissions, !opt.DisableKnowledgeServices), workingDir); err != nil {
		t.Fatalf("create space: %v", err)
	}

	env := &E2EEnv{
		Root:                QuarkRoot(t),
		SpacesDir:           spacesDir,
		Space:               spaceName,
		SupURL:              supURL,
		Sup:                 sup,
		NATS:                natsEndpoints,
		HTTPC:               &http.Client{Timeout: 30 * time.Second},
		Provider:            provider,
		Model:               model,
		Embedding:           embedding,
		Services:            append([]ServicePlugin(nil), opt.Services...),
		Agents:              append([]string(nil), opt.Agents...),
		ExtraServicePlugins: append([]string(nil), opt.ExtraServicePlugins...),

		ServiceAddresses: serviceAddressesFromOptions(embedding, opt.Services, supervisorEnv),
	}

	installSpacePlugins(t, env, bins, !opt.DisableKnowledgeServices)
	if opt.BeforeRuntime != nil {
		opt.BeforeRuntime(t, RuntimeSetup{
			Root:       env.Root,
			SpacesDir:  env.SpacesDir,
			Space:      env.Space,
			SupURL:     env.SupURL,
			NATS:       env.NATS,
			WorkingDir: workingDir,
		}, bins)
	}

	agentPort := ReservePort(t)
	runtimeID := "e2e-runtime-" + spaceName
	runtimeOverrides := map[string]string{
		"QUARK_RUNTIME_ID":     runtimeID,
		"QUARK_SUPERVISOR_URL": supURL,
		"QUARK_SPACE":          spaceName,
		"QUARK_PLUGINS_DIR":    filepath.Join(spacesDir, spaceName, "plugins"),
		"QUARK_MODEL_PROVIDER": provider,
		"QUARK_MODEL_NAME":     model,
	}
	for _, key := range providerCredentialEnvKeys() {
		if value := supervisorEnv[key]; value != "" {
			runtimeOverrides[key] = value
		}
	}
	runtimeEnv := RuntimeProcessEnv(runtimeOverrides)
	StartProcess(t, "runtime", bins.Agent, []string{
		"start",
		"--port", fmt.Sprint(agentPort),
		"--channel", "web",
	}, runtimeEnv)
	env.Agent = api.RuntimeInfo{
		ID:     runtimeID,
		Space:  spaceName,
		Port:   agentPort,
		Status: api.RuntimeRunning,
	}
	env.AgentURL = fmt.Sprintf("http://127.0.0.1:%d", agentPort)

	WaitForURL(t, env.AgentURL+"/health", 30*time.Second)
	// Wait for the agent's SSE subscription to the supervisor to go live,
	// otherwise the very first session event can be published before any
	// subscriber is attached and silently dropped.
	proc.WaitForLog(t, "supervisor event stream ready", 10*time.Second)
	Logf(t, "supervisor at %s, agent at %s (space=%s)", supURL, env.AgentURL, spaceName)
	return env
}

func serviceAddressesFromOptions(embedding EmbeddingOptions, services []ServicePlugin, supervisorEnv map[string]string) map[string]string {
	addresses := make(map[string]string)
	if addr := supervisorEnv["QUARK_INDEXER_ADDR"]; addr != "" {
		addresses["indexer"] = addr
	}
	if addr := supervisorEnv["QUARK_CORE_ADDR"]; addr != "" {
		addresses["core"] = addr
	}
	if addr := supervisorEnv["QUARK_MODEL_SERVICE_ADDR"]; addr != "" {
		addresses["model"] = addr
	}
	if addr := supervisorEnv["QUARK_EMBEDDING_ADDR"]; addr != "" {
		addresses["embedding"] = addr
		addresses[embedding.withDefaults().Plugin] = addr
	}
	if addr := supervisorEnv["QUARK_DOCUMENT_ADDR"]; addr != "" {
		addresses["document"] = addr
	}
	if addr := supervisorEnv["QUARK_INGESTION_ADDR"]; addr != "" {
		addresses["ingestion"] = addr
	}
	if addr := supervisorEnv["QUARK_CITATION_ADDR"]; addr != "" {
		addresses["citation"] = addr
	}
	if addr := supervisorEnv["QUARK_IO_ADDR"]; addr != "" {
		addresses["io"] = addr
	}
	for _, service := range services {
		service = service.withDefaults()
		if addr := supervisorEnv[service.AddressEnv]; addr != "" {
			addresses[service.Name] = addr
			addresses[service.Plugin] = addr
		}
	}
	return addresses
}
