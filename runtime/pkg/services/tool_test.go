package services

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"sync/atomic"
	"testing"

	embeddingv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/embedding/v1"
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
