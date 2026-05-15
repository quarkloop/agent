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
	if manifest.Service.Health.Protocol != "grpc_health_v1" || manifest.Service.Health.Timeout != "5s" {
		t.Fatalf("health = %+v", manifest.Service.Health)
	}
	if manifest.Service.Readiness.MinVersion != manifest.Version {
		t.Fatalf("readiness min version = %q, want %q", manifest.Service.Readiness.MinVersion, manifest.Version)
	}
}

func TestAgentManifestDefaultsProfileSystemAndSkill(t *testing.T) {
	manifest := &Manifest{
		Name:    "quark-knowledge",
		Version: "1.0.0",
		Type:    TypeAgent,
		Mode:    ModeAPI,
		Agent:   &AgentConfig{},
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if manifest.Agent.Profile != "PROFILE.yaml" {
		t.Fatalf("profile = %q, want PROFILE.yaml", manifest.Agent.Profile)
	}
	if manifest.Agent.System != "SYSTEM.md" {
		t.Fatalf("system = %q, want SYSTEM.md", manifest.Agent.System)
	}
	if manifest.Agent.Skill != "SKILL.md" {
		t.Fatalf("skill = %q, want SKILL.md", manifest.Agent.Skill)
	}
}

func TestParseAgentProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "PROFILE.yaml")
	if err := os.WriteFile(path, []byte(`id: quark-knowledge
name: Quark Knowledge
description: Reads and indexes workspace knowledge.
model:
  provider: openrouter
  model: anthropic/claude-sonnet-4.5
prompt:
  system: SYSTEM.md
  skills:
    - SKILL.md
permissions:
  tools:
    - fs.read
  services:
    - indexer.*
memory:
  scope: space
approval:
  required_for:
    - workspace.write
handoff:
  can_delegate_to:
    - quark-devops
evaluation:
  required_checks:
    - citation_coverage
`), 0o644); err != nil {
		t.Fatalf("write profile: %v", err)
	}

	profile, err := ParseAgentProfile(path)
	if err != nil {
		t.Fatalf("parse profile: %v", err)
	}
	if profile.ID != "quark-knowledge" || profile.Name != "Quark Knowledge" {
		t.Fatalf("profile identity = %+v", profile)
	}
	if len(profile.Permissions.Services) != 1 || profile.Permissions.Services[0] != "indexer.*" {
		t.Fatalf("profile permissions = %+v", profile.Permissions)
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
			if manifest.Service.Health.Protocol != "grpc_health_v1" {
				t.Fatalf("service health protocol = %q", manifest.Service.Health.Protocol)
			}
			if manifest.Service.Health.Service == "" {
				t.Fatal("service health service is required")
			}
			if !manifest.Service.Readiness.Required {
				t.Fatal("service readiness must be required")
			}
			if manifest.Service.Readiness.MinVersion == "" {
				t.Fatal("service readiness min_version is required")
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

func TestRepositoryAgentManifestsDeclareProfiles(t *testing.T) {
	manifests, err := filepath.Glob(filepath.Join("..", "..", "plugins", "agents", "*", "manifest.yaml"))
	if err != nil {
		t.Fatalf("glob agent manifests: %v", err)
	}
	if len(manifests) == 0 {
		t.Skip("repository agent manifests not present")
	}
	for _, manifestPath := range manifests {
		manifestPath := manifestPath
		t.Run(filepath.Base(filepath.Dir(manifestPath)), func(t *testing.T) {
			manifest, err := ParseManifest(manifestPath)
			if err != nil {
				t.Fatalf("parse manifest: %v", err)
			}
			if manifest.Type != TypeAgent {
				t.Fatalf("manifest type = %s, want agent", manifest.Type)
			}
			profile, err := ParseAgentProfile(filepath.Join(filepath.Dir(manifestPath), manifest.Agent.Profile))
			if err != nil {
				t.Fatalf("parse agent profile: %v", err)
			}
			if profile.ID != manifest.Name {
				t.Fatalf("profile id = %q, want manifest name %q", profile.ID, manifest.Name)
			}
			for _, name := range []string{manifest.Agent.System, manifest.Agent.Skill} {
				if _, err := os.Stat(filepath.Join(filepath.Dir(manifestPath), name)); err != nil {
					t.Fatalf("agent file %s missing: %v", name, err)
				}
			}
		})
	}
}
