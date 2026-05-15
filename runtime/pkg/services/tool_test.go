package services

import (
	"encoding/json"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

func TestNormalizeStringMapArgumentsCoercesNestedMetadataValues(t *testing.T) {
	raw := `{
		"chunkId": "chunk-1",
		"textContent": "hello",
		"embedding": [0.1, 0.2],
		"sourceMetadata": {"embedding_dimensions": 32, "verified": true},
		"document": {"metadata": {"pages": 2}},
		"facts": [{"subject": "s", "predicate": "p", "object": "o", "metadata": {"confidence": 0.9}}],
		"provenance": {"metadata": {"page": 1}}
	}`

	normalized, err := normalizeStringMapArguments("quark.indexer.v1.IndexRequest", raw)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	for _, want := range []string{
		`"embedding_dimensions":"32"`,
		`"verified":"true"`,
		`"pages":"2"`,
		`"confidence":"0.9"`,
		`"page":"1"`,
	} {
		if !strings.Contains(normalized, want) {
			t.Fatalf("normalized arguments missing %s:\n%s", want, normalized)
		}
	}

	msgType, err := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName("quark.indexer.v1.IndexRequest"))
	if err != nil {
		t.Fatalf("find message: %v", err)
	}
	msg := dynamicpb.NewMessage(msgType.Descriptor())
	if err := protojson.Unmarshal([]byte(normalized), msg); err != nil {
		t.Fatalf("protojson accepts normalized payload: %v\n%s", err, normalized)
	}
}

func TestNormalizeServiceArgumentJSONRepairsOnlyInvalidEscapes(t *testing.T) {
	raw := `{
		"chunkId": "chunk-1",
		"textContent": "2\Â × monitors with a valid newline\nkept",
		"sourceMetadata": {"filename": "receipt.md"}
	}`

	normalized, err := normalizeServiceArgumentJSON(raw)
	if err != nil {
		t.Fatalf("normalize service arguments: %v", err)
	}
	if strings.Contains(normalized, `\Â`) {
		t.Fatalf("invalid escape was not repaired:\n%s", normalized)
	}
	if !strings.Contains(normalized, `\nkept`) {
		t.Fatalf("valid JSON escape was not preserved:\n%s", normalized)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(normalized), &payload); err != nil {
		t.Fatalf("repaired arguments are not JSON: %v\n%s", err, normalized)
	}
	if got := payload["textContent"].(string); !strings.Contains(got, "2Â × monitors") {
		t.Fatalf("textContent = %q", got)
	}
}

func TestExecutorCapturesFilesystemContentRefsForIndexRequests(t *testing.T) {
	executor := NewExecutor(nil)

	result, err := executor.CaptureToolResult("fs", `{"command":"read","path":"/tmp/invoice.md"}`, `{"content":"# Invoice\nTotal due: EUR 18,450.00\n"}`)
	if err != nil {
		t.Fatalf("capture tool result: %v", err)
	}
	var toolPayload map[string]any
	if err := json.Unmarshal([]byte(result), &toolPayload); err != nil {
		t.Fatalf("tool result is not JSON: %v\n%s", err, result)
	}
	ref, ok := toolPayload["contentRef"].(string)
	if !ok || ref == "" {
		t.Fatalf("contentRef missing from tool result: %+v", toolPayload)
	}

	expanded, err := executor.expandRuntimeReferences("quark.indexer.v1.IndexRequest", `{"chunkId":"invoice","textContentRef":"`+ref+`"}`)
	if err != nil {
		t.Fatalf("expand refs: %v", err)
	}
	var indexPayload map[string]any
	if err := json.Unmarshal([]byte(expanded), &indexPayload); err != nil {
		t.Fatalf("expanded payload is not JSON: %v\n%s", err, expanded)
	}
	if _, ok := indexPayload["textContentRef"]; ok {
		t.Fatalf("textContentRef was not removed: %s", expanded)
	}
	if got := indexPayload["textContent"].(string); got != "# Invoice\nTotal due: EUR 18,450.00\n" {
		t.Fatalf("textContent = %q", got)
	}
}

func TestExecutorExpandsFilesystemContentRefsForEmbeddingRequests(t *testing.T) {
	executor := NewExecutor(nil)

	result, err := executor.CaptureToolResult("fs", `{"command":"extract_pdf","path":"/tmp/paper.pdf"}`, `{"content":"Attention Is All You Need\n"}`)
	if err != nil {
		t.Fatalf("capture tool result: %v", err)
	}
	var toolPayload map[string]any
	if err := json.Unmarshal([]byte(result), &toolPayload); err != nil {
		t.Fatalf("tool result is not JSON: %v\n%s", err, result)
	}
	ref, ok := toolPayload["contentRef"].(string)
	if !ok || ref == "" {
		t.Fatalf("contentRef missing from tool result: %+v", toolPayload)
	}

	expanded, err := executor.expandRuntimeReferences("quark.embedding.v1.EmbedRequest", `{"contentRef":"`+ref+`"}`)
	if err != nil {
		t.Fatalf("expand refs: %v", err)
	}
	var embedPayload map[string]any
	if err := json.Unmarshal([]byte(expanded), &embedPayload); err != nil {
		t.Fatalf("expanded payload is not JSON: %v\n%s", err, expanded)
	}
	if _, ok := embedPayload["contentRef"]; ok {
		t.Fatalf("contentRef was not removed: %s", expanded)
	}
	if got := embedPayload["input"].(string); got != "Attention Is All You Need\n" {
		t.Fatalf("input = %q", got)
	}
}
