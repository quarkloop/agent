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
	metadataQuery := metadataSearchQuery(2, map[string]string{"filename": "receipt.md"})
	for _, want := range []string{
		"chunks(func: has(quark.chunk_id), first: 2)",
		"@filter(eq(quark.meta_filename_",
		`"receipt.md"`,
		"quark.canonical_json",
	} {
		if !strings.Contains(metadataQuery, want) {
			t.Fatalf("metadata search query missing %q:\n%s", want, metadataQuery)
		}
	}
}

func TestVectorCandidateLimitOverfetchesWhenMetadataFiltersArePresent(t *testing.T) {
	t.Parallel()
	if got := vectorCandidateLimit(1, nil); got != 1 {
		t.Fatalf("unfiltered candidate limit = %d, want 1", got)
	}
	if got := vectorCandidateLimit(1, map[string]string{"filename": "receipt.md"}); got != 100 {
		t.Fatalf("filtered candidate limit = %d, want 100", got)
	}
	if got := vectorCandidateLimit(20, map[string]string{"filename": "receipt.md"}); got != 400 {
		t.Fatalf("filtered scaled candidate limit = %d, want 400", got)
	}
}

func TestTrimChunksAppliesCallerLimitAfterFilteredSearch(t *testing.T) {
	t.Parallel()
	chunks := []indexer.Chunk{{ID: "a"}, {ID: "b"}}
	got := trimChunks(chunks, 1)
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("trimmed chunks = %+v", got)
	}
	if got := trimChunks(chunks, 3); len(got) != 2 {
		t.Fatalf("untrimmed chunks = %+v", got)
	}
}

