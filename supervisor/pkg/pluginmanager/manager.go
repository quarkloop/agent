// Package pluginmanager handles plugin installation, uninstallation, and
// discovery within a space's plugins directory. The absolute plugins
// directory is supplied by the caller; the manager has no knowledge of
// the surrounding space layout.
package pluginmanager

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	plugin "github.com/quarkloop/pkg/plugin"
)

// Installer handles plugin management for a single plugins directory.
type Installer struct {
	mu         sync.RWMutex
	pluginsDir string
	hubClient  *HubClient
}

// NewInstaller creates a plugin installer rooted at pluginsDir. The directory
// must be the absolute path to the space's plugins directory.
func NewInstaller(pluginsDir string) *Installer {
	return &Installer{
		pluginsDir: pluginsDir,
		hubClient:  NewHubClient(""),
	}
}

// PluginsDir returns the absolute plugins directory managed by this Manager.
func (m *Installer) PluginsDir() string {
	return m.pluginsDir
}

// InstalledPlugin represents an installed plugin with its path.
type InstalledPlugin struct {
	Manifest *plugin.Manifest `json:"manifest"`
	Path     string           `json:"path"` // Full path to plugin directory
	Active   bool             `json:"active"`
}

// List returns all installed plugins in the space.
func (m *Installer) List() ([]InstalledPlugin, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var plugins []InstalledPlugin
	pluginsDir := m.PluginsDir()
	for _, dir := range []string{"tools", "agents", "skills", "services"} {
		plugins = append(plugins, installedInDir(filepath.Join(pluginsDir, dir))...)
	}

	return plugins, nil
}

func installedInDir(root string) []InstalledPlugin {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	plugins := make([]InstalledPlugin, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pluginDir := filepath.Join(root, e.Name())
		manifest, err := plugin.ParseManifest(filepath.Join(pluginDir, "manifest.yaml"))
		if err != nil {
			continue
		}
		plugins = append(plugins, InstalledPlugin{
			Manifest: manifest,
			Path:     pluginDir,
		})
	}
	return plugins
}

// Install installs a plugin from a reference (local path, git URL, or hub name).
func (m *Installer) Install(ctx context.Context, ref string) (*InstalledPlugin, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	pluginsDir := m.PluginsDir()

	// Ensure plugins directory exists
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		return nil, fmt.Errorf("create plugins dir: %w", err)
	}

	parsed := ParsePluginRef(ref)

	var srcDir, tmpDir string
	var err error

	switch {
	case IsLocalPath(parsed.Name):
		// Install from local path
		srcDir, err = filepath.Abs(parsed.Name)
		if err != nil {
			return nil, fmt.Errorf("resolve path: %w", err)
		}

	case IsGitURL(parsed.Name):
		// Clone from git
		tmpDir, srcDir, err = cloneSingle(parsed.Name, pluginsDir)
		if err != nil {
			return nil, fmt.Errorf("clone plugin: %w", err)
		}
		defer os.RemoveAll(tmpDir)

	default:
		// Try hub first, then fall back to registry
		info, hubErr := m.hubClient.GetInfo(parsed.Name)
		if hubErr == nil {
			// Install from hub
			version := parsed.Version
			if version == "" {
				version = info.Version
			}
			return m.installFromHub(ctx, parsed.Name, version, pluginsDir)
		}

		// Fall back to registry
		tmpDir, srcDir, err = cloneFromRegistry(registryRoot, parsed.Name, pluginsDir)
		if err != nil {
			return nil, fmt.Errorf("install %q: %w", ref, err)
		}
		defer os.RemoveAll(tmpDir)
	}

	// Parse manifest to determine destination
	manifest, err := plugin.ParseManifest(filepath.Join(srcDir, "manifest.yaml"))
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	// Determine destination based on type
	destDir := m.destDirForType(manifest.Type, manifest.Name)
	if _, err := os.Stat(destDir); err == nil {
		return nil, fmt.Errorf("plugin %s already installed", manifest.Name)
	}

	// Copy to destination
	if err := CopyDir(srcDir, destDir); err != nil {
		return nil, fmt.Errorf("copy plugin: %w", err)
	}

	return &InstalledPlugin{
		Manifest: manifest,
		Path:     destDir,
	}, nil
}

// installFromHub downloads and installs from the plugin hub.
func (m *Installer) installFromHub(ctx context.Context, name, version, pluginsDir string) (*InstalledPlugin, error) {
	// Download from hub
	extractDir, err := m.hubClient.Download(name, version, pluginsDir)
	if err != nil {
		return nil, fmt.Errorf("download from hub: %w", err)
	}
	defer os.RemoveAll(filepath.Dir(extractDir))

	// Parse manifest
	manifest, err := plugin.ParseManifest(filepath.Join(extractDir, "manifest.yaml"))
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	// Determine destination
	destDir := m.destDirForType(manifest.Type, manifest.Name)
	if _, err := os.Stat(destDir); err == nil {
		return nil, fmt.Errorf("plugin %s already installed", manifest.Name)
	}

	// Copy to destination
	if err := CopyDir(extractDir, destDir); err != nil {
		return nil, fmt.Errorf("copy plugin: %w", err)
	}

	return &InstalledPlugin{
		Manifest: manifest,
		Path:     destDir,
	}, nil
}

// destDirForType returns the destination directory for a plugin based on its type.
func (m *Installer) destDirForType(pluginType plugin.PluginType, name string) string {
	pluginsDir := m.PluginsDir()
	switch pluginType {
	case plugin.TypeTool:
		return filepath.Join(pluginsDir, "tools", name)
	case plugin.TypeAgent:
		return filepath.Join(pluginsDir, "agents", name)
	case plugin.TypeSkill:
		return filepath.Join(pluginsDir, "skills", name)
	case plugin.TypeService:
		return filepath.Join(pluginsDir, "services", name)
	default:
		return filepath.Join(pluginsDir, string(pluginType)+"s", name)
	}
}

// Uninstall removes an installed plugin.
func (m *Installer) Uninstall(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Find the plugin
	plugins, err := m.List()
	if err != nil {
		return err
	}

	for _, p := range plugins {
		if p.Manifest.Name == name {
			return os.RemoveAll(p.Path)
		}
	}

	return fmt.Errorf("plugin %q not found", name)
}

// Get returns a specific installed plugin by name.
func (m *Installer) Get(name string) (InstalledPlugin, error) {
	plugins, err := m.List()
	if err != nil {
		return InstalledPlugin{}, err
	}

	for _, p := range plugins {
		if p.Manifest.Name == name {
			return p, nil
		}
	}
	return InstalledPlugin{}, fmt.Errorf("plugin %q not found", name)
}

// ListByType returns installed plugins of a specific type.
func (m *Installer) ListByType(t plugin.PluginType) ([]InstalledPlugin, error) {
	all, err := m.List()
	if err != nil {
		return nil, err
	}

	var filtered []InstalledPlugin
	for _, p := range all {
		if p.Manifest.Type == t {
			filtered = append(filtered, p)
		}
	}
	return filtered, nil
}

// Search searches the hub for plugins matching the query.
func (m *Installer) Search(query string) ([]PluginSearchItem, error) {
	return m.hubClient.Search(query)
}

// GetHubInfo returns information about a plugin from the hub.
func (m *Installer) GetHubInfo(name string) (*HubPlugin, error) {
	return m.hubClient.GetInfo(name)
}
