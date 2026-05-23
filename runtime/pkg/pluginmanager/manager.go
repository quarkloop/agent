// Package pluginmanager loads and unloads plugins from paths on disk.
//
// The agent does not install or uninstall plugins itself — those operations
// are the supervisor's responsibility. When the supervisor provides a resolved
// catalog, the agent loads only those explicit plugin directories.
package pluginmanager

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/quarkloop/pkg/plugin"
)

// Manager loads tool plugins from supervisor-resolved paths and dispatches calls
// to them. Provider plugins are not loaded in runtime; Gateway owns provider
// dispatch and is registered through RegisterRuntimeProvider.
//
// Tool plugins run as api-mode HTTP servers or as lib-mode .so files loaded
// in-process. Provider dispatch is externalized behind Gateway and registered
// through runtime wiring when needed.
type Manager struct {
	mu         sync.RWMutex
	pluginsDir string
	binDir     string
	loader     *plugin.Loader
	catalog    *Catalog

	// API-mode tool plugins
	processes  map[string]*exec.Cmd
	endpoints  map[string]string
	httpClient *http.Client
	nextPort   int

	// Lib-mode tool plugins (.so)
	libTools map[string]plugin.ToolPlugin

	// Runtime-registered tools. These are still tools from the agent's point
	// of view, but their implementation is supplied by runtime-owned wiring
	// instead of a disk plugin process.
	runtimeTools map[string]RuntimeToolHandler

	// Tool schemas aggregated from all loaded tools
	tools []plugin.ToolSchema

	// Runtime-registered model providers. These should point to Gateway-backed
	// adapters, not direct provider plugins.
	providers map[string]plugin.Provider
}

// NewManager creates a plugin manager rooted at the given plugins directory
// (typically <space>/.quark/plugins).
func NewManager(pluginsDir string) *Manager {
	binDir := filepath.Join(pluginsDir, ".bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		slog.Error("failed to create bin dir", "path", binDir, "error", err)
	}

	return &Manager{
		pluginsDir:   pluginsDir,
		binDir:       binDir,
		loader:       plugin.NewLoader(pluginsDir),
		processes:    make(map[string]*exec.Cmd),
		endpoints:    make(map[string]string),
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		nextPort:     8100,
		libTools:     make(map[string]plugin.ToolPlugin),
		runtimeTools: make(map[string]RuntimeToolHandler),
		providers:    make(map[string]plugin.Provider),
		tools:        make([]plugin.ToolSchema, 0),
	}
}

// Initialize loads tool plugins. When a supervisor-resolved catalog
// is present, runtime loads exactly that catalog. Standalone runtimes can still
// register tools explicitly through a provided catalog or RegisterRuntimeTool.
func (m *Manager) Initialize(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.catalog != nil {
		return m.loadCatalogLocked(ctx, *m.catalog)
	}
	slog.Info("plugin manager initialized without catalog; local filesystem discovery disabled")
	return nil
}

// LoadProviders is retained for tests and legacy callers, but runtime no longer
// loads direct provider plugins. Gateway is the only production provider path.
func (m *Manager) LoadProviders(ctx context.Context) error {
	_ = ctx
	slog.Info("provider plugin loading skipped; Gateway owns provider dispatch")
	return nil
}

// LoadPluginFromDir loads a single plugin from the given directory. Intended
// for use when the supervisor notifies the agent that a new plugin has been
// installed.
func (m *Manager) LoadPluginFromDir(ctx context.Context, dir string) error {
	manifest, err := plugin.ParseManifest(filepath.Join(dir, "manifest.yaml"))
	if err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	switch manifest.Type {
	case plugin.TypeTool:
		return m.loadToolLocked(ctx, manifest, dir)
	case plugin.TypeProvider:
		slog.Info("provider plugin load skipped; Gateway owns provider dispatch", "plugin", manifest.Name)
		return nil
	default:
		return nil
	}
}

// UnloadPlugin stops and removes a tool plugin by name.
// Returns true if the plugin was loaded and has been unloaded.
func (m *Manager) UnloadPlugin(name string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	unloaded := false

	if _, ok := m.libTools[name]; ok {
		delete(m.libTools, name)
		unloaded = true
	}

	if cmd, ok := m.processes[name]; ok {
		m.stopProcess(cmd)
		delete(m.processes, name)
		delete(m.endpoints, name)
		unloaded = true
	}

	if unloaded {
		m.removeToolSchemaLocked(name)
	}
	return unloaded
}

