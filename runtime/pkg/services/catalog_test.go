package services

import (
	"encoding/json"
	"strings"
	"testing"

	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
)

func TestPromptBlockIncludesServiceSkillsAndRPCs(t *testing.T) {
	t.Parallel()

	block := PromptBlock([]*servicev1.ServiceDescriptor{{
		Name:    "indexer",
		Type:    "indexer",
		Version: "1.0.0",
		Address: "127.0.0.1:7301",
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:       "quark.indexer.v1.IndexerService",
			Method:        "QueryContext",
			Request:       "quark.indexer.v1.QueryRequest",
			Response:      "quark.indexer.v1.ContextResponse",
			FunctionName:  "indexer_QueryContext",
			Subject:       "svc.indexer.v1.query_context",
			RiskLevel:     "read",
			TimeoutMillis: 30000,
		}},
		Skills: []*servicev1.SkillDescriptor{{
			Name:     "service-indexer",
			Markdown: "# service-indexer\n\nUse query vectors.",
		}},
	}})

	for _, want := range []string{"Available Service Plugins", "indexer_QueryContext", "indexer", "service-indexer", "Use query vectors."} {
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
			Service:      "quark.indexer.v1.IndexerService",
			Method:       "QueryContext",
			Request:      "quark.indexer.v1.QueryRequest",
			Response:     "quark.indexer.v1.ContextResponse",
			FunctionName: "indexer_QueryContext",
			Subject:      "svc.indexer.v1.query_context",
		}},
	}})
	tools := catalog.ToolSchemas()
	if len(tools) != 1 || tools[0].Name != "indexer_QueryContext" {
		t.Fatalf("tools = %+v", tools)
	}
	if catalog.Prompt() == "" {
		t.Fatal("catalog prompt is empty")
	}
	if len(catalog.Descriptors()) != 1 {
		t.Fatalf("descriptors = %d, want 1", len(catalog.Descriptors()))
	}
	if _, err := catalog.Execute(nil, "io_Read", "{}"); err == nil {
		t.Fatal("non-catalog function unexpectedly executed without descriptor")
	}
}

func TestServiceFunctionOperationUsesCatalogSubjectAsRouteAuthority(t *testing.T) {
	t.Parallel()

	operation, err := serviceFunctionOperation(resolvedRPC{rpc: &servicev1.RpcDescriptor{
		Service:      "quark.indexer.v1.IndexerService",
		Method:       "QueryContext",
		Owner:        "wrong-owner",
		FunctionName: "wrong_Function",
		Subject:      "svc.indexer.v1.query_context",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if operation.Subject != "svc.indexer.v1.query_context" || operation.Owner != "indexer" || operation.Function != "query_context" {
		t.Fatalf("operation = %+v", operation)
	}
	if _, err := serviceFunctionOperation(resolvedRPC{rpc: &servicev1.RpcDescriptor{
		Service: "quark.indexer.v1.IndexerService",
		Method:  "QueryContext",
	}}); err == nil {
		t.Fatal("catalog RPC without a concrete subject was accepted")
	}
}

func TestServiceFunctionSchemaUsesRuntimeEmbeddingReferences(t *testing.T) {
	t.Parallel()

	embedParams := requestParameters("quark.gateway.v1.EmbedRequest")
	embedProperties, ok := embedParams["properties"].(map[string]any)
	if !ok {
		t.Fatalf("embedding properties missing: %+v", embedParams)
	}
	for _, want := range []string{"inputs", "inputRef", "contentRef", "pageRef", "imageRef"} {
		if _, ok := embedProperties[want]; !ok {
			t.Fatalf("embedding schema missing %q: %+v", want, embedProperties)
		}
	}
	if required, ok := embedParams["required"].([]string); ok && containsString(required, "inputs") {
		t.Fatalf("embedding inputs should be replaceable by runtime references, required=%+v", required)
	}

	params := requestParameters("quark.indexer.v1.UpsertChunkRequest")
	properties, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("properties missing: %+v", params)
	}
	if _, ok := properties["embeddingRef"]; !ok {
		t.Fatalf("embeddingRef missing from schema: %+v", properties)
	}
	if _, ok := properties["textContentRef"]; !ok {
		t.Fatalf("textContentRef missing from schema: %+v", properties)
	}
	required, ok := params["required"].([]string)
	if !ok {
		t.Fatalf("required missing: %+v", params)
	}
	for _, want := range []string{"chunkId", "embeddingRef"} {
		if !containsString(required, want) {
			t.Fatalf("required missing %q: %+v", want, required)
		}
	}
	if containsString(required, "textContent") {
		t.Fatalf("textContent should be replaceable by textContentRef, required=%+v", required)
	}

	deleteParams := requestParameters("quark.indexer.v1.DeleteChunkRequest")
	deleteRequired, ok := deleteParams["required"].([]string)
	if !ok || !containsString(deleteRequired, "chunkId") {
		t.Fatalf("DeleteChunk required fields missing chunkId: %+v", deleteParams)
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
			name:     "runstate start run",
			typeName: "quark.runstate.v1.StartRunRequest",
			fields:   []string{"space", "title", "items", "metadata", "kind"},
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

	got := ToolNameFor("devops", "Dry Run Release")
	if got != "devops_Dry_Run_Release" {
		t.Fatalf("tool name = %q, want devops_Dry_Run_Release", got)
	}
	if again := ToolNameFor("devops", "Dry Run Release"); again != got {
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
		"provider":    "fixture",
		"model":       "fixture/embed",
		"dimensions":  2,
		"contentHash": "abc123",
	}

	expanded, err := executor.expandRuntimeReferences("quark.indexer.v1.UpsertChunkRequest", `{"chunkId":"chunk","textContent":"text","embeddingRef":"ref-1"}`)
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
	if !ok || metadata["provider"] != "fixture" || metadata["model"] != "fixture/embed" {
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
