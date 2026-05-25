//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/quarkloop/e2e/utils"
	"github.com/quarkloop/pkg/natskit"
	gatewayv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/gateway/v1"
	indexerv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/indexer/v1"
	"github.com/quarkloop/supervisor/pkg/natshub"
)

func verifyPersistedPDFIndexState(t *testing.T, ctx context.Context, artifactDir string, env *utils.E2EEnv, documents []indexedPDFDocument) {
	t.Helper()
	conn := openIndexVerificationConn(t, env)
	defer conn.Close()

	report := make([]map[string]any, 0, len(documents))
	for _, document := range documents {
		var embedding gatewayv1.EmbedResponse
		requestServiceFunction(t, ctx, conn, env.Space, "gateway", "embed", &gatewayv1.EmbedRequest{
			Inputs: gatewayTextInputs(document.Name + " " + document.Filename),
		}, &embedding)
		if len(embedding.GetEmbeddings()) != 1 {
			t.Fatalf("gateway embedding result count for %s = %d, want 1", document.Filename, len(embedding.GetEmbeddings()))
		}
		vector := embedding.GetEmbeddings()[0]
		var resp indexerv1.ContextResponse
		requestServiceFunction(t, ctx, conn, env.Space, "indexer", "get_context", &indexerv1.QueryRequest{
			QueryVector: vector.GetVector(),
			Limit:       1,
			Depth:       1,
			Filters:     map[string]string{"filename": document.Filename},
		}, &resp)
		if len(resp.GetChunks()) == 0 {
			t.Fatalf("persisted index state missing chunk for %s", document.Filename)
		}
		chunk := resp.GetChunks()[0]
		if got := chunk.GetMetadata()["filename"]; got != document.Filename {
			t.Fatalf("persisted chunk filename = %q, want %q: %+v", got, document.Filename, chunk.GetMetadata())
		}
		if chunk.GetEmbeddingMetadata().GetDimensions() != vector.GetDimensions() {
			t.Fatalf("persisted embedding dimensions for %s = %d, want %d", document.Filename, chunk.GetEmbeddingMetadata().GetDimensions(), vector.GetDimensions())
		}
		report = append(report, map[string]any{
			"filename":           document.Filename,
			"chunk_id":           chunk.GetId(),
			"score":              chunk.GetScore(),
			"embedding_provider": chunk.GetEmbeddingMetadata().GetProvider(),
			"embedding_model":    chunk.GetEmbeddingMetadata().GetModel(),
			"embedding_dims":     chunk.GetEmbeddingMetadata().GetDimensions(),
			"citations":          resp.GetCitations(),
			"context_confidence": resp.GetContextPackage().GetConfidence(),
		})
	}
	writeJSONArtifact(t, artifactDir, "direct-index-state.json", report)
}

func verifyPersistedMarkdownIndexState(t *testing.T, ctx context.Context, artifactDir string, env *utils.E2EEnv, documents []indexedMarkdownDocument) {
	t.Helper()
	conn := openIndexVerificationConn(t, env)
	defer conn.Close()

	report := make([]map[string]any, 0, len(documents))
	for _, document := range documents {
		var embedding gatewayv1.EmbedResponse
		requestServiceFunction(t, ctx, conn, env.Space, "gateway", "embed", &gatewayv1.EmbedRequest{
			Inputs: gatewayTextInputs(document.Query),
		}, &embedding)
		if len(embedding.GetEmbeddings()) != 1 {
			t.Fatalf("gateway embedding result count for %s = %d, want 1", document.Filename, len(embedding.GetEmbeddings()))
		}
		vector := embedding.GetEmbeddings()[0]
		var resp indexerv1.ContextResponse
		requestServiceFunction(t, ctx, conn, env.Space, "indexer", "get_context", &indexerv1.QueryRequest{
			QueryVector: vector.GetVector(),
			Limit:       1,
			Depth:       1,
			Filters:     map[string]string{"filename": document.Filename},
		}, &resp)
		if len(resp.GetChunks()) == 0 {
			t.Fatalf("persisted markdown index state missing chunk for %s", document.Filename)
		}
		chunk := resp.GetChunks()[0]
		if got := chunk.GetMetadata()["filename"]; got != document.Filename {
			t.Fatalf("persisted markdown chunk filename = %q, want %q: %+v", got, document.Filename, chunk.GetMetadata())
		}
		if chunk.GetEmbeddingMetadata().GetDimensions() != vector.GetDimensions() {
			t.Fatalf("persisted markdown embedding dimensions for %s = %d, want %d", document.Filename, chunk.GetEmbeddingMetadata().GetDimensions(), vector.GetDimensions())
		}
		for _, want := range document.Want {
			if !containsText(chunk.GetText(), want) && !containsText(resp.GetReasoningContext(), want) {
				t.Fatalf("persisted markdown context for %s missing %q:\nchunk=%s\ncontext=%s", document.Filename, want, chunk.GetText(), resp.GetReasoningContext())
			}
		}
		report = append(report, map[string]any{
			"filename":           document.Filename,
			"chunk_id":           chunk.GetId(),
			"score":              chunk.GetScore(),
			"embedding_provider": chunk.GetEmbeddingMetadata().GetProvider(),
			"embedding_model":    chunk.GetEmbeddingMetadata().GetModel(),
			"embedding_dims":     chunk.GetEmbeddingMetadata().GetDimensions(),
			"metadata":           chunk.GetMetadata(),
			"context_confidence": resp.GetContextPackage().GetConfidence(),
		})
	}
	writeJSONArtifact(t, artifactDir, "markdown-direct-index-state.json", report)
}

func gatewayTextInputs(values ...string) []*gatewayv1.MultimodalInput {
	inputs := make([]*gatewayv1.MultimodalInput, 0, len(values))
	for _, value := range values {
		inputs = append(inputs, &gatewayv1.MultimodalInput{Content: []*gatewayv1.ContentPart{{
			Kind: gatewayv1.ContentKind_CONTENT_KIND_TEXT,
			Text: value,
		}}})
	}
	return inputs
}

func openIndexVerificationConn(t *testing.T, env *utils.E2EEnv) *natskit.Client {
	t.Helper()
	conn, err := natskit.Connect(context.Background(), natskit.Config{
		URL: env.NATS.ClientURL, Username: natshub.DefaultControlUser,
		Password: natshub.DefaultControlPassword, Name: "quark-e2e-index-verification",
	})
	if err != nil {
		t.Fatalf("connect control nats for index verification: %v", err)
	}
	return conn
}
