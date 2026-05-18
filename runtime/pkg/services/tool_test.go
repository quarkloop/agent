package services

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	documentv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/document/v1"
	embeddingv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/embedding/v1"
	indexerv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/indexer/v1"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
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

func TestExecutorExpandsDocumentContentRefsForEmbeddingRequests(t *testing.T) {
	executor := NewExecutor(nil)

	result, err := executor.documentExtractTextToolResult(&documentv1.ExtractTextResponse{
		Text:       "Attention Is All You Need\n",
		SourceHash: "sha256:paper",
	})
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

func TestCanonicalIndexerRequestSchemasExposeRuntimeReferenceFields(t *testing.T) {
	upsertSchema := requestParameters("quark.indexer.v1.UpsertChunkRequest")
	properties := upsertSchema["properties"].(map[string]any)
	if _, ok := properties["embeddingRef"]; !ok {
		t.Fatalf("UpsertChunk schema missing embeddingRef: %+v", upsertSchema)
	}
	if _, ok := properties["textContentRef"]; !ok {
		t.Fatalf("UpsertChunk schema missing textContentRef: %+v", upsertSchema)
	}
	if got := upsertSchema["required"]; !sameStrings(got, []string{"chunkId", "embeddingRef"}) {
		t.Fatalf("UpsertChunk required = %+v", got)
	}

	deleteSchema := requestParameters("quark.indexer.v1.DeleteDocumentRequest")
	if got := deleteSchema["required"]; !sameStrings(got, []string{"documentId"}) {
		t.Fatalf("DeleteDocument required = %+v", got)
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

type flakyEmbeddingServer struct {
	embeddingv1.UnimplementedEmbeddingServiceServer
	calls int32
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
