package pluginmanager

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/quarkloop/pkg/plugin"
)

// Catalog is the runtime-consumable plugin catalog resolved by the supervisor.
// Runtime loads exactly the plugin directories listed here and does not infer
// installed plugin state when a catalog is present.
type Catalog = plugin.RuntimeCatalog

type CatalogPlugin = plugin.RuntimeCatalogPlugin

func (m *Manager) SetCatalog(catalog *Catalog) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.catalog = catalog
}

func (m *Manager) loadCatalogLocked(ctx context.Context, catalog Catalog) error {
	for _, item := range catalog.Plugins {
		if item.Path == "" {
			return fmt.Errorf("plugin %q has empty path", item.Name)
		}
		manifest, err := plugin.ParseManifest(pluginManifestPath(item.Path))
		if err != nil {
			return fmt.Errorf("parse catalog plugin %s: %w", item.Name, err)
		}
		if item.Type != "" && manifest.Type != item.Type {
			return fmt.Errorf("catalog plugin %s type mismatch: manifest=%s catalog=%s", item.Name, manifest.Type, item.Type)
		}
		switch manifest.Type {
		case plugin.TypeTool:
			if err := m.loadToolLocked(ctx, manifest, item.Path); err != nil {
				return fmt.Errorf("load tool %s: %w", manifest.Name, err)
			}
		case plugin.TypeProvider:
			if err := m.loadProviderLocked(ctx, manifest, item.Path); err != nil {
				return fmt.Errorf("load provider %s: %w", manifest.Name, err)
			}
		}
	}
	return nil
}

func pluginManifestPath(pluginDir string) string {
	return filepath.Join(pluginDir, "manifest.yaml")
}
