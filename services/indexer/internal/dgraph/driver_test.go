package dgraph

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/quarkloop/services/indexer/internal/indexing"
	"github.com/quarkloop/services/indexer/pkg/indexer"
)

func TestDgraphHelpersBuildVectorAndFilters(t *testing.T) {
	t.Parallel()
	if got := vectorLiteral([]float32{1, 0.5}); got != "[1,0.5]" {
		t.Fatalf("vectorLiteral = %q", got)
	}
	filter := dgraphFilter(map[string]string{"tenant": "acme"})
	if !strings.Contains(filter, "@filter(eq(quark.meta_tenant_") || !strings.Contains(filter, `"acme"`) {
		t.Fatalf("unexpected filter: %q", filter)
	}
	query := vectorSearchQuery(3, nil)
	if !strings.Contains(query, "quark.canonical_json") {
		t.Fatalf("vector search query does not request canonical records: %q", query)
	}
}

func TestRecordUpsertPlanKeepsCanonicalVectorAndGraphTogether(t *testing.T) {
	t.Parallel()

	record := indexer.KnowledgeRecord{
		Chunk: indexer.Chunk{
			ID:       "chunk-1",
			Text:     "Transformer uses attention.",
			Vector:   []float32{0.1, 0.2},
			Metadata: map[string]string{"filename": "paper.pdf"},
		},
		Entities: []indexer.Entity{
			{ID: "transformer", Name: "Transformer", Type: "MODEL"},
			{ID: "attention", Name: "Attention", Type: "METHOD"},
		},
		Relations: []indexer.Relation{{FromID: "transformer", ToID: "attention", Relation: "USES"}},
	}

	query, vars := recordUpsertQuery(record)
	if !strings.Contains(query, "c as var(func: eq(quark.chunk_id, $chunk))") {
		t.Fatalf("record query missing chunk upsert var: %s", query)
	}
	if vars["$chunk"] != "chunk-1" || vars["$entity0"] != "transformer" || vars["$relation0"] != "transformer|USES|attention" {
		t.Fatalf("record query vars = %+v", vars)
	}

	nquads := recordMutationNQuads(record, `{"filename":"paper.pdf"}`, `{}`)
	for _, want := range []string{
		`uid(c) <quark.chunk_id> "chunk-1"`,
		`uid(c) <quark.embedding> "[0.1,0.2]"`,
		`uid(e0) <quark.entity_id> "transformer"`,
		`uid(c) <quark.chunk_entity> uid(e0)`,
		`uid(r0) <quark.relation_from> uid(e0)`,
		`uid(r0) <quark.relation_to> uid(e1)`,
	} {
		if !strings.Contains(nquads, want) {
			t.Fatalf("record nquads missing %q:\n%s", want, nquads)
		}
	}
}

func TestVectorSearchPayloadRestoresCanonicalRecord(t *testing.T) {
	t.Parallel()

	payload := vectorSearchPayload{Chunks: []struct {
		ID            string  `json:"quark.chunk_id"`
		Text          string  `json:"quark.text_content"`
		MetadataJSON  string  `json:"quark.metadata_json"`
		CanonicalJSON string  `json:"quark.canonical_json"`
		Score         float32 `json:"score"`
	}{{
		ID:           "chunk-1",
		Text:         "hello",
		MetadataJSON: `{"path":"fixture.pdf"}`,
		CanonicalJSON: `{
			"document":{"id":"doc-1","source_uri":"fixture.pdf"},
			"embedding_metadata":{"provider":"local","model":"local-hash-v1","dimensions":2},
			"citations":[{"source_uri":"fixture.pdf","chunk_id":"chunk-1"}],
			"provenance":{"source_uri":"fixture.pdf","trace_id":"trace-1"}
		}`,
		Score: 0.9,
	}},
	}

	chunks := payload.chunks()
	if len(chunks) != 1 {
		t.Fatalf("chunks = %d, want 1", len(chunks))
	}
	chunk := chunks[0]
	if chunk.Document.ID != "doc-1" || chunk.EmbeddingMetadata.Provider != "local" {
		t.Fatalf("canonical record was not restored: %+v", chunk)
	}
	if len(chunk.Citations) != 1 || chunk.Provenance.TraceID != "trace-1" {
		t.Fatalf("citation/provenance not restored: %+v", chunk)
	}
}

func TestDgraphEntityListAcceptsScalarAndList(t *testing.T) {
	t.Parallel()

	var scalar dgraphEntityList
	if err := json.Unmarshal([]byte(`{"quark.entity_id":"quark","quark.entity_name":"Quark"}`), &scalar); err != nil {
		t.Fatalf("decode scalar entity: %v", err)
	}
	if len(scalar) != 1 || scalar[0].ID != "quark" {
		t.Fatalf("scalar decode = %+v", scalar)
	}

	var list dgraphEntityList
	if err := json.Unmarshal([]byte(`[{"quark.entity_id":"dgraph","quark.entity_name":"Dgraph"}]`), &list); err != nil {
		t.Fatalf("decode entity list: %v", err)
	}
	if len(list) != 1 || list[0].ID != "dgraph" {
		t.Fatalf("list decode = %+v", list)
	}
}

func TestIndexerWithDgraph(t *testing.T) {
	addr := os.Getenv("DGRAPH_TEST_ADDR")
	if addr == "" {
		t.Skip("set DGRAPH_TEST_ADDR to run Dgraph-backed indexer integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	driver, err := New(ctx, Config{Address: addr})
	if err != nil {
		t.Fatal(err)
	}
	defer driver.Close()

	svc, err := indexing.New(driver)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.IndexDocument(ctx, indexing.IndexCommand{
		ChunkID: "chunk-1",
		Text:    "Quark extracts services behind gRPC contracts.",
		Vector:  []float32{1, 0},
		Entities: []indexer.Entity{
			{ID: "quark", Name: "Quark", Type: "PROJECT"},
			{ID: "grpc", Name: "gRPC", Type: "TECH"},
		},
		Relations: []indexer.Relation{
			{FromID: "quark", ToID: "grpc", Relation: "USES"},
		},
		Metadata: map[string]string{"source": "docs/plan.md", "tenant": "test"},
	}); err != nil {
		t.Fatal(err)
	}
	resp, err := svc.GetContext(ctx, indexing.ContextQuery{
		Vector:  []float32{1, 0},
		Limit:   3,
		Depth:   2,
		Filters: map[string]string{"tenant": "test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(resp.Chunks); got != 1 {
		t.Fatalf("chunks = %d, want 1", got)
	}
	if resp.Chunks[0].ID != "chunk-1" {
		t.Fatalf("top chunk = %q, want chunk-1", resp.Chunks[0].ID)
	}
	if !strings.Contains(resp.ReasoningContext, "Graph relationships") {
		t.Fatalf("reasoning context missing graph: %q", resp.ReasoningContext)
	}
}
