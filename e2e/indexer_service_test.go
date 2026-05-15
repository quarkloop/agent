//go:build e2e

package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/quarkloop/e2e/utils"
	indexerv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/indexer/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
)

func TestIndexerServiceWithRealDgraph(t *testing.T) {
	bins := utils.BuildAllOnce(t)
	dgraphAddr := utils.StartDgraph(t)
	indexerAddr := startIndexerService(t, bins.Indexer, dgraphAddr)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	conn, err := servicekit.Dial(ctx, indexerAddr)
	if err != nil {
		t.Fatalf("dial indexer: %v", err)
	}
	defer conn.Close()

	client := indexerv1.NewIndexerServiceClient(conn)
	if _, err := client.IndexDocument(ctx, &indexerv1.IndexRequest{
		ChunkId:     "e2e-chunk-1",
		TextContent: "Quark service extraction uses gRPC and Dgraph vector indexes.",
		Embedding:   []float32{1, 0, 0},
		Entities: []*indexerv1.Entity{
			{Id: "quark", Name: "Quark", Type: "PROJECT"},
			{Id: "dgraph", Name: "Dgraph", Type: "DATABASE"},
		},
		Relations: []*indexerv1.Relation{
			{FromId: "quark", ToId: "dgraph", Relation: "USES"},
		},
		SourceMetadata: map[string]string{"source": "e2e", "tenant": "quark"},
	}); err != nil {
		t.Fatalf("index document: %v", err)
	}
	resp, err := client.GetContext(ctx, &indexerv1.QueryRequest{
		QueryVector: []float32{1, 0, 0},
		Limit:       5,
		Depth:       2,
		Filters:     map[string]string{"tenant": "quark"},
	})
	if err != nil {
		t.Fatalf("get context: %v", err)
	}
	if len(resp.GetChunks()) == 0 || resp.GetChunks()[0].GetId() != "e2e-chunk-1" {
		t.Fatalf("unexpected chunks: %+v", resp.GetChunks())
	}
	if !strings.Contains(resp.GetReasoningContext(), "Quark service extraction") {
		t.Fatalf("context missing indexed text: %q", resp.GetReasoningContext())
	}
	if !strings.Contains(resp.GetReasoningContext(), "USES") {
		t.Fatalf("context missing graph relation: %q", resp.GetReasoningContext())
	}
}
