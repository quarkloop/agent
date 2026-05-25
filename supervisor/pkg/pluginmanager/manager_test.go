package pluginmanager

import (
	"os"
	"path/filepath"
	"testing"

	plugin "github.com/quarkloop/pkg/plugin"
)

func TestInstallerListsServicePlugins(t *testing.T) {
	pluginsDir := t.TempDir()
	serviceDir := filepath.Join(pluginsDir, "services", "indexer")
	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		t.Fatalf("mkdir service dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(serviceDir, "manifest.yaml"), []byte(`name: indexer
version: "1.0.0"
type: service
mode: api
description: Indexer service
service:
  address_env: QUARK_INDEXER_ADDR
  skill: SKILL.md
  readme: README.md
  functions:
    - name: indexer_QueryContext
      service: quark.indexer.v1.IndexerService
      method: QueryContext
      request: quark.indexer.v1.QueryRequest
      response: quark.indexer.v1.ContextResponse
      description: Retrieve context.
      risk_level: read
      idempotent: true
`), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	plugins, err := NewInstaller(pluginsDir).ListByType(plugin.TypeService)
	if err != nil {
		t.Fatalf("list service plugins: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("plugins = %d, want 1", len(plugins))
	}
	if plugins[0].Manifest.Name != "indexer" {
		t.Fatalf("plugin = %+v", plugins[0].Manifest)
	}
}
