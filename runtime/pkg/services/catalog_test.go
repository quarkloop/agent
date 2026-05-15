package services

import (
	"encoding/json"
	"strings"
	"testing"

	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
)

func TestPromptBlockIncludesServiceSkillsAndRPCs(t *testing.T) {
	t.Parallel()

	block := PromptBlock([]*servicev1.ServiceDescriptor{{
		Name:    "indexer",
		Type:    "indexer",
		Version: "1.0.0",
		Address: "127.0.0.1:7301",
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:  "quark.indexer.v1.IndexerService",
			Method:   "GetContext",
			Request:  "quark.indexer.v1.QueryRequest",
			Response: "quark.indexer.v1.ContextResponse",
		}},
		Skills: []*servicev1.SkillDescriptor{{
			Name:     "service-indexer",
			Markdown: "# service-indexer\n\nUse query vectors.",
		}},
	}})

	for _, want := range []string{"Available Service Plugins", "indexer_GetContext", "indexer", "service-indexer", "Use query vectors."} {
		if !strings.Contains(block, want) {
			t.Fatalf("prompt block missing %q:\n%s", want, block)
		}
	}
}

func TestCatalogExposesServiceFunctions(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog([]*servicev1.ServiceDescriptor{{
		Name:    "indexer",
		Address: "127.0.0.1:7301",
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:  "quark.indexer.v1.IndexerService",
			Method:   "GetContext",
			Request:  "quark.indexer.v1.QueryRequest",
			Response: "quark.indexer.v1.ContextResponse",
		}},
	}})
	tools := catalog.ToolSchemas()
	if len(tools) != 1 || tools[0].Name != "indexer_GetContext" {
		t.Fatalf("tools = %+v", tools)
	}
	if catalog.Prompt() == "" {
		t.Fatal("catalog prompt is empty")
	}
	if len(catalog.Descriptors()) != 1 {
		t.Fatalf("descriptors = %d, want 1", len(catalog.Descriptors()))
	}
	if _, err := catalog.Execute(nil, "fs", "{}"); err == nil {
		t.Fatal("non-service function unexpectedly executed")
	}
}

func TestCatalogFromEnvUsesVersionedServiceCatalog(t *testing.T) {
	payload, err := servicekit.MarshalRuntimeServiceCatalog([]*servicev1.ServiceDescriptor{{
		Name:    "indexer",
		Type:    "indexer",
		Version: "1.0.0",
		Address: "127.0.0.1:7301",
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:     "quark.indexer.v1.IndexerService",
			Method:      "GetContext",
			Request:     "quark.indexer.v1.QueryRequest",
			Response:    "quark.indexer.v1.ContextResponse",
			Description: "Retrieve context.",
		}},
	}})
	if err != nil {
		t.Fatalf("marshal catalog: %v", err)
	}
	t.Setenv(EnvRuntimeCatalog, string(payload))

	catalog, err := CatalogFromEnv()
	if err != nil {
		t.Fatalf("catalog from env: %v", err)
	}
	if catalog == nil || catalog.Empty() {
		t.Fatal("expected service catalog")
	}
}

func TestCatalogFromEnvRejectsUnsupportedVersion(t *testing.T) {
	t.Setenv(EnvRuntimeCatalog, `{"version":999,"services":[]}`)

	_, err := CatalogFromEnv()
	if err == nil || !strings.Contains(err.Error(), "unsupported runtime service catalog version") {
		t.Fatalf("expected unsupported version error, got: %v", err)
	}
}

func TestServiceFunctionSchemaUsesRuntimeEmbeddingReferences(t *testing.T) {
	t.Parallel()

	params := requestParameters("quark.indexer.v1.IndexRequest")
	properties, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties missing: %+v", params)
	}
	if _, ok := properties["embeddingRef"]; !ok {
		t.Fatalf("embeddingRef missing from schema: %+v", properties)
	}
	required, ok := params["required"].([]string)
	if !ok {
		t.Fatalf("required missing: %+v", params)
	}
	for _, want := range []string{"chunkId", "textContent", "embeddingRef"} {
		if !containsString(required, want) {
			t.Fatalf("required missing %q: %+v", want, required)
		}
	}
}

func TestServiceFunctionSchemasIncludeKnowledgeContracts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		typeName string
		fields   []string
	}{
		{
			name:     "document extract text",
			typeName: "quark.document.v1.ExtractTextRequest",
			fields:   []string{"input", "maxChars"},
		},
		{
			name:     "ingestion start run",
			typeName: "quark.ingestion.v1.StartRunRequest",
			fields:   []string{"space", "title", "sources", "metadata"},
		},
		{
			name:     "citation verify grounding",
			typeName: "quark.citation.v1.VerifyGroundingRequest",
			fields:   []string{"claims"},
		},
		{
			name:     "memory put",
			typeName: "quark.memory.v1.PutRequest",
			fields:   []string{"space", "collection", "key", "value", "metadata", "provenance"},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			params := requestParameters(tt.typeName)
			properties, ok := params["properties"].(map[string]any)
			if !ok {
				t.Fatalf("properties missing: %+v", params)
			}
			for _, field := range tt.fields {
				if _, ok := properties[field]; !ok {
					t.Fatalf("field %q missing from %s schema: %+v", field, tt.typeName, properties)
				}
			}
		})
	}
}

func TestToolNameForIsDeterministicAndSafe(t *testing.T) {
	t.Parallel()

	got := ToolNameFor("build-release", "Dry Run")
	if got != "build_release_Dry_Run" {
		t.Fatalf("tool name = %q, want build_release_Dry_Run", got)
	}
	if again := ToolNameFor("build-release", "Dry Run"); again != got {
		t.Fatalf("tool name not deterministic: %q then %q", got, again)
	}
	if empty := ToolNameFor("", ""); empty != "service_call" {
		t.Fatalf("empty tool name = %q, want service_call", empty)
	}
}

func TestExecutorExpandsEmbeddingReferences(t *testing.T) {
	t.Parallel()

	executor := NewExecutor(nil)
	executor.embeddings["ref-1"] = []float32{0.25, -0.5}
	executor.embeddingInfo["ref-1"] = map[string]any{
		"provider":    "local",
		"model":       "local-hash-v1",
		"dimensions":  2,
		"contentHash": "abc123",
	}

	expanded, err := executor.expandRuntimeReferences("quark.indexer.v1.IndexRequest", `{"chunkId":"chunk","textContent":"text","embeddingRef":"ref-1"}`)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(expanded), &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["embeddingRef"]; ok {
		t.Fatalf("embeddingRef was not removed: %s", expanded)
	}
	vector, ok := payload["embedding"].([]any)
	if !ok || len(vector) != 2 {
		t.Fatalf("embedding was not expanded: %s", expanded)
	}
	metadata, ok := payload["embeddingMetadata"].(map[string]any)
	if !ok || metadata["provider"] != "local" || metadata["model"] != "local-hash-v1" {
		t.Fatalf("embedding metadata was not expanded: %s", expanded)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
