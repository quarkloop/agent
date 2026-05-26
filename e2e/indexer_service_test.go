//go:build e2e

package e2e

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/quarkloop/e2e/utils"
	"github.com/quarkloop/pkg/natskit"
	indexerv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/indexer/v1"
	"github.com/quarkloop/supervisor/pkg/natshub"
)

func TestIndexerServiceWithRealDgraph(t *testing.T) {
	embeddingModel := strings.TrimSpace(os.Getenv("OPENROUTER_E2E_EMBEDDING_MODEL"))
	if embeddingModel == "" {
		t.Fatal("OPENROUTER_E2E_EMBEDDING_MODEL is required for real Gateway embedding E2E execution")
	}
	env := utils.StartE2E(t, true, utils.StartOptions{
		DisableKnowledgeServices: true,
		Embedding:                utils.GatewayEmbeddingOptions{Provider: "openrouter", Model: embeddingModel},
		Services:                 append(localServicePlugins("indexer"), gatewayServicePlugin()),
	})

	conn, err := natskit.Connect(context.Background(), natskit.Config{
		URL: env.NATS.ClientURL, Username: natshub.DefaultControlUser,
		Password: natshub.DefaultControlPassword, Name: "quark-e2e-indexer-service", Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("connect control nats: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	text := "Quark service extraction uses NATS service functions and Dgraph vector indexes."
	vector, model := requestRealEmbedding(t, ctx, conn, env, text)
	var indexResp indexerv1.IndexStatus
	requestServiceFunction(t, ctx, conn, env.Space, "indexer", "upsert_chunk", &indexerv1.UpsertChunkRequest{
		ChunkId:     "e2e-chunk-1",
		TextContent: text,
		Embedding:   vector,
		EmbeddingMetadata: &indexerv1.EmbeddingMetadata{
			Provider:   "openrouter",
			Model:      model,
			Dimensions: int32(len(vector)),
			Modalities: []string{"text"},
		},
		Entities: []*indexerv1.Entity{
			{Id: "quark", Name: "Quark", Type: "PROJECT"},
			{Id: "dgraph", Name: "Dgraph", Type: "DATABASE"},
		},
		Relations: []*indexerv1.Relation{
			{FromId: "quark", ToId: "dgraph", Relation: "USES"},
		},
		SourceMetadata: map[string]string{"source": "e2e", "tenant": "quark"},
	}, &indexResp)
	if !indexResp.GetSuccess() {
		t.Fatalf("index response failed: %+v", &indexResp)
	}

	var contextResp indexerv1.ContextResponse
	queryVector, _ := requestRealEmbedding(t, ctx, conn, env, "Which database is used for Quark service extraction?")
	queryCall := requestServiceFunction(t, ctx, conn, env.Space, "indexer", "query_context", &indexerv1.QueryRequest{
		QueryVector: queryVector,
		Limit:       5,
		Depth:       2,
		Filters:     map[string]string{"tenant": "quark"},
	}, &contextResp)
	if len(contextResp.GetChunks()) == 0 || contextResp.GetChunks()[0].GetId() != "e2e-chunk-1" {
		t.Fatalf("unexpected chunks: %+v", contextResp.GetChunks())
	}
	if !strings.Contains(contextResp.GetReasoningContext(), "Quark service extraction") {
		t.Fatalf("context missing indexed text: %q", contextResp.GetReasoningContext())
	}
	if !strings.Contains(contextResp.GetReasoningContext(), "USES") {
		t.Fatalf("context missing graph relation: %q", contextResp.GetReasoningContext())
	}
	audit := utils.GetAuditRecord(t, env, queryCall.ReferenceID)
	if audit.ReferenceID != queryCall.ReferenceID || audit.Service != "indexer" || audit.Function != "query_context" || audit.Status != "ok" {
		t.Fatalf("query audit record = %+v", audit)
	}
	utils.CaptureGatewayUsage(t, env)
}
