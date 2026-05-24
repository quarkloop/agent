package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"
)

// ToolRegistry manages loaded tool plugins.
type ToolRegistry struct {
	mu     sync.RWMutex
	tools  map[string]*LoadedTool
	loader *Loader
	client *http.Client
}

// LoadedTool represents a registered tool with its execution context.
type LoadedTool struct {
	*LoadedPlugin
	ToolPlugin ToolPlugin // For lib mode (nil for api/cli mode)
	Endpoint   string     // For api mode HTTP endpoint
}

// NewToolRegistry creates a new tool registry.
func NewToolRegistry(loader *Loader) *ToolRegistry {
	return &ToolRegistry{
		tools:  make(map[string]*LoadedTool),
		loader: loader,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// Register adds a loaded plugin to the tool registry.
func (r *ToolRegistry) Register(loaded *LoadedPlugin) error {
	if loaded.Manifest.Type != TypeTool {
		return fmt.Errorf("plugin %s is not a tool plugin", loaded.Manifest.Name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	lt := &LoadedTool{LoadedPlugin: loaded}

	switch loaded.Mode {
	case ModeLib:
		tp, ok := loaded.Plugin.(ToolPlugin)
		if !ok {
			return fmt.Errorf("plugin %s does not implement ToolPlugin", loaded.Manifest.Name)
		}
		lt.ToolPlugin = tp
	case ModeAPI:
		// API mode - get endpoint from loader
		endpoint, ok := r.loader.GetAPIEndpoint(loaded.Manifest.Name)
		if !ok {
			return fmt.Errorf("no endpoint found for api plugin %s", loaded.Manifest.Name)
		}
		lt.Endpoint = endpoint + "/" + loaded.Manifest.Tool.Schema.Name
	case ModeCLI:
		// CLI mode - no endpoint, binary path resolved at execution time
	}

	r.tools[loaded.Manifest.Name] = lt
	return nil
}

// Unregister removes a tool from the registry.
func (r *ToolRegistry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tools, name)
}

// Get returns a loaded tool by name.
func (r *ToolRegistry) Get(name string) (*LoadedTool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

// Execute runs a tool by name with the given arguments.
func (r *ToolRegistry) Execute(ctx context.Context, name string, args map[string]any) (ToolResult, error) {
	r.mu.RLock()
	tool, ok := r.tools[name]
	r.mu.RUnlock()

	if !ok {
		return ToolResult{IsError: true, Error: fmt.Sprintf("tool %q not found", name)}, nil
	}

	switch tool.Mode {
	case ModeLib:
		return tool.ToolPlugin.Execute(ctx, args)
	case ModeAPI:
		return r.httpExecute(ctx, tool.Endpoint, args)
	case ModeCLI:
		return r.cliExecute(ctx, tool, args)
	default:
		return ToolResult{IsError: true, Error: fmt.Sprintf("unknown tool mode %q", tool.Mode)}, nil
	}
}

// httpExecute sends an HTTP POST request to an api-mode tool.
func (r *ToolRegistry) httpExecute(ctx context.Context, endpoint string, args map[string]any) (ToolResult, error) {
	body, err := json.Marshal(args)
	if err != nil {
		return ToolResult{IsError: true, Error: fmt.Sprintf("marshal args: %v", err)}, nil
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return ToolResult{IsError: true, Error: fmt.Sprintf("create request: %v", err)}, nil
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return ToolResult{IsError: true, Error: fmt.Sprintf("http request: %v", err)}, nil
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ToolResult{IsError: true, Error: fmt.Sprintf("read response: %v", err)}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return ToolResult{IsError: true, Error: fmt.Sprintf("http %d: %s", resp.StatusCode, string(respBody))}, nil
	}

	var result ToolResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		var output map[string]any
		if err2 := json.Unmarshal(respBody, &output); err2 == nil {
			result.Output = output
		} else {
			return ToolResult{IsError: true, Error: fmt.Sprintf("parse response: %v", err)}, nil
		}
	}

	return result, nil
}

// cliExecute invokes a CLI-mode plugin as a subprocess with JSON args via stdin.
func (r *ToolRegistry) cliExecute(ctx context.Context, tool *LoadedTool, args map[string]any) (ToolResult, error) {
	binPath := tool.Manifest.APITargetPath(tool.Dir)
	if _, err := os.Stat(binPath); err != nil {
		return ToolResult{IsError: true, Error: fmt.Sprintf("cli binary not found: %s", binPath)}, nil
	}

	body, err := json.Marshal(args)
	if err != nil {
		return ToolResult{IsError: true, Error: fmt.Sprintf("marshal args: %v", err)}, nil
	}

	cmd := exec.CommandContext(ctx, binPath, "--pipe")
	cmd.Dir = tool.Dir
	cmd.Stdin = bytes.NewReader(body)
	cmd.Env = os.Environ()

	out, err := cmd.CombinedOutput()
	if err != nil {
		return ToolResult{IsError: true, Error: fmt.Sprintf("cli execution: %v\n%s", err, string(out))}, nil
	}

	var result ToolResult
	if err := json.Unmarshal(out, &result); err != nil {
		return ToolResult{Output: map[string]any{"raw": string(out)}}, nil
	}
	return result, nil
}

// Schemas returns all tool schemas for LLM function calling.
func (r *ToolRegistry) Schemas() []ToolSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()

	schemas := make([]ToolSchema, 0, len(r.tools))
	for _, t := range r.tools {
		if t.Mode == ModeLib && t.ToolPlugin != nil {
			schemas = append(schemas, t.ToolPlugin.Schema())
		} else if t.Manifest.Tool != nil {
			schemas = append(schemas, t.Manifest.Tool.Schema)
		}
	}
	return schemas
}

// List returns all registered tool names.
func (r *ToolRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}