// GetTools returns the aggregated tool schemas for all loaded tools.
func (m *Manager) GetTools() []plugin.ToolSchema {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]plugin.ToolSchema, len(m.tools))
	copy(out, m.tools)
	return out
}

// GetProviders returns a copy of the loaded provider map keyed by provider ID.
func (m *Manager) GetProviders() map[string]plugin.Provider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]plugin.Provider, len(m.providers))
	for k, v := range m.providers {
		out[k] = v
	}
	return out
}

// GetProvider returns a single provider by ID.
func (m *Manager) GetProvider(id string) (plugin.Provider, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.providers[id]
	return p, ok
}

// RegisterRuntimeProvider registers a provider supplied by runtime wiring
// instead of a disk plugin. It replaces an existing provider with the same ID.
func (m *Manager) RegisterRuntimeProvider(id string, provider plugin.Provider) {
	if id == "" || providerIsNil(provider) {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.providers[id] = provider
}

func providerIsNil(provider plugin.Provider) bool {
	if provider == nil {
		return true
	}
	value := reflect.ValueOf(provider)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// ListLoaded returns the names of all currently-loaded tool plugins.
func (m *Manager) ListLoaded() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.libTools)+len(m.processes))
	for n := range m.libTools {
		names = append(names, n)
	}
	for n := range m.processes {
		names = append(names, n)
	}
	return names
}

// IsLoaded reports whether a tool plugin with the given name is loaded.
func (m *Manager) IsLoaded(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.runtimeTools[name]; ok {
		return true
	}
	if _, ok := m.libTools[name]; ok {
		return true
	}
	_, ok := m.processes[name]
	return ok
}

// ExecuteTool invokes a loaded tool. Lib-mode tools are called in-process;
// api-mode tools are invoked over HTTP.
func (m *Manager) ExecuteTool(ctx context.Context, name, arguments string) (string, error) {
	m.mu.RLock()
	runtimeTool, isRuntime := m.runtimeTools[name]
	libTool, isLib := m.libTools[name]
	endpoint, isBinary := m.endpoints[name]
	m.mu.RUnlock()

	if isRuntime {
		return runtimeTool(ctx, arguments)
	}
	if isLib {
		return m.executeLib(ctx, libTool, arguments)
	}
	if isBinary {
		return m.executeBinary(ctx, name, endpoint, arguments)
	}
	return "", fmt.Errorf("tool not found or not running")
}

// Shutdown stops all api-mode plugin processes.
func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, cmd := range m.processes {
		m.stopProcess(cmd)
	}
	m.processes = make(map[string]*exec.Cmd)
	m.endpoints = make(map[string]string)
}

// --- internal helpers (must be called with m.mu held where noted) ---

func (m *Manager) loadToolLocked(ctx context.Context, manifest *plugin.Manifest, pluginDir string) error {
	if manifest.Mode == plugin.ModeLib {
		if err := m.loadToolLibLocked(ctx, manifest, pluginDir); err == nil {
			return nil
		} else {
			fmt.Printf("pluginmanager: lib mode failed for %s (%v), trying api mode\n", manifest.Name, err)
		}
	}
	return m.loadToolBinaryLocked(ctx, manifest, pluginDir)
}

func (m *Manager) loadToolLibLocked(ctx context.Context, manifest *plugin.Manifest, pluginDir string) error {
	loaded, err := m.loader.Load(ctx, manifest, pluginDir)
	if err != nil {
		return fmt.Errorf("load .so: %w", err)
	}
	if loaded.Plugin == nil {
		return fmt.Errorf("no plugin instance in .so")
	}
	tp, ok := loaded.Plugin.(plugin.ToolPlugin)
	if !ok {
		return fmt.Errorf("%s does not implement ToolPlugin", manifest.Name)
	}

	schema := tp.Schema()
	toolName := schema.Name
	if toolName == "" {
		toolName = manifest.Name
	}

	m.libTools[toolName] = tp
	m.tools = append(m.tools, schema)
	fmt.Printf("pluginmanager: loaded tool %s (lib)\n", toolName)
	return nil
}

