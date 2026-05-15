package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/quarkloop/pkg/plugin"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
)

func TestRuntimePluginCatalogEntryIncludesToolSchemaAndSkill(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("Use the tool carefully.\n"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	entry, err := runtimePluginCatalogEntryFromInstalled(pluginmanager.InstalledPlugin{
		Path: dir,
		Manifest: &plugin.Manifest{
			Name: "fs",
			Type: plugin.TypeTool,
			Tool: &plugin.ToolConfig{
				Schema: plugin.ToolSchema{
					Name:        "fs",
					Description: "filesystem",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("catalog entry: %v", err)
	}

	if entry.Name != "fs" || entry.Type != plugin.TypeTool || entry.Path != dir {
		t.Fatalf("unexpected entry identity: %+v", entry)
	}
	if entry.Schema == nil || entry.Schema.Name != "fs" {
		t.Fatalf("tool schema missing: %+v", entry)
	}
	if entry.Skill != "Use the tool carefully." {
		t.Fatalf("skill = %q", entry.Skill)
	}
}

func TestRuntimePluginCatalogEntryIncludesAgentProfile(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"PROFILE.yaml": "id: quark-knowledge\nname: Quark Knowledge\n",
		"SYSTEM.md":    "You are Quark Knowledge.\n",
		"SKILL.md":     "Use Knowledge services.\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	manifest := &plugin.Manifest{
		Name: "quark-knowledge",
		Type: plugin.TypeAgent,
		Agent: &plugin.AgentConfig{
			Profile: "PROFILE.yaml",
			System:  "SYSTEM.md",
			Skill:   "SKILL.md",
		},
	}
	entry, err := runtimePluginCatalogEntryFromInstalled(pluginmanager.InstalledPlugin{
		Path:     dir,
		Manifest: manifest,
	})
	if err != nil {
		t.Fatalf("catalog entry: %v", err)
	}
	if entry.AgentProfile == nil || entry.AgentProfile.ID != "quark-knowledge" {
		t.Fatalf("agent profile missing: %+v", entry.AgentProfile)
	}
	if entry.SystemPrompt != "You are Quark Knowledge." {
		t.Fatalf("system prompt = %q", entry.SystemPrompt)
	}
	if entry.Skill != "Use Knowledge services." {
		t.Fatalf("skill = %q", entry.Skill)
	}
}

func TestRuntimePluginCatalogUsesVersionedContract(t *testing.T) {
	catalog := plugin.NewRuntimeCatalog([]runtimePluginCatalogEntry{{
		Name: "quark-knowledge",
		Type: plugin.TypeAgent,
		Path: "/plugins/quark-knowledge",
		AgentProfile: &plugin.AgentProfile{
			ID:   "quark-knowledge",
			Name: "Quark Knowledge",
		},
	}})
	if err := catalog.Validate(); err != nil {
		t.Fatalf("catalog should validate: %v", err)
	}
	if catalog.Version != plugin.RuntimeCatalogVersion {
		t.Fatalf("catalog version = %d", catalog.Version)
	}
}

func TestApplyServiceFunctionMetadata(t *testing.T) {
	desc := &servicev1.ServiceDescriptor{
		Name: "indexer",
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:  "quark.indexer.v1.IndexerService",
			Method:   "GetContext",
			Request:  "old.Request",
			Response: "old.Response",
		}},
	}
	manifest := &plugin.Manifest{
		Name: "indexer",
		Type: plugin.TypeService,
		Service: &plugin.ServiceConfig{
			Functions: []plugin.ServiceFunctionConfig{{
				Name:        "indexer_GetContext",
				Service:     "quark.indexer.v1.IndexerService",
				Method:      "GetContext",
				Request:     "quark.indexer.v1.QueryRequest",
				Response:    "quark.indexer.v1.ContextResponse",
				Description: "Retrieve context using a query embedding.",
				RiskLevel:   "read",
				Idempotent:  true,
			}},
		},
	}

	if err := applyServiceFunctionMetadata(desc, manifest); err != nil {
		t.Fatalf("apply metadata: %v", err)
	}
	rpc := desc.GetRpcs()[0]
	if rpc.GetRequest() != "quark.indexer.v1.QueryRequest" || rpc.GetResponse() != "quark.indexer.v1.ContextResponse" {
		t.Fatalf("rpc types were not updated: %+v", rpc)
	}
	if rpc.GetDescription() != "Retrieve context using a query embedding." {
		t.Fatalf("description = %q", rpc.GetDescription())
	}
}

func TestApplyServiceFunctionMetadataRequiresEveryRPC(t *testing.T) {
	desc := &servicev1.ServiceDescriptor{
		Name: "indexer",
		Rpcs: []*servicev1.RpcDescriptor{{
			Service: "quark.indexer.v1.IndexerService",
			Method:  "GetContext",
		}},
	}
	manifest := &plugin.Manifest{
		Name: "indexer",
		Type: plugin.TypeService,
		Service: &plugin.ServiceConfig{
			Functions: []plugin.ServiceFunctionConfig{{
				Name:        "indexer_IndexDocument",
				Service:     "quark.indexer.v1.IndexerService",
				Method:      "IndexDocument",
				Request:     "quark.indexer.v1.IndexRequest",
				Response:    "quark.indexer.v1.IndexStatus",
				Description: "Persist one canonical index record.",
			}},
		},
	}

	if err := applyServiceFunctionMetadata(desc, manifest); err == nil {
		t.Fatal("apply metadata unexpectedly succeeded")
	}
}
