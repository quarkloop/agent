package services

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quarkloop/pkg/boundary"
	citationv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/citation/v1"
	documentv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/document/v1"
	embeddingv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/embedding/v1"
	indexerv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/indexer/v1"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/runtime/pkg/modelservice"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

func TestServiceRequestUnmarshalDiscardsUnknownGeneratedFields(t *testing.T) {
	var req citationv1.VerifyGroundingRequest
	raw := `{
		"claims": [{
			"id": "claim-1",
			"claim": "The invoice total is EUR 18,450.00.",
			"citations": [{
				"id": "cite-1",
				"chunkId": "chunk-invoice",
				"sourceUri": "invoice.md",
				"textSpan": "Total due: EUR 18,450.00",
				"startOffset": 10,
				"endOffset": 35,
				"confidence": 0.92
			}]
		}]
	}`

	if err := serviceRequestUnmarshalOptions().Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("service request unmarshal rejected harmless extra field: %v", err)
	}
	citations := req.GetClaims()[0].GetCitations()
	if len(citations) != 1 {
		t.Fatalf("citations = %+v", citations)
	}
	if got := citations[0].GetTextSpan(); got != "Total due: EUR 18,450.00" {
		t.Fatalf("textSpan = %q", got)
	}
}

func TestInjectRuntimeContextArgumentsAddsSpaceForSpaceScopedRequests(t *testing.T) {
	ctx := modelservice.WithSpaceID(context.Background(), "space-1")

	normalized, err := injectRuntimeContextArguments(ctx, "quark.ingestion.v1.StartRunRequest", `{"title":"Import"}`)
	if err != nil {
		t.Fatalf("inject runtime context: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(normalized), &payload); err != nil {
		t.Fatalf("runtime context payload is not JSON: %v\n%s", err, normalized)
	}
	if payload["space"] != "space-1" || payload["title"] != "Import" {
		t.Fatalf("payload = %+v", payload)
	}
}

func TestInjectRuntimeContextArgumentsDoesNotOverrideExplicitSpace(t *testing.T) {
	ctx := modelservice.WithSpaceID(context.Background(), "runtime-space")

	normalized, err := injectRuntimeContextArguments(ctx, "quark.ingestion.v1.StartRunRequest", `{"space":"explicit-space","title":"Import"}`)
	if err != nil {
		t.Fatalf("inject runtime context: %v", err)
	}
	if !strings.Contains(normalized, `"space":"explicit-space"`) {
		t.Fatalf("explicit space was overwritten: %s", normalized)
	}
}

func TestNormalizeDocumentInputArgumentsPromotesTopLevelSourceURI(t *testing.T) {
	normalized, err := normalizeDocumentInputArguments("quark.document.v1.ExtractTextRequest", `{
		"sourceUri": "/tmp/uploaded/source.pdf",
		"mimeType": "application/pdf"
	}`)
	if err != nil {
		t.Fatalf("normalize document input: %v", err)
	}
	var req documentv1.ExtractTextRequest
	if err := protojson.Unmarshal([]byte(normalized), &req); err != nil {
		t.Fatalf("decode normalized request: %v\n%s", err, normalized)
	}
	if got := req.GetInput().GetSourceUri(); got != "/tmp/uploaded/source.pdf" {
		t.Fatalf("sourceUri = %q", got)
	}
	if got := req.GetInput().GetFilename(); got != "source.pdf" {
		t.Fatalf("filename = %q", got)
	}
	if got := req.GetInput().GetMimeType(); got != "application/pdf" {
		t.Fatalf("mimeType = %q", got)
	}
}

func TestNormalizeDocumentInputArgumentsAcceptsStringInput(t *testing.T) {
	normalized, err := normalizeDocumentInputArguments("quark.document.v1.GetPagesRequest", `{
		"input": "/tmp/uploaded/report.pdf"
	}`)
	if err != nil {
		t.Fatalf("normalize document input: %v", err)
	}
	var req documentv1.GetPagesRequest
	if err := protojson.Unmarshal([]byte(normalized), &req); err != nil {
		t.Fatalf("decode normalized request: %v\n%s", err, normalized)
	}
	if got := req.GetInput().GetSourceUri(); got != "/tmp/uploaded/report.pdf" {
		t.Fatalf("sourceUri = %q", got)
	}
	if got := req.GetInput().GetFilename(); got != "report.pdf" {
		t.Fatalf("filename = %q", got)
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

func TestExecutorExpandsRuntimeRefsForCanonicalUpsertChunkRequests(t *testing.T) {
	executor := NewExecutor(nil)

	contentResult, err := executor.CaptureToolResult("fs", `{"command":"read","path":"/tmp/source.md"}`, `{"content":"Canonical text\n"}`)
	if err != nil {
		t.Fatalf("capture content result: %v", err)
	}
	var contentPayload map[string]any
	if err := json.Unmarshal([]byte(contentResult), &contentPayload); err != nil {
		t.Fatalf("decode content result: %v\n%s", err, contentResult)
	}

	embeddingResult, err := executor.embeddingToolResult(&embeddingv1.EmbedResponse{
		Vector:      []float32{0.5, 0.25},
		Model:       "local-hash-v1",
		Dimensions:  2,
		Provider:    "local",
		ContentHash: "sha256:source",
	})
	if err != nil {
		t.Fatalf("capture embedding result: %v", err)
	}
	var embeddingPayload map[string]any
	if err := json.Unmarshal([]byte(embeddingResult), &embeddingPayload); err != nil {
		t.Fatalf("decode embedding result: %v\n%s", err, embeddingResult)
	}

	expanded, err := executor.expandRuntimeReferences("quark.indexer.v1.UpsertChunkRequest", `{"chunkId":"chunk-1","textContentRef":"`+contentPayload["contentRef"].(string)+`","embeddingRef":"`+embeddingPayload["embeddingRef"].(string)+`"}`)
	if err != nil {
		t.Fatalf("expand refs: %v", err)
	}
	var req indexerv1.UpsertChunkRequest
	if err := protojson.Unmarshal([]byte(expanded), &req); err != nil {
		t.Fatalf("expanded payload is not canonical UpsertChunkRequest: %v\n%s", err, expanded)
	}
	if req.GetTextContent() != "Canonical text\n" {
		t.Fatalf("textContent = %q", req.GetTextContent())
	}
	if got := req.GetEmbedding(); len(got) != 2 || got[0] != 0.5 || got[1] != 0.25 {
		t.Fatalf("embedding = %+v", got)
	}
	if req.GetEmbeddingMetadata().GetModel() != "local-hash-v1" {
		t.Fatalf("embedding metadata was not attached: %+v", req.GetEmbeddingMetadata())
	}
}

func TestExecutorNormalizesCanonicalUpsertChunkCitationEvidence(t *testing.T) {
	executor := NewExecutor(nil)
	result, err := executor.documentExtractTextToolResult(&documentv1.ExtractTextResponse{
		Text:       "Evidence line one.\nEvidence line two.",
		SourceHash: "sha256:source",
	}, "")
	if err != nil {
		t.Fatalf("capture document result: %v", err)
	}
	var documentPayload map[string]any
	if err := json.Unmarshal([]byte(result), &documentPayload); err != nil {
		t.Fatalf("decode document result: %v\n%s", err, result)
	}
	arguments := `{
		"chunkId": "chunk-1",
		"textContentRef": "` + documentPayload["contentRef"].(string) + `",
		"embeddingRef": "emb_1",
		"document": {"id": "doc-1", "sourceUri": "/tmp/source.pdf"},
		"sourceMetadata": {"filename": "source.pdf"},
		"provenance": {"sourceUri": "/tmp/source.pdf", "sourceHash": "sha256:source"},
		"facts": [{"subject": "source", "predicate": "contains", "object": "evidence", "citations": []}],
		"entities": [{"id": "doc-1", "name": "source.pdf", "type": "document"}],
		"relations": [],
		"citations": []
	}`

	normalized, err := executor.NormalizeToolCallArguments(context.Background(), "indexer_UpsertChunk", arguments)
	if err != nil {
		t.Fatalf("normalize tool call arguments: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(normalized), &payload); err != nil {
		t.Fatalf("normalized payload is not JSON: %v\n%s", err, normalized)
	}
	citations, ok := payload["citations"].([]any)
	if !ok || len(citations) != 1 {
		t.Fatalf("citations = %+v", payload["citations"])
	}
	citation := citations[0].(map[string]any)
	if got := citation["textSpan"].(string); !strings.Contains(got, "Evidence line one.") {
		t.Fatalf("citation textSpan = %q", got)
	}
	facts := payload["facts"].([]any)
	factCitations := facts[0].(map[string]any)["citations"].([]any)
	if len(facts) != 1 || len(factCitations) != 1 {
		t.Fatalf("fact citation evidence missing: %+v", payload["facts"])
	}
}

func TestExecutorCompletesCanonicalUpsertChunkDefaultsFromRuntimeReferences(t *testing.T) {
	executor := NewExecutor(nil)
	result, err := executor.documentExtractTextToolResult(&documentv1.ExtractTextResponse{
		Text:       "Attention Is All You Need introduced the Transformer architecture.",
		SourceHash: "sha256:paper",
	}, `{"input":{"sourceUri":"/tmp/attention_is_all_you_need_paper.pdf","filename":"attention_is_all_you_need_paper.pdf","mimeType":"application/pdf"}}`)
	if err != nil {
		t.Fatalf("capture document result: %v", err)
	}
	var documentPayload map[string]any
	if err := json.Unmarshal([]byte(result), &documentPayload); err != nil {
		t.Fatalf("decode document result: %v\n%s", err, result)
	}
	embeddingResult, err := executor.embeddingToolResult(&embeddingv1.EmbedResponse{
		Vector:      []float32{0.5, 0.25},
		Model:       "local-hash-v1",
		Dimensions:  2,
		Provider:    "local",
		ContentHash: "sha256:paper",
	})
	if err != nil {
		t.Fatalf("capture embedding result: %v", err)
	}
	var embeddingPayload map[string]any
	if err := json.Unmarshal([]byte(embeddingResult), &embeddingPayload); err != nil {
		t.Fatalf("decode embedding result: %v\n%s", err, embeddingResult)
	}

	normalized, err := executor.NormalizeToolCallArguments(context.Background(), "indexer_UpsertChunk", `{
		"textContentRef": "`+documentPayload["contentRef"].(string)+`",
		"embeddingRef": "`+embeddingPayload["embeddingRef"].(string)+`"
	}`)
	if err != nil {
		t.Fatalf("normalize tool call arguments: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(normalized), &payload); err != nil {
		t.Fatalf("normalized payload is not JSON: %v\n%s", err, normalized)
	}
	for _, key := range []string{"chunkId", "document", "sourceMetadata", "provenance", "embeddingMetadata", "facts", "entities", "relations", "citations"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("normalized payload missing %s: %+v", key, payload)
		}
	}
	document := payload["document"].(map[string]any)
	if got := document["sourceUri"].(string); got != "/tmp/attention_is_all_you_need_paper.pdf" {
		t.Fatalf("document sourceUri = %q", got)
	}
	sourceMetadata := payload["sourceMetadata"].(map[string]any)
	if got := sourceMetadata["filename"].(string); got != "attention_is_all_you_need_paper.pdf" {
		t.Fatalf("source metadata filename = %q", got)
	}
	facts := payload["facts"].([]any)
	if len(facts) != 1 {
		t.Fatalf("facts = %+v", payload["facts"])
	}
	fact := facts[0].(map[string]any)
	if got := fact["object"].(string); !strings.Contains(got, "Transformer architecture") {
		t.Fatalf("fallback fact object = %q", got)
	}
}

func TestExecutorExpandsDocumentContentRefsForEmbeddingRequests(t *testing.T) {
	executor := NewExecutor(nil)

	result, err := executor.documentExtractTextToolResult(&documentv1.ExtractTextResponse{
		Text:       "Attention Is All You Need\n",
		SourceHash: "sha256:paper",
	}, "")
	if err != nil {
		t.Fatalf("capture document result: %v", err)
	}
	var toolPayload map[string]any
	if err := json.Unmarshal([]byte(result), &toolPayload); err != nil {
		t.Fatalf("document result is not JSON: %v\n%s", err, result)
	}
	ref, ok := toolPayload["contentRef"].(string)
	if !ok || ref == "" {
		t.Fatalf("contentRef missing from document result: %+v", toolPayload)
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

func TestExecutorStripsEmbeddingProviderOverrides(t *testing.T) {
	executor := NewExecutor(nil)

	expanded, err := executor.expandRuntimeReferences("quark.embedding.v1.EmbedRequest", `{"input":"hello","model":"wrong-model","dimensions":384}`)
	if err != nil {
		t.Fatalf("expand refs: %v", err)
	}
	var embedPayload map[string]any
	if err := json.Unmarshal([]byte(expanded), &embedPayload); err != nil {
		t.Fatalf("expanded payload is not JSON: %v\n%s", err, expanded)
	}
	if _, ok := embedPayload["model"]; ok {
		t.Fatalf("model override was not removed: %s", expanded)
	}
	if _, ok := embedPayload["dimensions"]; ok {
		t.Fatalf("dimensions override was not removed: %s", expanded)
	}
	if got := embedPayload["input"].(string); got != "hello" {
		t.Fatalf("input = %q", got)
	}
}

func TestExecutorPromotesContentReferencePassedThroughStringFields(t *testing.T) {
	executor := NewExecutor(nil)

	result, err := executor.documentExtractTextToolResult(&documentv1.ExtractTextResponse{
		Text: "Canonical source text\n",
	}, "")
	if err != nil {
		t.Fatalf("capture document result: %v", err)
	}
	var toolPayload map[string]any
	if err := json.Unmarshal([]byte(result), &toolPayload); err != nil {
		t.Fatalf("document result is not JSON: %v\n%s", err, result)
	}
	ref := toolPayload["contentRef"].(string)

	expanded, err := executor.expandRuntimeReferences("quark.embedding.v1.EmbedRequest", `{"input":["`+ref+`"]}`)
	if err != nil {
		t.Fatalf("expand refs: %v", err)
	}
	var embedPayload map[string]any
	if err := json.Unmarshal([]byte(expanded), &embedPayload); err != nil {
		t.Fatalf("expanded payload is not JSON: %v\n%s", err, expanded)
	}
	if got := embedPayload["input"].(string); got != "Canonical source text\n" {
		t.Fatalf("input = %q", got)
	}
}

func TestNormalizeStringArgumentsAcceptsStructuredTextShapes(t *testing.T) {
	normalized, err := normalizeStringMapArguments("quark.embedding.v1.EmbedRequest", `{"input":[{"type":"text","text":"first"},{"content":[{"type":"text","text":"second"}]}]}`)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	var req embeddingv1.EmbedRequest
	if err := protojson.Unmarshal([]byte(normalized), &req); err != nil {
		t.Fatalf("protojson accepts normalized payload: %v\n%s", err, normalized)
	}
	if req.GetInput() != "first\n\nsecond" {
		t.Fatalf("input = %q", req.GetInput())
	}
}

func TestCanonicalIndexerRequestSchemasExposeRuntimeReferenceFields(t *testing.T) {
	embedSchema := requestParameters("quark.embedding.v1.EmbedRequest")
	embedProperties := embedSchema["properties"].(map[string]any)
	if _, ok := embedProperties["inputRef"]; !ok {
		t.Fatalf("EmbedRequest schema missing inputRef: %+v", embedSchema)
	}
	if _, ok := embedProperties["model"]; ok {
		t.Fatalf("EmbedRequest schema exposed model override: %+v", embedSchema)
	}
	if _, ok := embedProperties["dimensions"]; ok {
		t.Fatalf("EmbedRequest schema exposed dimensions override: %+v", embedSchema)
	}

	upsertSchema := requestParameters("quark.indexer.v1.UpsertChunkRequest")
	properties := upsertSchema["properties"].(map[string]any)
	if _, ok := properties["embeddingRef"]; !ok {
		t.Fatalf("UpsertChunk schema missing embeddingRef: %+v", upsertSchema)
	}
	if _, ok := properties["embedding"]; ok {
		t.Fatalf("UpsertChunk schema exposed direct embedding vectors: %+v", upsertSchema)
	}
	if _, ok := properties["textContentRef"]; !ok {
		t.Fatalf("UpsertChunk schema missing textContentRef: %+v", upsertSchema)
	}
	if got := upsertSchema["required"]; !sameStrings(got, []string{
		"chunkId",
		"embeddingRef",
		"document",
		"sourceMetadata",
		"provenance",
		"facts",
		"entities",
		"relations",
		"citations",
	}) {
		t.Fatalf("UpsertChunk required = %+v", got)
	}
	if got := properties["facts"].(map[string]any)["minItems"]; got != 1 {
		t.Fatalf("UpsertChunk facts should require at least one evidence-backed fact: %+v", properties["facts"])
	}
	if got := properties["sourceMetadata"].(map[string]any)["minProperties"]; got != 1 {
		t.Fatalf("UpsertChunk sourceMetadata should require source context: %+v", properties["sourceMetadata"])
	}

	querySchema := requestParameters("quark.indexer.v1.QueryRequest")
	queryProperties := querySchema["properties"].(map[string]any)
	if _, ok := queryProperties["queryVectorRef"]; !ok {
		t.Fatalf("QueryRequest schema missing queryVectorRef: %+v", querySchema)
	}
	if _, ok := queryProperties["queryVector"]; ok {
		t.Fatalf("QueryRequest schema exposed direct query vectors: %+v", querySchema)
	}
	if got := querySchema["required"]; !sameStrings(got, []string{"queryVectorRef"}) {
		t.Fatalf("QueryRequest required = %+v", got)
	}

	deleteSchema := requestParameters("quark.indexer.v1.DeleteDocumentRequest")
	if got := deleteSchema["required"]; !sameStrings(got, []string{"documentId"}) {
		t.Fatalf("DeleteDocument required = %+v", got)
	}
}

func TestRuntimeIndexRequestsRejectDirectEmbeddingVectors(t *testing.T) {
	err := requireRuntimeReferenceArguments("quark.indexer.v1.UpsertChunkRequest", `{"chunkId":"chunk-1","embedding":[0.1,0.2]}`)
	if err == nil || !strings.Contains(err.Error(), "embeddingRef") {
		t.Fatalf("expected embeddingRef validation error, got %v", err)
	}
	if err := requireRuntimeReferenceArguments("quark.indexer.v1.UpsertChunkRequest", `{"chunkId":"chunk-1","embeddingRef":"emb_1"}`); err != nil {
		t.Fatalf("embeddingRef should satisfy runtime validation: %v", err)
	}
	err = requireRuntimeReferenceArguments("quark.indexer.v1.QueryRequest", `{"queryVector":[0.1,0.2]}`)
	if err == nil || !strings.Contains(err.Error(), "queryVectorRef") {
		t.Fatalf("expected queryVectorRef validation error, got %v", err)
	}
}

func TestExecutorAddsExpiringReferencesForLargeServiceResults(t *testing.T) {
	executor := NewExecutor(nil)
	result := `{"contexts":[{"text":"` + strings.Repeat("retrieved context ", 200) + `"}]}`

	withRef, err := executor.attachResultReference("indexer_QueryContext", "quark.indexer.v1.ContextResponse", []byte(result))
	if err != nil {
		t.Fatalf("attach result reference: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(withRef), &payload); err != nil {
		t.Fatalf("decode referenced result: %v\n%s", err, withRef)
	}
	ref, ok := payload["resultRef"].(string)
	if !ok || ref == "" {
		t.Fatalf("resultRef missing: %+v", payload)
	}
	if content, ok := executor.contentByRef(ref); !ok || content != result {
		t.Fatalf("stored result ref = %q ok=%t", content, ok)
	}

	executor.setReferenceTTL(time.Millisecond)
	executor.CleanupExpiredReferences(time.Now().Add(time.Hour))
	if _, ok := executor.contentByRef(ref); ok {
		t.Fatalf("expired result reference %s was not cleaned up", ref)
	}
}

func TestExecutorCompactsReferencedIndexerContextForLLM(t *testing.T) {
	executor := NewExecutor(nil)
	longText := strings.Repeat("retrieved context evidence ", 200)
	result := `{
		"reasoningContext": "` + longText + `",
		"chunks": [{
			"id": "chunk-1",
			"text": "` + longText + `",
			"document": {"sourceUri": "/tmp/source.pdf"},
			"citations": [{"sourceUri": "/tmp/source.pdf", "textSpan": "evidence"}]
		}],
		"contextPackage": {
			"chunks": [{
				"id": "chunk-1",
				"text": "` + longText + `",
				"document": {"sourceUri": "/tmp/source.pdf"}
			}]
		}
	}`

	withRef, err := executor.attachResultReference("indexer_QueryContext", "quark.indexer.v1.ContextResponse", []byte(result))
	if err != nil {
		t.Fatalf("attach result reference: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(withRef), &payload); err != nil {
		t.Fatalf("decode referenced result: %v\n%s", err, withRef)
	}
	if payload["resultCompacted"] != true {
		t.Fatalf("resultCompacted missing: %+v", payload)
	}
	ref, ok := payload["resultRef"].(string)
	if !ok || ref == "" {
		t.Fatalf("resultRef missing: %+v", payload)
	}
	stored, ok := executor.contentByRef(ref)
	if !ok || stored != result {
		t.Fatalf("stored full result mismatch ok=%t", ok)
	}
	chunks := payload["chunks"].([]any)
	chunk := chunks[0].(map[string]any)
	if chunk["textTruncated"] != true {
		t.Fatalf("chunk text was not marked truncated: %+v", chunk)
	}
	if got := chunk["text"].(string); len([]rune(got)) >= len([]rune(longText)) {
		t.Fatalf("chunk text was not compacted")
	}
}

func TestExecutorCompactsLargeDocumentExtractionTextForLLM(t *testing.T) {
	executor := NewExecutor(nil)
	longText := strings.Repeat("source evidence paragraph ", 400)

	result, err := executor.documentExtractTextToolResult(&documentv1.ExtractTextResponse{
		Text:       longText,
		SourceHash: "sha256:source",
	}, `{"input":{"sourceUri":"/tmp/source.pdf","filename":"source.pdf"}}`)
	if err != nil {
		t.Fatalf("capture document result: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result), &payload); err != nil {
		t.Fatalf("decode document result: %v\n%s", err, result)
	}
	ref, ok := payload["contentRef"].(string)
	if !ok || ref == "" {
		t.Fatalf("contentRef missing: %+v", payload)
	}
	stored, ok := executor.contentByRef(ref)
	if !ok || stored != longText {
		t.Fatalf("stored full document text mismatch ok=%t", ok)
	}
	if payload["textTruncated"] != true {
		t.Fatalf("textTruncated missing: %+v", payload)
	}
	if got := payload["text"].(string); len([]rune(got)) >= len([]rune(longText)) {
		t.Fatalf("document text was not compacted")
	}
}

func TestExecutorDoesNotCaptureFilesystemPDFExtractionAsContentSource(t *testing.T) {
	executor := NewExecutor(nil)
	result, err := executor.CaptureToolResult("fs", `{"command":"extract_pdf","path":"/tmp/paper.pdf"}`, `{"content":"Attention Is All You Need\n"}`)
	if err != nil {
		t.Fatalf("capture tool result: %v", err)
	}
	if strings.Contains(result, "contentRef") {
		t.Fatalf("fs extract_pdf should not produce runtime content refs after document service migration: %s", result)
	}
}

func TestExecutorRetriesRetryableServiceFunctionFailures(t *testing.T) {
	server := grpc.NewServer()
	fake := &flakyEmbeddingServer{}
	embeddingv1.RegisterEmbeddingServiceServer(server, fake)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- server.Serve(ln) }()
	defer func() {
		server.Stop()
		_ = ln.Close()
		<-errCh
	}()

	executor := NewExecutor([]*servicev1.ServiceDescriptor{{
		Name:    "embedding",
		Address: ln.Addr().String(),
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:       embeddingv1.EmbeddingService_ServiceDesc.ServiceName,
			Method:        "Embed",
			Request:       "quark.embedding.v1.EmbedRequest",
			Response:      "quark.embedding.v1.EmbedResponse",
			Description:   "Embed text.",
			FunctionName:  "embedding_Embed",
			TimeoutMillis: 5000,
			RetryPolicy: &servicev1.RetryPolicy{
				MaxAttempts:    2,
				RetryableCodes: []string{"Unavailable"},
			},
		}},
	}})

	result, err := executor.Execute(context.Background(), "embedding_Embed", `{"input":"hello"}`)
	if err != nil {
		t.Fatalf("execute retryable service function: %v", err)
	}
	if calls := atomic.LoadInt32(&fake.calls); calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if !strings.Contains(result, "embeddingRef") {
		t.Fatalf("expected embedding reference result, got %s", result)
	}
}

func TestExecutorWrapsMissingServiceFunctionAsDiagnosticNotFound(t *testing.T) {
	executor := NewExecutor(nil)

	_, err := executor.Execute(context.Background(), "missing_Service", `{}`)
	if !boundary.IsCategory(err, boundary.NotFound) {
		t.Fatalf("expected not found boundary error, got %v", err)
	}
	diag := boundary.DiagnosticFromError(err, boundary.Service, "service function")
	if diag.Code != "service.not_found" || diag.Hint == "" {
		t.Fatalf("diagnostic = %+v", diag)
	}
}

func TestExecutorMapsServiceInvalidArgumentToDiagnostics(t *testing.T) {
	server := grpc.NewServer()
	embeddingv1.RegisterEmbeddingServiceServer(server, invalidArgumentEmbeddingServer{})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	errCh := make(chan error, 1)
	go func() { errCh <- server.Serve(ln) }()
	defer func() {
		server.Stop()
		_ = ln.Close()
		<-errCh
	}()

	executor := NewExecutor([]*servicev1.ServiceDescriptor{{
		Name:    "embedding",
		Address: ln.Addr().String(),
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:      embeddingv1.EmbeddingService_ServiceDesc.ServiceName,
			Method:       "Embed",
			Request:      "quark.embedding.v1.EmbedRequest",
			Response:     "quark.embedding.v1.EmbedResponse",
			FunctionName: "embedding_Embed",
		}},
	}})

	_, err = executor.Execute(context.Background(), "embedding_Embed", `{"input":"bad"}`)
	if !boundary.IsCategory(err, boundary.InvalidArgument) {
		t.Fatalf("expected invalid argument boundary error, got %v", err)
	}
}

type flakyEmbeddingServer struct {
	embeddingv1.UnimplementedEmbeddingServiceServer
	calls int32
}

type invalidArgumentEmbeddingServer struct {
	embeddingv1.UnimplementedEmbeddingServiceServer
}

func (invalidArgumentEmbeddingServer) Embed(context.Context, *embeddingv1.EmbedRequest) (*embeddingv1.EmbedResponse, error) {
	return nil, status.Error(codes.InvalidArgument, "parser rejected input")
}

func (s *flakyEmbeddingServer) Embed(context.Context, *embeddingv1.EmbedRequest) (*embeddingv1.EmbedResponse, error) {
	if atomic.AddInt32(&s.calls, 1) == 1 {
		return nil, status.Error(codes.Unavailable, "try again")
	}
	return &embeddingv1.EmbedResponse{
		Vector:      []float32{0.1, 0.2},
		Model:       "test",
		Dimensions:  2,
		Provider:    "test",
		ContentHash: "abc123",
	}, nil
}

func sameStrings(raw any, want []string) bool {
	got, ok := raw.([]string)
	if !ok || len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
