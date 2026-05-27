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

	vector := requestVerificationEmbedding(t, ctx, conn, env, "verify indexed PDF source records")
	report := make([]map[string]any, 0, len(documents))
	for _, document := range documents {
		var resp indexerv1.ContextResponse
		queryCall := requestServiceFunction(t, ctx, conn, env.Space, "indexer", "query_context", &indexerv1.QueryRequest{
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
		audit := utils.GetAuditRecord(t, env, queryCall.ReferenceID)
		if audit.ReferenceID != queryCall.ReferenceID || audit.Service != "indexer" || audit.Function != "query_context" || audit.Status != "ok" {
			t.Fatalf("query audit record for %s = %+v", document.Filename, audit)
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
	utils.CaptureGatewayUsage(t, env)
}

func requestVerificationEmbedding(t *testing.T, ctx context.Context, conn *natskit.Client, env *utils.E2EEnv, text string) *gatewayv1.Embedding {
	t.Helper()
	var embedding gatewayv1.EmbedResponse
	requestServiceFunction(t, ctx, conn, env.Space, "gateway", "embed", &gatewayv1.EmbedRequest{
		Inputs: gatewayTextInputs(text),
	}, &embedding)
	if len(embedding.GetEmbeddings()) != 1 {
		t.Fatalf("gateway verification embedding result count = %d, want 1", len(embedding.GetEmbeddings()))
	}
	return embedding.GetEmbeddings()[0]
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
