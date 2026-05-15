package plugin

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestParseServiceManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(path, []byte(`name: indexer
version: "1.0.0"
type: service
mode: api
description: Indexer service
service:
  address_env: QUARK_INDEXER_ADDR
  skill: SKILL.md
  readme: README.md
  proto_services:
    - quark.indexer.v1.IndexerService
  functions:
    - name: indexer_GetContext
      service: quark.indexer.v1.IndexerService
      method: GetContext
      request: quark.indexer.v1.QueryRequest
      response: quark.indexer.v1.ContextResponse
      description: Retrieve context for an agent-provided query embedding.
      risk_level: read
      idempotent: true
`), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	manifest, err := ParseManifest(path)
	if err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if manifest.Type != TypeService {
		t.Fatalf("type = %s, want %s", manifest.Type, TypeService)
	}
	if manifest.Service == nil || manifest.Service.AddressEnv != "QUARK_INDEXER_ADDR" {
		t.Fatalf("service config = %+v", manifest.Service)
	}
	if len(manifest.Service.Functions) != 1 || manifest.Service.Functions[0].Name != "indexer_GetContext" {
		t.Fatalf("service functions = %+v", manifest.Service.Functions)
	}
}

func TestServiceManifestDefaultsSkillAndReadme(t *testing.T) {
	manifest := &Manifest{
		Name:    "embedding",
		Version: "1.0.0",
		Type:    TypeService,
		Mode:    ModeAPI,
		Service: &ServiceConfig{
			Functions: []ServiceFunctionConfig{{
				Name:        "embedding_Embed",
				Service:     "quark.embedding.v1.EmbeddingService",
				Method:      "Embed",
				Request:     "quark.embedding.v1.EmbedRequest",
				Response:    "quark.embedding.v1.EmbedResponse",
				Description: "Embed text.",
				RiskLevel:   "read",
				Idempotent:  true,
			}},
		},
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if manifest.Service.Skill != "SKILL.md" {
		t.Fatalf("skill = %q, want SKILL.md", manifest.Service.Skill)
	}
	if manifest.Service.Readme != "README.md" {
		t.Fatalf("readme = %q, want README.md", manifest.Service.Readme)
	}
}

func TestServiceManifestRequiresFunctions(t *testing.T) {
	manifest := &Manifest{
		Name:    "embedding",
		Version: "1.0.0",
		Type:    TypeService,
		Mode:    ModeAPI,
		Service: &ServiceConfig{},
	}
	if err := manifest.Validate(); err == nil {
		t.Fatal("validate unexpectedly succeeded")
	}
}

func TestServiceFunctionConfigRejectsUnknownRisk(t *testing.T) {
	function := ServiceFunctionConfig{
		Name:        "embedding_Embed",
		Service:     "quark.embedding.v1.EmbeddingService",
		Method:      "Embed",
		Request:     "quark.embedding.v1.EmbedRequest",
		Response:    "quark.embedding.v1.EmbedResponse",
		Description: "Embed text.",
		RiskLevel:   "spicy",
	}
	if err := function.Validate(); err == nil {
		t.Fatal("validate unexpectedly succeeded")
	}
}

func TestRepositoryServiceManifestsDeclareFunctionsAndReadmes(t *testing.T) {
	manifests, err := filepath.Glob(filepath.Join("..", "..", "plugins", "services", "*", "manifest.yaml"))
	if err != nil {
		t.Fatalf("glob service manifests: %v", err)
	}
	if len(manifests) == 0 {
		t.Skip("repository service manifests not present")
	}
	for _, manifestPath := range manifests {
		manifestPath := manifestPath
		t.Run(filepath.Base(filepath.Dir(manifestPath)), func(t *testing.T) {
			manifest, err := ParseManifest(manifestPath)
			if err != nil {
				t.Fatalf("parse manifest: %v", err)
			}
			if manifest.Type != TypeService {
				t.Fatalf("manifest type = %s, want service", manifest.Type)
			}
			readmePath := filepath.Join(filepath.Dir(manifestPath), manifest.Service.Readme)
			if _, err := os.Stat(readmePath); err != nil {
				t.Fatalf("service README missing at %s: %v", readmePath, err)
			}
			for _, function := range manifest.Service.Functions {
				if serviceFunctionUnsafeChars.MatchString(function.Name) {
					t.Fatalf("function name contains unsafe characters: %q", function.Name)
				}
				if strings.TrimSpace(function.Name) == "" {
					t.Fatal("function name is empty")
				}
			}
		})
	}
}

var serviceFunctionUnsafeChars = regexp.MustCompile(`[^A-Za-z0-9_]+`)
