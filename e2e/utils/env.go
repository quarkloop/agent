//go:build e2e

package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	"github.com/quarkloop/supervisor/pkg/api"
)

type E2EEnv struct {
	Root                string
	SpacesDir           string
	Space               string
	SupURL              string
	NATS                NATSEndpoints
	Agent               api.RuntimeInfo
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
// plugin manifests and their pre-built artifacts. The agent's api-mode loader
// detects a co-located binary for tool plugins and runs it directly; there is
// no runtime `go build`.
//
// Pre-built artifacts come from BuildAllOnce.
type StartOptions struct {
	// ForceBinaryTools, when true, omits the tool plugin.so files from the
	// installed space so the agent's pluginmanager must fall back to
	// api-mode loading. Used to test binary fallback end-to-end.
	ForceBinaryTools bool
	// DisableServiceDiscovery keeps legacy tool E2Es focused on plugin behavior
	// instead of adding generated service functions from the runtime service
	// catalog.
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
	// profile so legacy tool E2Es stay focused.
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
	supURL, spacesDir, natsEndpoints := startSupervisor(t, bins, supervisorEnv)

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
	agents := withDefaultMainAgent(opt.Agents)
	createSpace(t, natsEndpoints, clientcontract.CreateSpaceRequest{
		Name:       spaceName,
		Quarkfile:  quarkfileFor(spaceName, provider, model, embedding, opt.Services, opt.ExtraServicePlugins, agents, opt.AgentServicePermissions, !opt.DisableKnowledgeServices),
		WorkingDir: workingDir,
	})
	runtimeCredential := issueRuntimeCredential(t, natsEndpoints, spaceName)

	env := &E2EEnv{
		Root:                QuarkRoot(t),
		SpacesDir:           spacesDir,
		Space:               spaceName,
		SupURL:              supURL,
		NATS:                natsEndpoints,
		Provider:            provider,
		Model:               model,
		Embedding:           embedding,
		Services:            append([]ServicePlugin(nil), opt.Services...),
		Agents:              append([]string(nil), agents...),
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
	DumpNATSCLIDiagnostics(t, env.NATS, "after-services")

	runtimeID := "e2e-runtime-" + spaceName
	runtimeOverrides := map[string]string{
		"QUARK_RUNTIME_ID":                runtimeID,
		"QUARK_SUPERVISOR_URL":            supURL,
		"QUARK_SPACE":                     spaceName,
		"QUARK_PLUGINS_DIR":               filepath.Join(spacesDir, spaceName, "plugins"),
		"QUARK_MODEL_PROVIDER":            provider,
		"QUARK_MODEL_NAME":                model,
		"QUARK_GATEWAY_REQUEST_TIMEOUT":   e2eModelGatewayTimeout(),
		"QUARK_GATEWAY_MAX_OUTPUT_TOKENS": e2eModelMaxOutputTokens(),
		"QUARK_NATS_URL":                  runtimeCredential.URL,
		"QUARK_NATS_USER":                 runtimeCredential.Username,
		"QUARK_NATS_PASSWORD":             runtimeCredential.Password,
	}
	for _, key := range providerCredentialEnvKeys() {
		if value := supervisorEnv[key]; value != "" {
			runtimeOverrides[key] = value
		}
	}
	runtimeEnv := RuntimeProcessEnv(runtimeOverrides)
	runtimeProc := StartProcess(t, "runtime", bins.Agent, []string{
		"start",
		"--channel", "nats",
	}, runtimeEnv)
	env.Agent = api.RuntimeInfo{
		ID:     runtimeID,
		Space:  spaceName,
		Status: api.RuntimeRunning,
	}

	runtimeProc.WaitForLog(t, "nats channel listening", 30*time.Second)
	// Wait for the agent's event subscription to the supervisor to go live,
	// otherwise the very first session event can be published before any
	// subscriber is attached and silently dropped.
	runtimeProc.WaitForLog(t, "supervisor event stream ready", 10*time.Second)
	DumpNATSCLIDiagnostics(t, env.NATS, "after-runtime")
	Logf(t, "supervisor at %s, runtime=%s nats=%s (space=%s)", supURL, runtimeID, natsEndpoints.ClientURL, spaceName)
	return env
}

func withDefaultMainAgent(agents []string) []string {
	const mainAgent = "quark-main"
	out := make([]string, 0, len(agents)+1)
	seen := make(map[string]struct{}, len(agents)+1)
	add := func(agent string) {
		agent = strings.TrimSpace(agent)
		if agent == "" {
			return
		}
		if _, ok := seen[agent]; ok {
			return
		}
		seen[agent] = struct{}{}
		out = append(out, agent)
	}
	add(mainAgent)
	for _, agent := range agents {
		add(agent)
	}
	return out
}

func e2eModelGatewayTimeout() string {
	if value := strings.TrimSpace(os.Getenv("QUARK_E2E_MODEL_GATEWAY_TIMEOUT")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("QUARK_GATEWAY_REQUEST_TIMEOUT")); value != "" {
		return value
	}
	return "2m"
}

func e2eModelMaxOutputTokens() string {
	if value := strings.TrimSpace(os.Getenv("QUARK_E2E_MODEL_MAX_OUTPUT_TOKENS")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("QUARK_GATEWAY_MAX_OUTPUT_TOKENS")); value != "" {
		return value
	}
	return "4096"
}

func serviceAddressesFromOptions(embedding EmbeddingOptions, services []ServicePlugin, supervisorEnv map[string]string) map[string]string {
	addresses := make(map[string]string)
	if addr := supervisorEnv["QUARK_INDEXER_ADDR"]; addr != "" {
		addresses["indexer"] = addr
	}
	if addr := supervisorEnv["QUARK_CORE_ADDR"]; addr != "" {
		addresses["core"] = addr
	}
	if addr := supervisorEnv["QUARK_GATEWAY_SERVICE_ADDR"]; addr != "" {
		addresses["gateway"] = addr
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
