//go:build e2e

package e2e

import (
	"context"
	"testing"

	embeddingv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/embedding/v1"
	indexerv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/indexer/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
)

func verifyPersistedPDFIndexState(t *testing.T, ctx context.Context, artifactDir, indexerAddr, embeddingAddr string, documents []indexedPDFDocument) {
	t.Helper()
	embeddingClient, indexerClient, closeClients := openIndexVerificationClients(t, ctx, embeddingAddr, indexerAddr, "persisted-state")
	defer closeClients()

	report := make([]map[string]any, 0, len(documents))
	for _, document := range documents {
		embedding, err := embeddingClient.Embed(ctx, &embeddingv1.EmbedRequest{
			Input: document.Name + " " + document.Filename,
		})
		if err != nil {
			t.Fatalf("embed verification query for %s: %v", document.Filename, err)
		}
		resp, err := indexerClient.GetContext(ctx, &indexerv1.QueryRequest{
			QueryVector: embedding.GetVector(),
			Limit:       1,
			Depth:       1,
			Filters:     map[string]string{"filename": document.Filename},
		})
		if err != nil {
			t.Fatalf("query persisted index state for %s: %v", document.Filename, err)
		}
		if len(resp.GetChunks()) == 0 {
			t.Fatalf("persisted index state missing chunk for %s", document.Filename)
		}
		chunk := resp.GetChunks()[0]
		if got := chunk.GetMetadata()["filename"]; got != document.Filename {
			t.Fatalf("persisted chunk filename = %q, want %q: %+v", got, document.Filename, chunk.GetMetadata())
		}
		if chunk.GetEmbeddingMetadata().GetDimensions() != embedding.GetDimensions() {
			t.Fatalf("persisted embedding dimensions for %s = %d, want %d", document.Filename, chunk.GetEmbeddingMetadata().GetDimensions(), embedding.GetDimensions())
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

func verifyPersistedMarkdownIndexState(t *testing.T, ctx context.Context, artifactDir, indexerAddr, embeddingAddr string, documents []indexedMarkdownDocument) {
	t.Helper()
	embeddingClient, indexerClient, closeClients := openIndexVerificationClients(t, ctx, embeddingAddr, indexerAddr, "markdown verification")
	defer closeClients()

	report := make([]map[string]any, 0, len(documents))
	for _, document := range documents {
		embedding, err := embeddingClient.Embed(ctx, &embeddingv1.EmbedRequest{
			Input: document.Query,
		})
		if err != nil {
			t.Fatalf("embed markdown verification query for %s: %v", document.Filename, err)
		}
		resp, err := indexerClient.GetContext(ctx, &indexerv1.QueryRequest{
			QueryVector: embedding.GetVector(),
			Limit:       1,
			Depth:       1,
			Filters:     map[string]string{"filename": document.Filename},
		})
		if err != nil {
			t.Fatalf("query persisted markdown index for %s: %v", document.Filename, err)
		}
		if len(resp.GetChunks()) == 0 {
			t.Fatalf("persisted markdown index state missing chunk for %s", document.Filename)
		}
		chunk := resp.GetChunks()[0]
		if got := chunk.GetMetadata()["filename"]; got != document.Filename {
			t.Fatalf("persisted markdown chunk filename = %q, want %q: %+v", got, document.Filename, chunk.GetMetadata())
		}
		if chunk.GetEmbeddingMetadata().GetDimensions() != embedding.GetDimensions() {
			t.Fatalf("persisted markdown embedding dimensions for %s = %d, want %d", document.Filename, chunk.GetEmbeddingMetadata().GetDimensions(), embedding.GetDimensions())
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

func openIndexVerificationClients(t *testing.T, ctx context.Context, embeddingAddr, indexerAddr, label string) (embeddingv1.EmbeddingServiceClient, indexerv1.IndexerServiceClient, func()) {
	t.Helper()
	embeddingConn, err := servicekit.Dial(ctx, embeddingAddr)
	if err != nil {
		t.Fatalf("dial embedding service for %s: %v", label, err)
	}
	indexerConn, err := servicekit.Dial(ctx, indexerAddr)
	if err != nil {
		embeddingConn.Close()
		t.Fatalf("dial indexer service for %s: %v", label, err)
	}
	closeClients := func() {
		indexerConn.Close()
		embeddingConn.Close()
	}
	return embeddingv1.NewEmbeddingServiceClient(embeddingConn), indexerv1.NewIndexerServiceClient(indexerConn), closeClients
}
