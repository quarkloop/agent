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
  transport: nats
  subject_prefix: svc.indexer.v1
  queue_group: q.service.v1.indexer
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
	if manifest.Service.Transport != "nats" || manifest.Service.SubjectPrefix != "svc.indexer.v1" || manifest.Service.QueueGroup != "q.service.v1.indexer" {
		t.Fatalf("service nats config = %+v", manifest.Service)
	}
	if len(manifest.Service.Functions) != 1 || manifest.Service.Functions[0].Name != "indexer_GetContext" {
		t.Fatalf("service functions = %+v", manifest.Service.Functions)
	}
	if manifest.Service.Functions[0].Subject != "svc.indexer.v1.get_context" {
		t.Fatalf("service function subject = %q", manifest.Service.Functions[0].Subject)
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
	if manifest.Service.Transport != "nats" || manifest.Service.SubjectPrefix != "svc.embedding.v1" || manifest.Service.QueueGroup != "q.service.v1.embedding" {
		t.Fatalf("service nats defaults = %+v", manifest.Service)
	}
	if manifest.Service.Health.Protocol != "nats_service" || manifest.Service.Health.Timeout != "5s" {
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
			if manifest.Service.Transport != "nats" {
				t.Fatalf("service transport = %q, want nats", manifest.Service.Transport)
			}
			if !strings.HasPrefix(manifest.Service.SubjectPrefix, "svc.") {
				t.Fatalf("service subject prefix = %q", manifest.Service.SubjectPrefix)
			}
			if !strings.HasPrefix(manifest.Service.QueueGroup, "q.service.v1.") {
				t.Fatalf("service queue group = %q", manifest.Service.QueueGroup)
			}
			if manifest.Service.Health.Protocol != "nats_service" {
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
			readmeData, err := os.ReadFile(readmePath)
			if err != nil {
				t.Fatalf("service README missing at %s: %v", readmePath, err)
			}
			skillData, err := os.ReadFile(filepath.Join(filepath.Dir(manifestPath), manifest.Service.Skill))
			if err != nil {
				t.Fatalf("service SKILL missing at %s: %v", manifest.Service.Skill, err)
			}
			readme := string(readmeData)
			skill := string(skillData)
			for _, function := range manifest.Service.Functions {
				if serviceFunctionUnsafeChars.MatchString(function.Name) {
					t.Fatalf("function name contains unsafe characters: %q", function.Name)
				}
				if strings.TrimSpace(function.Name) == "" {
					t.Fatal("function name is empty")
				}
				if !strings.HasPrefix(function.Subject, manifest.Service.SubjectPrefix+".") {
					t.Fatalf("function %q subject %q does not use service prefix %q", function.Name, function.Subject, manifest.Service.SubjectPrefix)
				}
				if !strings.Contains(readme, "`"+function.Name+"`") {
					t.Fatalf("README does not list service function %q", function.Name)
				}
				if !strings.Contains(skill, "`"+function.Name+"`") {
					t.Fatalf("SKILL does not describe service function %q", function.Name)
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
			assertConcreteAgentRefs(t, manifest.Name, manifest.Agent.Tools, manifest.Agent.Services, profile.Permissions.Tools, profile.Permissions.Services)
			for _, name := range []string{manifest.Agent.System, manifest.Agent.Skill} {
				if _, err := os.Stat(filepath.Join(filepath.Dir(manifestPath), name)); err != nil {
					t.Fatalf("agent file %s missing: %v", name, err)
				}
			}
		})
	}
}

func assertConcreteAgentRefs(t *testing.T, name string, manifestTools, manifestServices, profileTools, profileServices []string) {
	t.Helper()
	for _, ref := range append(append([]string{}, manifestTools...), profileTools...) {
		if strings.Contains(ref, "*") {
			t.Fatalf("%s declares wildcard tool permission %q; agent profiles must use concrete tools", name, ref)
		}
	}
	for _, ref := range append(append([]string{}, manifestServices...), profileServices...) {
		if strings.Contains(ref, "*") {
			t.Fatalf("%s declares wildcard service permission %q; agent profiles must use concrete service functions", name, ref)
		}
	}
	if !sameStringSet(manifestTools, profileTools) {
		t.Fatalf("%s manifest tools %+v do not match profile tools %+v", name, manifestTools, profileTools)
	}
	if !sameStringSet(manifestServices, profileServices) {
		t.Fatalf("%s manifest services %+v do not match profile services %+v", name, manifestServices, profileServices)
	}
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	seen := make(map[string]int, len(left))
	for _, value := range left {
		seen[value]++
	}
	for _, value := range right {
		seen[value]--
		if seen[value] < 0 {
			return false
		}
	}
	return true
}

func TestQuarkKnowledgeProfileDeclaresConcreteServiceFunctions(t *testing.T) {
	profile, err := ParseAgentProfile(filepath.Join("..", "..", "plugins", "agents", "quark-knowledge", "PROFILE.yaml"))
	if err != nil {
		t.Fatalf("parse quark knowledge profile: %v", err)
	}
	permissions := make(map[string]bool, len(profile.Permissions.Services))
	for _, name := range profile.Permissions.Services {
		permissions[name] = true
	}
	for _, want := range []string{
		"document_DetectType",
		"document_ParseBytes",
		"document_ExtractText",
		"document_ExtractLayout",
		"document_GetPages",
		"document_ExtractTables",
		"document_ExtractImages",
		"document_RunOCR",
		"ingestion_StartRun",
		"ingestion_GetRun",
		"ingestion_ResumeRun",
		"ingestion_UpdateSourceState",
		"ingestion_ListIncompleteSources",
		"ingestion_ListArtifacts",
		"embedding_Embed",
		"indexer_IndexDocument",
		"indexer_GetContext",
		"indexer_DeleteChunk",
		"citation_ResolveSpans",
		"citation_VerifyGrounding",
		"core_CreateWorkspaceMutationPlan",
		"core_ApproveWorkspaceMutationPlan",
		"core_RequestApproval",
		"core_EvaluatePolicy",
		"core_RecordAuditEvent",
		"core_PutArtifact",
	} {
		if !permissions[want] {
			t.Fatalf("quark knowledge profile missing service function permission %q", want)
		}
	}
}

func TestIngestionServiceContractTracksResumableSourceState(t *testing.T) {
	manifest, err := ParseManifest(filepath.Join("..", "..", "plugins", "services", "ingestion", "manifest.yaml"))
	if err != nil {
		t.Fatalf("parse ingestion manifest: %v", err)
	}
	found := false
	for _, function := range manifest.Service.Functions {
		if function.Name == "ingestion_ListIncompleteSources" {
			found = true
			if function.RiskLevel != "read" || !function.Idempotent {
				t.Fatalf("ingestion_ListIncompleteSources contract = %+v", function)
			}
		}
	}
	if !found {
		t.Fatal("ingestion service missing ingestion_ListIncompleteSources")
	}
	protoData, err := os.ReadFile(filepath.Join("..", "..", "proto", "quark", "ingestion", "v1", "ingestion.proto"))
	if err != nil {
		t.Fatalf("read ingestion proto: %v", err)
	}
	protoText := string(protoData)
	for _, want := range []string{
		"string file_path",
		"SourceStepState extraction",
		"SourceStepState structuring",
		"SourceStepState embedding",
		"SourceStepState indexing",
		"SourceStepState citation",
		"string last_error",
	} {
		if !strings.Contains(protoText, want) {
			t.Fatalf("ingestion proto missing resumable state field %q", want)
		}
	}
}

func TestQuarkSystemProfileDeclaresConcreteServiceFunctions(t *testing.T) {
	profile, err := ParseAgentProfile(filepath.Join("..", "..", "plugins", "agents", "quark-system", "PROFILE.yaml"))
	if err != nil {
		t.Fatalf("parse quark system profile: %v", err)
	}
	if len(profile.Permissions.Tools) != 0 {
		t.Fatalf("quark system profile should not grant shell/tool fallback permissions: %+v", profile.Permissions.Tools)
	}
	permissions := make(map[string]bool, len(profile.Permissions.Services))
	for _, name := range profile.Permissions.Services {
		permissions[name] = true
	}
	for _, want := range []string{
		"system_Snapshot",
		"system_GetOSInfo",
		"system_GetKernelInfo",
		"system_GetUptime",
		"system_ListPackages",
		"system_ListServices",
		"system_ListUsers",
		"system_ListMounts",
		"system_GetDiskUsage",
		"system_ListProcesses",
		"system_ListPorts",
		"system_ListNetworkConnections",
		"system_ReadLogs",
		"system_GetMetrics",
		"system_KillProcess",
		"system_RestartService",
	} {
		if !permissions[want] {
			t.Fatalf("quark system profile missing service function permission %q", want)
		}
	}
	for _, want := range []string{"system_KillProcess", "system_RestartService"} {
		found := false
		for _, approval := range profile.Approval.RequiredFor {
			if approval == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("quark system profile missing approval requirement %q", want)
		}
	}
}

func TestQuarkDevOpsProfileDeclaresConcreteServiceFunctions(t *testing.T) {
	profile, err := ParseAgentProfile(filepath.Join("..", "..", "plugins", "agents", "quark-devops", "PROFILE.yaml"))
	if err != nil {
		t.Fatalf("parse quark devops profile: %v", err)
	}
	for _, tool := range profile.Permissions.Tools {
		if tool == "bash" {
			t.Fatal("quark devops profile should prefer typed services instead of bash fallback")
		}
	}
	permissions := make(map[string]bool, len(profile.Permissions.Services))
	for _, name := range profile.Permissions.Services {
		permissions[name] = true
	}
	for _, want := range []string{
		"repo_Status",
		"repo_Diff",
		"repo_GetBranch",
		"repo_ListChangedFiles",
		"repo_ApplyPatch",
		"repo_Commit",
		"repo_GenerateReleaseNotes",
		"build_DetectProject",
		"build_ResolveTask",
		"build_RunTask",
		"build_CreateArtifact",
		"test_DiscoverTests",
		"test_RunTests",
		"test_ExplainFailure",
		"container_BuildImage",
		"container_ListImages",
		"container_PlanRun",
		"deploy_Plan",
		"deploy_Apply",
		"policy_EvaluateChange",
		"build_release_DryRun",
		"build_release_Init",
		"build_release_Release",
	} {
		if !permissions[want] {
			t.Fatalf("quark devops profile missing service function permission %q", want)
		}
	}
}

func TestCoreServiceManifestDeclaresGovernanceContracts(t *testing.T) {
	manifestPath := filepath.Join("..", "..", "plugins", "services", "core", "manifest.yaml")
	manifest, err := ParseManifest(manifestPath)
	if err != nil {
		t.Fatalf("parse core manifest: %v", err)
	}
	functions := make(map[string]ServiceFunctionConfig, len(manifest.Service.Functions))
	for _, function := range manifest.Service.Functions {
		functions[function.Name] = function
	}
	for _, name := range []string{
		"core_SetConfig",
		"core_CreateWorkspaceMutationPlan",
		"core_ApproveWorkspaceMutationPlan",
		"core_ScheduleRun",
	} {
		function, ok := functions[name]
		if !ok {
			t.Fatalf("core service missing function %q", name)
		}
		if !function.ApprovalRequired {
			t.Fatalf("core function %q must require approval", name)
		}
	}
	for _, name := range []string{"core_GetSecretRef", "core_EvaluatePolicy", "core_PutArtifact", "core_ListEvents"} {
		if _, ok := functions[name]; !ok {
			t.Fatalf("core service missing function %q", name)
		}
	}

	protoData, err := os.ReadFile(filepath.Join("..", "..", "proto", "quark", "core", "v1", "core.proto"))
	if err != nil {
		t.Fatalf("read core proto: %v", err)
	}
	protoText := string(protoData)
	for _, want := range []string{
		"bool redacted",
		"repeated string redaction_reasons",
		"uint64 sequence",
		"bool allowed",
		"repeated string violations",
		"repeated string required_approvals",
	} {
		if !strings.Contains(protoText, want) {
			t.Fatalf("core proto missing governance field %q", want)
		}
	}
}
