package plugin_test

import (
	"strings"
	"testing"

	"github.com/quarkloop/pkg/plugin"
)

func TestRuntimeCatalogValidate(t *testing.T) {
	catalog := plugin.NewRuntimeCatalog([]plugin.RuntimeCatalogPlugin{
		{
			Name: "fs",
			Type: plugin.TypeTool,
			Path: "/tmp/fs",
			Schema: &plugin.ToolSchema{
				Name: "fs",
			},
		},
		{
			Name: "openrouter",
			Type: plugin.TypeProvider,
			Path: "/tmp/openrouter",
		},
		{
			Name: "quark-knowledge",
			Type: plugin.TypeAgent,
			Path: "/tmp/quark-knowledge",
			AgentProfile: &plugin.AgentProfile{
				ID:   "quark-knowledge",
				Name: "Quark Knowledge",
				Model: plugin.AgentProfileModel{
					Provider: "openrouter",
					Model:    "openai/gpt-5-mini",
				},
			},
		},
	})

	if err := catalog.Validate(); err != nil {
		t.Fatalf("expected valid catalog, got: %v", err)
	}
}

func TestRuntimeCatalogRejectsUnsupportedVersion(t *testing.T) {
	catalog := plugin.RuntimeCatalog{Version: 999}
	err := catalog.Validate()
	if err == nil || !strings.Contains(err.Error(), "unsupported runtime plugin catalog version") {
		t.Fatalf("expected unsupported version error, got: %v", err)
	}
}

func TestRuntimeCatalogRequiresAgentProfile(t *testing.T) {
	catalog := plugin.NewRuntimeCatalog([]plugin.RuntimeCatalogPlugin{{
		Name: "quark-system",
		Type: plugin.TypeAgent,
		Path: "/tmp/quark-system",
	}})
	err := catalog.Validate()
	if err == nil || !strings.Contains(err.Error(), "agent profile") {
		t.Fatalf("expected agent profile validation error, got: %v", err)
	}
}
