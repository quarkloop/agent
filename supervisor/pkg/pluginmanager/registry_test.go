package pluginmanager

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/quarkloop/pkg/plugin"
)

func TestRegistryResolvesBundledMaterialWithoutSpaceInstallation(t *testing.T) {
	bundled := t.TempDir()
	writeRegistryFixture(t, bundled, "agents", "quark-main", plugin.TypeAgent)
	registry, err := NewRegistry(bundled, filepath.Join(t.TempDir(), "installed"))
	if err != nil {
		t.Fatal(err)
	}

	item, err := registry.Get("quark-main")
	if err != nil {
		t.Fatalf("resolve bundled main agent: %v", err)
	}
	if item.Manifest.Type != plugin.TypeAgent || item.Path != filepath.Join(bundled, "agents", "quark-main") {
		t.Fatalf("bundled item = %+v", item)
	}
}

func TestRegistryPersistsOptionalInstallationsInSupervisorRoot(t *testing.T) {
	bundled := t.TempDir()
	writeRegistryFixture(t, bundled, "agents", "quark-main", plugin.TypeAgent)
	installed := filepath.Join(t.TempDir(), "installed")
	registry, err := NewRegistry(bundled, installed)
	if err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	writeManifest(t, source, "indexer", plugin.TypeService)

	item, err := registry.Install(context.Background(), source)
	if err != nil {
		t.Fatalf("install optional service: %v", err)
	}
	if item.Path != filepath.Join(installed, "services", "indexer") {
		t.Fatalf("installation path = %q", item.Path)
	}
	if _, err := os.Stat(item.Path); err != nil {
		t.Fatalf("stat installed plugin: %v", err)
	}
}

func TestReferenceNameMapsSelectionsToManifestIdentity(t *testing.T) {
	for input, want := range map[string]string{
		"quark/service-indexer":     "indexer",
		"/bundle/agents/quark-main": "quark-main",
		"tool-repo":                 "repo",
	} {
		if got := ReferenceName(input); got != want {
			t.Fatalf("ReferenceName(%q) = %q, want %q", input, got, want)
		}
	}
}

func writeRegistryFixture(t *testing.T, root, kind, name string, pluginType plugin.PluginType) {
	t.Helper()
	dir := filepath.Join(root, kind, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, dir, name, pluginType)
}

func writeManifest(t *testing.T, dir, name string, pluginType plugin.PluginType) {
	t.Helper()
	content := "name: " + name + "\nversion: \"1.0.0\"\ntype: " + string(pluginType) + "\n"
	switch pluginType {
	case plugin.TypeAgent:
		content += "agent:\n  profile: PROFILE.yaml\n  system: SYSTEM.md\n  skill: SKILL.md\n"
	case plugin.TypeService:
		content += `service:
  functions:
    - name: indexer_QueryContext
      service: quark.indexer.v1.IndexerService
      method: QueryContext
      request: quark.indexer.v1.QueryRequest
      response: quark.indexer.v1.ContextResponse
      description: Retrieve indexed context.
`
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.yaml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