func (m *Manager) loadToolBinaryLocked(ctx context.Context, manifest *plugin.Manifest, pluginDir string) error {
	binName := manifest.Name
	if manifest.Build != nil && manifest.Build.APITarget != "" {
		binName = manifest.Build.APITarget
	}
	outPath := filepath.Join(m.binDir, binName)

	// Prefer a pre-built binary shipped alongside the manifest. Installers
	// (including `make build-tools` + plugin install) drop the compiled
	// binary at <pluginDir>/<api_target>; when present, we skip the
	// in-process `go build` step entirely.
	prebuilt := filepath.Join(pluginDir, binName)
	if info, err := os.Stat(prebuilt); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
		outPath = prebuilt
	} else {
		cmdDir := filepath.Join(pluginDir, "cmd", manifest.Name)
		if _, err := os.Stat(cmdDir); os.IsNotExist(err) {
			entries, err := os.ReadDir(filepath.Join(pluginDir, "cmd"))
			if err != nil || len(entries) == 0 {
				return fmt.Errorf("no cmd/ directory found")
			}
			cmdDir = filepath.Join(pluginDir, "cmd", entries[0].Name())
		}

		buildCmd := exec.CommandContext(ctx, "go", "build", "-o", outPath, ".")
		buildCmd.Dir = cmdDir
		buildCmd.Stdout = os.Stdout
		buildCmd.Stderr = os.Stderr
		if err := buildCmd.Run(); err != nil {
			return fmt.Errorf("build: %w", err)
		}
	}

	if manifest.Tool != nil {
		m.tools = append(m.tools, manifest.Tool.Schema)
	}

	port := m.nextPort
	m.nextPort++
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	daemonCmd := exec.Command(outPath, "serve", "--addr", addr)
	daemonCmd.Stdout = os.Stdout
	daemonCmd.Stderr = os.Stderr
	if err := daemonCmd.Start(); err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}

	toolName := manifest.Name
	if manifest.Tool != nil && manifest.Tool.Schema.Name != "" {
		toolName = manifest.Tool.Schema.Name
	}

	m.processes[toolName] = daemonCmd
	m.endpoints[toolName] = fmt.Sprintf("http://%s/%s", addr, toolName)

	time.Sleep(50 * time.Millisecond)

	fmt.Printf("pluginmanager: loaded tool %s (binary, %s)\n", toolName, addr)
	return nil
}

func (m *Manager) executeLib(ctx context.Context, tool plugin.ToolPlugin, arguments string) (string, error) {
	var args map[string]any
	if err := json.Unmarshal([]byte(arguments), &args); err != nil {
		return "", fmt.Errorf("parse arguments: %w", err)
	}
	result, err := tool.Execute(ctx, args)
	if err != nil {
		return "", fmt.Errorf("execute tool: %w", err)
	}
	if result.IsError {
		resp := map[string]any{"error": result.Error, "is_error": true}
		data, _ := json.Marshal(resp)
		return string(data), nil
	}
	data, err := json.Marshal(result.Output)
	if err != nil {
		return "", fmt.Errorf("serialize result: %w", err)
	}
	return string(data), nil
}

func (m *Manager) executeBinary(ctx context.Context, name, endpoint, arguments string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBufferString(arguments))
	if err != nil {
		return "", fmt.Errorf("create plugin request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("plugin http error: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read plugin response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("plugin %q returned HTTP %d: %s", name, resp.StatusCode, string(data))
	}
	return normalizeToolResponse(data), nil
}

func normalizeToolResponse(data []byte) string {
	var out struct {
		Data  json.RawMessage `json:"data"`
		Error string          `json:"error"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return string(data)
	}
	if out.Error != "" {
		resp := map[string]any{"error": out.Error, "is_error": true}
		normalized, _ := json.Marshal(resp)
		return string(normalized)
	}
	if len(out.Data) > 0 && string(out.Data) != "null" {
		return string(out.Data)
	}
	return string(data)
}

// removeToolSchemaLocked removes the tool schema for the given name.
// Caller must hold m.mu.
func (m *Manager) removeToolSchemaLocked(name string) {
	for i, t := range m.tools {
		if t.Name == name {
			m.tools = append(m.tools[:i], m.tools[i+1:]...)
			return
		}
	}
}

func (m *Manager) stopProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		slog.Error("failed to send SIGINT to process", "pid", cmd.Process.Pid, "error", err)
	}
	done := make(chan struct{})
	go func() {
		if err := cmd.Wait(); err != nil {
			slog.Error("process exited with error", "pid", cmd.Process.Pid, "error", err)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		if err := cmd.Process.Kill(); err != nil {
			slog.Error("failed to kill process", "pid", cmd.Process.Pid, "error", err)
		}
		<-done
	}
}