func TestMergeChunksKeepsVectorOrderAndAddsExactMatches(t *testing.T) {
	t.Parallel()
	got := mergeChunks(
		[]indexer.Chunk{{ID: "a", Score: 0.9}, {ID: "b", Score: 0.7}},
		[]indexer.Chunk{{ID: "b", Score: 0}, {ID: "c", Score: 0}},
	)
	if len(got) != 3 {
		t.Fatalf("merged chunks = %+v", got)
	}
	for i, want := range []string{"a", "b", "c"} {
		if got[i].ID != want {
			t.Fatalf("merged chunk %d = %q, want %q: %+v", i, got[i].ID, want, got)
		}
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
			Document: indexer.Document{ID: "doc-1", Name: "paper.pdf", SourceURI: "file:///paper.pdf"},
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
	if vars["$chunk"] != "chunk-1" || vars["$entity0"] != "transformer" || vars["$relation0"] != "chunk-1|transformer|USES|attention" {
		t.Fatalf("record query vars = %+v", vars)
	}
	if vars["$document"] != "doc-1" || !strings.Contains(query, "d as var(func: eq(quark.document_id, $document))") {
		t.Fatalf("record query missing document lookup: query=%s vars=%+v", query, vars)
	}

	nquads := recordMutationNQuads(record, `{"filename":"paper.pdf"}`, `{}`)
	for _, want := range []string{
		`uid(d) <dgraph.type> "QuarkDocument"`,
		`uid(d) <quark.document_id> "doc-1"`,
		`uid(c) <quark.chunk_document> uid(d)`,
		`uid(c) <quark.chunk_id> "chunk-1"`,
		`uid(c) <quark.embedding> "[0.1,0.2]"`,
		`uid(e0) <quark.entity_id> "transformer"`,
		`uid(c) <quark.chunk_entity> uid(e0)`,
		`uid(c) <quark.chunk_relation> uid(r0)`,
		`uid(r0) <quark.relation_id> "chunk-1|transformer|USES|attention"`,
		`uid(r0) <quark.relation_from> uid(e0)`,
		`uid(r0) <quark.relation_to> uid(e1)`,
	} {
		if !strings.Contains(nquads, want) {
			t.Fatalf("record nquads missing %q:\n%s", want, nquads)
		}
	}
}

func TestRecordCleanupPlanRemovesPreviousChunkOwnedState(t *testing.T) {
	t.Parallel()

	keys := make(metadataKeySet)
	keys.addMap(map[string]string{"tenant": "acme", "old": "value"})

	nquads := recordCleanupNQuads(keys)
	for _, want := range []string{
		`uid(c) <quark.chunk_entity> * .`,
		`uid(c) <quark.chunk_relation> * .`,
		`uid(oldRelations) * * .`,
		`uid(c) <quark.text_content> * .`,
		`uid(c) <quark.embedding> * .`,
		`uid(c) <quark.metadata_json> * .`,
		`uid(c) <quark.canonical_json> * .`,
		`uid(c) <quark.chunk_document> * .`,
		`uid(c) <quark.meta_tenant_`,
		`uid(c) <quark.meta_old_`,
	} {
		if !strings.Contains(nquads, want) {
			t.Fatalf("cleanup nquads missing %q:\n%s", want, nquads)
		}
	}
}

func TestDeleteChunkPlanRemovesChunkAndOwnedRelations(t *testing.T) {
	t.Parallel()

	nquads := deleteChunkNQuads()
	for _, want := range []string{
		`uid(c) <quark.chunk_entity> * .`,
		`uid(c) <quark.chunk_relation> * .`,
		`uid(r) * * .`,
		`uid(f) * * .`,
		`uid(cite) * * .`,
		`uid(c) * * .`,
	} {
		if !strings.Contains(nquads, want) {
			t.Fatalf("delete nquads missing %q:\n%s", want, nquads)
		}
	}
}

func TestCanonicalMutationHelpersUseTypedRecords(t *testing.T) {
	t.Parallel()

	docNQuads := documentMutationNQuads("d", indexer.Document{
		ID:        "doc-1",
		Name:      "paper.pdf",
		Type:      "paper",
		SourceURI: "file:///paper.pdf",
		Metadata:  map[string]string{"tenant": "acme"},
	})
	for _, want := range []string{
		`uid(d) <dgraph.type> "QuarkDocument"`,
		`uid(d) <quark.document_id> "doc-1"`,
		`uid(d) <quark.document_metadata_json> "{\"tenant\":\"acme\"}"`,
	} {
		if !strings.Contains(docNQuads, want) {
			t.Fatalf("document nquads missing %q:\n%s", want, docNQuads)
		}
	}

	factNQuads := factMutationNQuads("f", indexer.Fact{
		ID:         "fact-1",
		Subject:    "Quark",
		Predicate:  "uses",
		Object:     "Dgraph",
		Confidence: 0.7,
	}, "chunk-1", `{}`) + factChunkLinkNQuads("chunk-1", "f")
	for _, want := range []string{
		`uid(f) <dgraph.type> "QuarkFact"`,
		`uid(f) <quark.fact_id> "fact-1"`,
		`uid(f) <quark.fact_chunk_id> "chunk-1"`,
		`uid(f) <quark.fact_chunk> uid(c)`,
		`uid(f) <quark.fact_confidence> 0.700000`,
	} {
		if !strings.Contains(factNQuads, want) {
			t.Fatalf("fact nquads missing %q:\n%s", want, factNQuads)
		}
	}

	citationNQuads := citationMutationNQuads("cite", indexer.Citation{
		ID:          "cite-1",
		SourceURI:   "file:///paper.pdf",
		ChunkID:     "chunk-1",
		TextSpan:    "attention",
		StartOffset: 11,
		EndOffset:   20,
		Confidence:  1,
	}) + citationChunkLinkNQuads("chunk-1", "cite")
	for _, want := range []string{
		`uid(cite) <dgraph.type> "QuarkCitation"`,
		`uid(cite) <quark.citation_id> "cite-1"`,
		`uid(cite) <quark.citation_start_offset> 11`,
		`uid(cite) <quark.citation_end_offset> 20`,
		`uid(cite) <quark.citation_chunk> uid(c)`,
	} {
		if !strings.Contains(citationNQuads, want) {
			t.Fatalf("citation nquads missing %q:\n%s", want, citationNQuads)
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
			"embedding_metadata":{"provider":"fixture","model":"fixture/embed","dimensions":2},
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
	if chunk.Document.ID != "doc-1" || chunk.EmbeddingMetadata.Provider != "fixture" {
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
