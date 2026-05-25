package pluginmanager

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// Registry owns the supervisor-visible plugin installation catalog. Bundled
// plugins are immutable product material; optional installations are persisted
// under a supervisor-owned root, never inside a space directory.
type Registry struct {
	bundled   *Installer
	installed *Installer
}

func NewRegistry(bundledDir, installedDir string) (*Registry, error) {
	if strings.TrimSpace(bundledDir) == "" {
		return nil, fmt.Errorf("bundled plugin directory is required")
	}
	if strings.TrimSpace(installedDir) == "" {
		return nil, fmt.Errorf("installed plugin directory is required")
	}
	return &Registry{bundled: NewInstaller(bundledDir), installed: NewInstaller(installedDir)}, nil
}

func (r *Registry) List() ([]InstalledPlugin, error) {
	bundled, err := r.bundled.List()
	if err != nil {
		return nil, err
	}
	installed, err := r.installed.List()
	if err != nil {
		return nil, err
	}
	byName := make(map[string]InstalledPlugin, len(bundled)+len(installed))
	for _, item := range bundled {
		byName[item.Manifest.Name] = item
	}
	for _, item := range installed {
		byName[item.Manifest.Name] = item
	}
	out := make([]InstalledPlugin, 0, len(byName))
	for _, item := range byName {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Manifest.Name < out[j].Manifest.Name })
	return out, nil
}

func (r *Registry) Get(name string) (InstalledPlugin, error) {
	name = ReferenceName(name)
	if name == "" {
		return InstalledPlugin{}, fmt.Errorf("plugin name is required")
	}
	if item, err := r.installed.Get(name); err == nil {
		return item, nil
	}
	return r.bundled.Get(name)
}

func (r *Registry) Install(ctx context.Context, ref string) (InstalledPlugin, error) {
	if item, err := r.Get(ref); err == nil {
		return item, nil
	}
	item, err := r.installed.Install(ctx, ref)
	if err != nil {
		return InstalledPlugin{}, err
	}
	return *item, nil
}

func (r *Registry) Search(query string) ([]PluginSearchItem, error) {
	return r.installed.Search(query)
}

func (r *Registry) GetHubInfo(name string) (*HubPlugin, error) {
	return r.installed.GetHubInfo(name)
}

// ReferenceName maps a space-selected reference to a global catalog name.
// Plugin storage identity is the manifest name; URI/path syntax does not
// become part of space persistence.
func ReferenceName(ref string) string {
	name := filepath.Base(strings.TrimSuffix(strings.TrimSpace(ref), "/"))
	for _, prefix := range []string{"service-", "agent-", "tool-"} {
		name = strings.TrimPrefix(name, prefix)
	}
	return name
}
