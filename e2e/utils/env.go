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
	Root      string
	SpacesDir string
	Space     string
	SupURL    string
	Sup       *supclient.Client
	Agent     api.RuntimeInfo
	AgentURL  string
	HTTPC     *http.Client
	Embedding EmbeddingOptions
	Services  []ServicePlugin
}

// RuntimeSetup is the read-only setup state exposed to a StartOptions hook
// before the runtime process is launched.
type RuntimeSetup struct {
	Root       string
	SpacesDir  string
	Space      string
	SupURL     string
	WorkingDir string
}

// installSpacePlugins populates the space's plugins directory with the
// plugin manifests and their pre-built artifacts (tool binaries and provider
// .so files). The agent's api-mode loader detects the co-located binary
// and runs it directly; there is no runtime `go build`.
//
// Pre-built artifacts come from BuildAllOnce (tool binaries) and the
// repo-shipped provider .so (produced by `make build-providers`).
func installSpacePlugins(t *testing.T, env *E2EEnv, bins BuiltBinaries) {
	t.Helper()
	pluginsDir := filepath.Join(env.SpacesDir, env.Space, "plugins")
	srcRoot := filepath.Join(QuarkRoot(t), "plugins")

	// installTool lays out a tool plugin exactly the way production installs
	// do: manifest + the binary + (optionally) the lib-mode plugin.so. The
	// agent's pluginmanager prefers lib mode when the .so is present and
	// falls back to api mode otherwise, so shipping both proves both
	// code paths work.
	installTool := func(name, binPath, libPath string) {
		dst := filepath.Join(pluginsDir, "tools", name)
		if err := os.MkdirAll(dst, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dst, err)
		}
		copyFile(t, filepath.Join(srcRoot, "tools", name, "manifest.yaml"), filepath.Join(dst, "manifest.yaml"), 0o644)
		copyFile(t, binPath, filepath.Join(dst, name), 0o755)
		if libPath != "" {
			copyFile(t, libPath, filepath.Join(dst, "plugin.so"), 0o644)
		}
	}
	installTool("bash", bins.Bash, bins.BashLib)
	installTool("fs", bins.FS, bins.FSLib)

	installService := func(name string) {
		src := filepath.Join(srcRoot, "services", name)
		dst := filepath.Join(pluginsDir, "services", name)
		if err := os.MkdirAll(dst, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dst, err)
		}
		copyFile(t, filepath.Join(src, "manifest.yaml"), filepath.Join(dst, "manifest.yaml"), 0o644)
		copyFile(t, filepath.Join(src, "SKILL.md"), filepath.Join(dst, "SKILL.md"), 0o644)
	}
	installService("indexer")
	embeddingPlugin := env.Embedding.Plugin
	if embeddingPlugin == "" {
		embeddingPlugin = "embedding"
	}
	installService(embeddingPlugin)
	for _, service := range env.Services {
		installService(service.withDefaults().Plugin)
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
func quarkfileFor(name, provider, model string, embedding EmbeddingOptions, services []ServicePlugin, includeKnowledgeServices bool) []byte {
	env := ""
	if provider != "noop" {
		env = `  env:
    - OPENROUTER_API_KEY
`
	}
	embedding = embedding.withDefaults()
	pluginRefs := fmt.Sprintf(`  - ref: quark/tool-bash
  - ref: quark/tool-fs
`)
	serviceBlocks := ""
	embeddingBlock := ""
	if includeKnowledgeServices {
		pluginRefs += fmt.Sprintf(`  - ref: quark/service-indexer
  - ref: quark/service-%s
`, embedding.Plugin)
		serviceBlocks += fmt.Sprintf(`  - name: indexer
    ref: quark/service-indexer
    mode: local
    address_env: QUARK_INDEXER_ADDR
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
services:
%s
%s`, name, provider, model, env, pluginRefs, serviceBlocks, embeddingBlock)
	return []byte(qf)
}

// ServicePlugin declares an additional service plugin for an e2e space.
type ServicePlugin struct {
	Name       string
	Plugin     string
	Mode       string
	AddressEnv string
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
// handle lets tests wait for log markers from the supervisor or its agent
// child (whose stdio is inherited into the same buffer).
func startSupervisor(t *testing.T, bins BuiltBinaries, extraEnv map[string]string) (*supclient.Client, string, string, *StartedProcess) {
	t.Helper()

	spacesDir := filepath.Join(t.TempDir(), "spaces")
	if err := os.MkdirAll(spacesDir, 0o755); err != nil {
		t.Fatalf("mkdir spaces: %v", err)
	}
	port := ReservePort(t)

	overrides := map[string]string{
		"QUARK_SPACES_ROOT": spacesDir,
	}
	for k, v := range extraEnv {
		overrides[k] = v
	}
	env := ProcessEnv(overrides)
	proc := StartProcess(t, "supervisor", bins.Supervisor, []string{
		"start",
		"--port", fmt.Sprint(port),
		"--runtime", bins.Agent,
	}, env)

	supURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	// Supervisor exposes GET /v1/spaces for liveness.
	WaitForURL(t, supURL+"/v1/spaces", 10*time.Second)

	sup := supclient.New(supclient.WithBaseURL(supURL))
	return sup, supURL, spacesDir, proc
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
	// SupervisorEnv is appended to the supervisor process environment. Runtime
	// children inherit these values, so service discovery addresses should be
	// supplied here before StartRuntime is called.
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
	if opt.ForceBinaryTools {
		bins.BashLib, bins.FSLib = "", ""
	}
	embedding := opt.Embedding.withDefaults()

	supervisorEnv := make(map[string]string, len(opt.SupervisorEnv)+1)
	for k, v := range opt.SupervisorEnv {
		supervisorEnv[k] = v
	}
	if opt.DisableServiceDiscovery {
		supervisorEnv["QUARK_DISABLE_SERVICE_DISCOVERY"] = "true"
	}
	sup, supURL, spacesDir, proc := startSupervisor(t, bins, supervisorEnv)

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
	if _, err := sup.CreateSpace(createCtx, spaceName, quarkfileFor(spaceName, provider, model, embedding, opt.Services, !opt.DisableKnowledgeServices), workingDir); err != nil {
		t.Fatalf("create space: %v", err)
	}

	env := &E2EEnv{
		Root:      QuarkRoot(t),
		SpacesDir: spacesDir,
		Space:     spaceName,
		SupURL:    supURL,
		Sup:       sup,
		HTTPC:     &http.Client{Timeout: 30 * time.Second},
		Embedding: embedding,
		Services:  append([]ServicePlugin(nil), opt.Services...),
	}

	installSpacePlugins(t, env, bins)
	if opt.BeforeRuntime != nil {
		opt.BeforeRuntime(t, RuntimeSetup{
			Root:       env.Root,
			SpacesDir:  env.SpacesDir,
			Space:      env.Space,
			SupURL:     env.SupURL,
			WorkingDir: workingDir,
		}, bins)
	}

	agentPort := ReservePort(t)
	runtimeCtx, runtimeCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer runtimeCancel()
	info, err := sup.StartRuntime(runtimeCtx, spaceName, agentPort)
	if err != nil {
		t.Fatalf("start runtime: %v", err)
	}
	env.Agent = info
	env.AgentURL = fmt.Sprintf("http://127.0.0.1:%d", info.Port)

	WaitForURL(t, env.AgentURL+"/health", 30*time.Second)
	// Wait for the agent's SSE subscription to the supervisor to go live,
	// otherwise the very first session event can be published before any
	// subscriber is attached and silently dropped.
	proc.WaitForLog(t, "supervisor event stream ready", 10*time.Second)
	Logf(t, "supervisor at %s, agent at %s (space=%s)", supURL, env.AgentURL, spaceName)
	return env
}
