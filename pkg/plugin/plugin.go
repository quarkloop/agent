// Package plugin defines the unified plugin interfaces and types for Quark.
//
// Quark supports three plugin modes:
//   - lib mode: plugins compiled as .so files, loaded via Go's plugin system
//   - api mode: plugins run as separate HTTP server processes
//   - cli mode: plugins invoked as CLI subprocess per call
//
// All plugins implement the base Plugin interface. Type-specific interfaces
// such as ToolPlugin extend this for specialized functionality.
package plugin

import "context"

// PluginType classifies what a plugin provides.
type PluginType string

const (
	TypeTool    PluginType = "tool"
	TypeAgent   PluginType = "agent"
	TypeSkill   PluginType = "skill"
	TypeService PluginType = "service"
)

// PluginMode indicates how the plugin is loaded.
type PluginMode string

const (
	ModeLib PluginMode = "lib" // .so file loaded via plugin.Open()
	ModeAPI PluginMode = "api" // Separate HTTP server process
	ModeCLI PluginMode = "cli" // Invoked as CLI subprocess per call
)

// Plugin is the base interface all plugins must implement.
// For lib-mode plugins, export a variable named QuarkPlugin of this type.
type Plugin interface {
	// Name returns the plugin's unique identifier.
	Name() string

	// Version returns the semantic version string.
	Version() string

	// Type returns the plugin type (tool, agent, skill, service).
	Type() PluginType

	// Initialize is called after loading to set up the plugin.
	// Config contains plugin-specific configuration from the manifest.
	Initialize(ctx context.Context, config map[string]any) error

	// Shutdown is called before unloading to clean up resources.
	Shutdown(ctx context.Context) error
}

// LoadedPlugin represents a successfully loaded plugin instance.
type LoadedPlugin struct {
	Manifest *Manifest  // Parsed manifest
	Plugin   Plugin     // Plugin instance (nil for api/cli mode)
	Mode     PluginMode // How it was loaded
	Dir      string     // Plugin directory path
}
