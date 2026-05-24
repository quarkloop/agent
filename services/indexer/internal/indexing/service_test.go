package indexing

import (
	"context"
	"strings"
	"testing"

	"github.com/quarkloop/services/indexer/pkg/indexer"
)

func TestGetContextReturnsOwnedCopies(t *testing.T) {
	store := &fakeStore{
		chunks: []indexer.Chunk{{
			ID:                "chunk-1",
			Text:              "hello",
			Vector:            []float32{0.1, 0.2},
			Metadata:          map[string]string{"source": "fixture.pdf"},
			EmbeddingMetadata: indexer.EmbeddingMetadata{Provider: "fixture", Model: "fixture/embed", Dimensions: 2},
			Facts:             []indexer.Fact{{ID: "fact-1", Subject: "Quark", Predicate: "stores", Object: "context", Confidence: 0.8}},
			Citations:         []indexer.Citation{{SourceURI: "fixture.pdf", ChunkID: "chunk-1"}},
			Provenance:        indexer.Provenance{SourceURI: "fixture.pdf", Metadata: map[string]string{"trace": "t1"}},
			Score:             1.2,
		}},
		graph: &indexer.GraphFragment{
			Nodes: []indexer.GraphNode{{ID: "n1", Label: "Node", Type: "THING"}},
			Edges: []indexer.GraphEdge{{FromID: "n1", ToID: "n2", Relation: "related"}},
		},
	}
	svc, err := New(store)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	result, err := svc.GetContext(context.Background(), ContextQuery{Vector: []float32{0.1}, Limit: 1, Depth: 1})
	if err != nil {
		t.Fatalf("get context: %v", err)
	}

	result.Chunks[0].Vector[0] = 9
	result.Chunks[0].Metadata["source"] = "mutated"
	result.Chunks[0].Citations[0].SourceURI = "mutated"
	result.Chunks[0].Provenance.Metadata["trace"] = "mutated"
	result.Package.Chunks[0].Text = "mutated"
	result.Package.Facts[0].Object = "mutated"
	result.Package.Provenance[0].Metadata["trace"] = "mutated"
	result.Graph.Nodes[0].Label = "mutated"
	result.Graph.Edges[0].Relation = "mutated"

	if store.chunks[0].Vector[0] != 0.1 {
		t.Fatalf("chunk vector was mutated through result: %+v", store.chunks[0].Vector)
	}
	if store.chunks[0].Metadata["source"] != "fixture.pdf" {
		t.Fatalf("chunk metadata was mutated through result: %+v", store.chunks[0].Metadata)
	}
	if store.chunks[0].Citations[0].SourceURI != "fixture.pdf" {
		t.Fatalf("chunk citations were mutated through result: %+v", store.chunks[0].Citations)
	}
	if store.chunks[0].Provenance.Metadata["trace"] != "t1" {
		t.Fatalf("chunk provenance was mutated through result: %+v", store.chunks[0].Provenance)
	}
	if store.chunks[0].Facts[0].Object != "context" {
		t.Fatalf("chunk facts were mutated through package: %+v", store.chunks[0].Facts)
	}
	if store.graph.Nodes[0].Label != "Node" {
		t.Fatalf("graph node was mutated through result: %+v", store.graph.Nodes[0])
	}
	if store.graph.Edges[0].Relation != "related" {
		t.Fatalf("graph edge was mutated through result: %+v", store.graph.Edges[0])
	}
	if result.Chunks[0].Score != 1 {
		t.Fatalf("score was not normalized to [0,1]: %f", result.Chunks[0].Score)
	}
	if result.Package.Confidence != 1 {
		t.Fatalf("context package confidence = %f, want 1", result.Package.Confidence)
	}
}

func TestIndexDocumentNormalizesCanonicalRecord(t *testing.T) {
	store := &fakeStore{}
	svc, err := New(store)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	err = svc.IndexDocument(context.Background(), IndexCommand{
		ChunkID: "chunk-1",
		Text:    "Quark indexes agent-produced records.",
		Vector:  []float32{0.1, 0.2, 0.3},
		Metadata: map[string]string{
			"path":                   "/tmp/source.pdf",
			"filename":               "source.pdf",
			"document_type":          "paper",
			"embedding_provider":     "fixture",
			"embedding_model":        "fixture/embed",
			"embedding_dimensions":   "3",
			"embedding_content_hash": "abc123",
			"trace_id":               "trace-1",
		},
		Facts: []indexer.Fact{{
			Subject:   "Quark",
			Predicate: "indexes",
			Object:    "records",
		}},
	})
	if err != nil {
		t.Fatalf("index document: %v", err)
	}
	if len(store.inserted) != 1 {
		t.Fatalf("upserted records = %d, want 1", len(store.inserted))
	}
	chunk := store.inserted[0].Chunk
	if chunk.Document.SourceURI != "/tmp/source.pdf" || chunk.Document.Name != "source.pdf" || chunk.Document.Type != "paper" {
		t.Fatalf("document normalization failed: %+v", chunk.Document)
	}
	if chunk.EmbeddingMetadata.Provider != "fixture" || chunk.EmbeddingMetadata.Model != "fixture/embed" || chunk.EmbeddingMetadata.Dimensions != 3 || chunk.EmbeddingMetadata.ContentHash != "abc123" {
		t.Fatalf("embedding metadata normalization failed: %+v", chunk.EmbeddingMetadata)
	}
	if chunk.Provenance.SourceURI != "/tmp/source.pdf" || chunk.Provenance.TraceID != "trace-1" {
		t.Fatalf("provenance normalization failed: %+v", chunk.Provenance)
	}
	if chunk.Provenance.SourceHash == "" {
		t.Fatalf("provenance source hash was not defined: %+v", chunk.Provenance)
	}
	if len(chunk.Citations) != 1 || chunk.Citations[0].SourceURI != "/tmp/source.pdf" || chunk.Citations[0].ChunkID != "chunk-1" {
		t.Fatalf("citation normalization failed: %+v", chunk.Citations)
	}
	if len(chunk.Facts) != 1 || chunk.Facts[0].Subject != "Quark" || len(chunk.Facts[0].Citations) != 1 {
		t.Fatalf("fact normalization failed: %+v", chunk.Facts)
	}
}

func TestIndexDocumentRejectsEmbeddingDimensionMismatch(t *testing.T) {
	svc, err := New(&fakeStore{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	err = svc.IndexDocument(context.Background(), IndexCommand{
		ChunkID:           "chunk-1",
		Text:              "hello",
		Vector:            []float32{0.1, 0.2},
		EmbeddingMetadata: indexer.EmbeddingMetadata{Dimensions: 3},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestIndexDocumentDerivesSearchMetadataFromCanonicalSourceFields(t *testing.T) {
	store := &fakeStore{}
	svc, err := New(store)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	err = svc.UpsertChunk(context.Background(), IndexCommand{
		ChunkID: "chunk-invoice",
		Text:    "Invoice INV-2026-014 for Northwind Retail GmbH totals EUR 18,450.00.",
		Vector:  []float32{0.1, 0.2, 0.3},
		Document: indexer.Document{
			ID:        "doc-invoice",
			Name:      "Aurora cloud migration invoice",
			Type:      "invoice",
			SourceURI: "/uploads/company-records/invoice_2026_aurora_cloud_migration.md",
			Metadata:  map[string]string{"tenant": "e2e"},
		},
		Provenance: indexer.Provenance{
			SourceHash: "hash-invoice",
			Metadata:   map[string]string{"run_id": "run-1"},
		},
	})
	if err != nil {
		t.Fatalf("upsert chunk: %v", err)
	}

	chunk := store.inserted[0].Chunk
	if got := chunk.Metadata["filename"]; got != "invoice_2026_aurora_cloud_migration.md" {
		t.Fatalf("derived filename = %q, want invoice filename: %+v", got, chunk.Metadata)
	}
	if got := chunk.Metadata["source_uri"]; got != "/uploads/company-records/invoice_2026_aurora_cloud_migration.md" {
		t.Fatalf("derived source_uri = %q: %+v", got, chunk.Metadata)
	}
	if got := chunk.Metadata["document_id"]; got != "doc-invoice" {
		t.Fatalf("derived document_id = %q: %+v", got, chunk.Metadata)
	}
	if got := chunk.Metadata["source_hash"]; got != "hash-invoice" {
		t.Fatalf("derived source_hash = %q: %+v", got, chunk.Metadata)
	}
	if got := chunk.Metadata["tenant"]; got != "e2e" {
		t.Fatalf("document metadata was not copied into searchable metadata: %+v", chunk.Metadata)
	}
	if got := chunk.Metadata["run_id"]; got != "run-1" {
		t.Fatalf("provenance metadata was not copied into searchable metadata: %+v", chunk.Metadata)
	}
	if chunk.Document.Name != "Aurora cloud migration invoice" {
		t.Fatalf("document name was overwritten: %+v", chunk.Document)
	}
}

func TestIndexDocumentRejectsMixedEmbeddingDimensions(t *testing.T) {
	svc, err := New(&fakeStore{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if err := svc.IndexDocument(context.Background(), IndexCommand{
		ChunkID:           "chunk-1",
		Text:              "hello",
		Vector:            []float32{0.1, 0.2},
		EmbeddingMetadata: indexer.EmbeddingMetadata{Provider: "fixture", Model: "fixture/embed", Dimensions: 2},
	}); err != nil {
		t.Fatalf("first index document: %v", err)
	}

	err = svc.IndexDocument(context.Background(), IndexCommand{
		ChunkID:           "chunk-2",
		Text:              "goodbye",
		Vector:            []float32{0.1, 0.2, 0.3},
		EmbeddingMetadata: indexer.EmbeddingMetadata{Provider: "fixture", Model: "fixture/embed", Dimensions: 3},
	})
	if err == nil || !strings.Contains(err.Error(), "2-dimensional embeddings") {
		t.Fatalf("expected mixed-dimension validation error, got %v", err)
	}
}

func TestGetContextRejectsQueryDimensionMismatch(t *testing.T) {
	svc, err := New(&fakeStore{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if err := svc.IndexDocument(context.Background(), IndexCommand{
		ChunkID:           "chunk-1",
		Text:              "hello",
		Vector:            []float32{0.1, 0.2},
		EmbeddingMetadata: indexer.EmbeddingMetadata{Provider: "fixture", Model: "fixture/embed", Dimensions: 2},
	}); err != nil {
		t.Fatalf("index document: %v", err)
	}

	_, err = svc.GetContext(context.Background(), ContextQuery{Vector: []float32{0.1, 0.2, 0.3}})
	if err == nil || !strings.Contains(err.Error(), "2-dimensional embeddings") {
		t.Fatalf("expected query dimension validation error, got %v", err)
	}
}

func TestIndexDocumentDeduplicatesPrimaryGraphWrites(t *testing.T) {
	store := &fakeStore{}
	svc, err := New(store)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	err = svc.IndexDocument(context.Background(), IndexCommand{
		ChunkID: "chunk-1",
		Text:    "Productivity apps include calendar planning.",
		Vector:  []float32{0.1, 0.2},
		Entities: []indexer.Entity{
			{ID: "category_productivity", Name: "Productivity", Type: "CATEGORY"},
			{ID: "category_productivity", Name: "Productivity", Type: "CATEGORY"},
			{ID: "calendar_planner", Name: "Calendar Planner", Type: "APP"},
		},
		Relations: []indexer.Relation{
			{FromID: "calendar_planner", ToID: "category_productivity", Relation: "BELONGS_TO"},
			{FromID: "calendar_planner", ToID: "category_productivity", Relation: "BELONGS_TO"},
		},
	})
	if err != nil {
		t.Fatalf("index document: %v", err)
	}

	if len(store.inserted) != 1 {
		t.Fatalf("upserted records = %d, want 1: %+v", len(store.inserted), store.inserted)
	}
	record := store.inserted[0]
	if len(record.Entities) != 2 {
		t.Fatalf("record entities = %d, want 2: %+v", len(record.Entities), record.Entities)
	}
	if len(record.Relations) != 1 {
		t.Fatalf("record relations = %d, want 1: %+v", len(record.Relations), record.Relations)
	}
}

func TestIndexDocumentUpsertsDuplicateChunkAsOneCanonicalRecord(t *testing.T) {
	store := &fakeStore{}
	svc, err := New(store)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	for _, text := range []string{"first", "updated"} {
		if err := svc.IndexDocument(context.Background(), IndexCommand{
			ChunkID: "chunk-1",
			Text:    text,
			Vector:  []float32{0.1, 0.2},
		}); err != nil {
			t.Fatalf("index document %q: %v", text, err)
		}
	}

	if len(store.inserted) != 2 {
		t.Fatalf("upsert calls = %d, want 2", len(store.inserted))
	}
	if got := store.inserted[1].Chunk.Text; got != "updated" {
		t.Fatalf("latest upsert text = %q, want updated", got)
	}
	if store.inserted[0].Chunk.ID != store.inserted[1].Chunk.ID {
		t.Fatalf("duplicate chunk changed canonical ID: %+v", store.inserted)
	}
}

func TestDeleteChunkValidatesAndUsesStoreBoundary(t *testing.T) {
	store := &fakeStore{}
	svc, err := New(store)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if err := svc.DeleteChunk(context.Background(), " "); err == nil {
		t.Fatal("expected blank chunk delete to fail")
	}
	if err := svc.DeleteChunk(context.Background(), " chunk-1 "); err != nil {
		t.Fatalf("delete chunk: %v", err)
	}
	if len(store.deleted) != 1 || store.deleted[0] != "chunk-1" {
		t.Fatalf("deleted chunks = %+v, want chunk-1", store.deleted)
	}
}

func TestCanonicalStorageFunctionsValidateAndUseStoreBoundary(t *testing.T) {
	store := &fakeStore{}
	svc, err := New(store)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	if err := svc.UpsertDocument(context.Background(), indexer.Document{ID: " doc-1 ", Name: "Source"}); err != nil {
		t.Fatalf("upsert document: %v", err)
	}
	if err := svc.UpsertEntity(context.Background(), indexer.Entity{Name: "Quark", Type: "PROJECT"}); err != nil {
		t.Fatalf("upsert entity: %v", err)
	}
	if err := svc.UpsertRelation(context.Background(), indexer.Relation{FromID: "quark", ToID: "dgraph", Relation: "USES"}, " chunk-1 "); err != nil {
		t.Fatalf("upsert relation: %v", err)
	}
	if err := svc.UpsertFact(context.Background(), indexer.Fact{
		Subject:    "Quark",
		Predicate:  "uses",
		Object:     "Dgraph",
		Confidence: 0.9,
		Citations:  []indexer.Citation{{SourceURI: "paper.pdf"}},
	}, "chunk-1"); err != nil {
		t.Fatalf("upsert fact: %v", err)
	}
	if err := svc.UpsertCitation(context.Background(), indexer.Citation{SourceURI: "paper.pdf", TextSpan: "Quark uses Dgraph.", Confidence: 0.8}, "chunk-1"); err != nil {
		t.Fatalf("upsert citation: %v", err)
	}
	if err := svc.DeleteDocument(context.Background(), " doc-1 "); err != nil {
		t.Fatalf("delete document: %v", err)
	}

	if len(store.documents) != 1 || store.documents[0].ID != "doc-1" {
		t.Fatalf("documents = %+v, want doc-1", store.documents)
	}
	if len(store.entities) != 1 || store.entities[0].ID == "" || store.entities[0].Name != "Quark" {
		t.Fatalf("entities = %+v, want normalized Quark entity", store.entities)
	}
	if len(store.relations) != 1 || store.relations[0].chunkID != "chunk-1" {
		t.Fatalf("relations = %+v, want chunk-linked relation", store.relations)
	}
	if len(store.facts) != 1 || store.facts[0].fact.ID == "" || len(store.facts[0].fact.Citations) != 1 {
		t.Fatalf("facts = %+v, want normalized fact with citation", store.facts)
	}
	if len(store.citations) != 1 || store.citations[0].citation.ChunkID != "chunk-1" {
		t.Fatalf("citations = %+v, want chunk-linked citation", store.citations)
	}
	if len(store.deletedDocuments) != 1 || store.deletedDocuments[0] != "doc-1" {
		t.Fatalf("deleted documents = %+v, want doc-1", store.deletedDocuments)
	}

	for name, call := range map[string]func() error{
		"document": func() error { return svc.UpsertDocument(context.Background(), indexer.Document{}) },
		"relation": func() error {
			return svc.UpsertRelation(context.Background(), indexer.Relation{FromID: "a"}, "")
		},
		"fact": func() error {
			return svc.UpsertFact(context.Background(), indexer.Fact{Subject: "a"}, "")
		},
		"citation": func() error {
			return svc.UpsertCitation(context.Background(), indexer.Citation{StartOffset: 9, EndOffset: 1}, "")
		},
		"deleteDocument": func() error { return svc.DeleteDocument(context.Background(), " ") },
	} {
		if err := call(); err == nil {
			t.Fatalf("%s: expected validation error", name)
		}
	}
}

func TestIndexDocumentPreservesGraphVectorConsistencyInOneStoreRecord(t *testing.T) {
	store := &fakeStore{}
	svc, err := New(store)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	vector := []float32{0.4, 0.6}
	err = svc.IndexDocument(context.Background(), IndexCommand{
		ChunkID: "chunk-graph",
		Text:    "Transformer uses attention.",
		Vector:  vector,
		Entities: []indexer.Entity{
			{ID: "transformer", Name: "Transformer", Type: "MODEL"},
			{ID: "attention", Name: "Attention", Type: "METHOD"},
		},
		Relations: []indexer.Relation{{FromID: "transformer", ToID: "attention", Relation: "USES"}},
	})
	if err != nil {
		t.Fatalf("index document: %v", err)
	}
	record := store.inserted[0]
	if record.Chunk.ID == "" || len(record.Chunk.Vector) != 2 || len(record.Entities) != 2 || len(record.Relations) != 1 {
		t.Fatalf("record did not keep vector and graph state together: %+v", record)
	}
	vector[0] = 99
	if store.inserted[0].Chunk.Vector[0] != 0.4 {
		t.Fatalf("store record leaked vector backing slice: %+v", store.inserted[0].Chunk.Vector)
	}
}

func TestBuildContextPackageDeduplicatesEvidence(t *testing.T) {
	chunks := []indexer.Chunk{
		{
			ID:    "chunk-1",
			Text:  "alpha",
			Score: 0.2,
			Facts: []indexer.Fact{{
				ID:         "fact-1",
				Subject:    "A",
				Predicate:  "mentions",
				Object:     "B",
				Confidence: 0.7,
			}},
			Citations:  []indexer.Citation{{ID: "cite-1", SourceURI: "source.pdf", ChunkID: "chunk-1"}},
			Provenance: indexer.Provenance{SourceURI: "source.pdf", TraceID: "trace-1", Metadata: map[string]string{"source": "source.pdf"}},
		},
		{
			ID:         "chunk-2",
			Text:       "beta",
			Score:      0.8,
			Facts:      []indexer.Fact{{ID: "fact-1", Subject: "A", Predicate: "mentions", Object: "B"}},
			Citations:  []indexer.Citation{{ID: "cite-1", SourceURI: "source.pdf", ChunkID: "chunk-1"}},
			Provenance: indexer.Provenance{SourceURI: "source.pdf", TraceID: "trace-1"},
		},
	}

	pkg := BuildContextPackage(chunks, &indexer.GraphFragment{Nodes: []indexer.GraphNode{{ID: "A"}}})
	if len(pkg.Chunks) != 2 || len(pkg.Facts) != 1 || len(pkg.Citations) != 1 || len(pkg.Provenance) != 1 {
		t.Fatalf("unexpected context package: %+v", pkg)
	}
	if pkg.Confidence != 0.5 {
		t.Fatalf("confidence = %f, want 0.5", pkg.Confidence)
	}
	pkg.Provenance[0].Metadata["source"] = "mutated"
	if chunks[0].Provenance.Metadata["source"] != "source.pdf" {
		t.Fatalf("context package leaked provenance metadata backing map")
	}
}

type fakeStore struct {
	inserted         []indexer.KnowledgeRecord
	documents        []indexer.Document
	entities         []indexer.Entity
	relations        []storedRelation
	facts            []storedFact
	citations        []storedCitation
	deleted          []string
	deletedDocuments []string
	chunks           []indexer.Chunk
	graph            *indexer.GraphFragment
}

type storedRelation struct {
	relation indexer.Relation
	chunkID  string
}

type storedFact struct {
	fact    indexer.Fact
	chunkID string
}

type storedCitation struct {
	citation indexer.Citation
	chunkID  string
}

func (s *fakeStore) UpsertRecord(_ context.Context, record indexer.KnowledgeRecord) error {
	record.Chunk = cloneChunks([]indexer.Chunk{record.Chunk})[0]
	record.Entities = append([]indexer.Entity(nil), record.Entities...)
	record.Relations = append([]indexer.Relation(nil), record.Relations...)
	s.inserted = append(s.inserted, record)
	return nil
}

func (s *fakeStore) UpsertDocument(_ context.Context, document indexer.Document) error {
	document.Metadata = cloneMetadata(document.Metadata)
	s.documents = append(s.documents, document)
	return nil
}

func (s *fakeStore) UpsertEntity(_ context.Context, entity indexer.Entity) error {
	s.entities = append(s.entities, entity)
	return nil
}

func (s *fakeStore) UpsertRelation(_ context.Context, relation indexer.Relation, chunkID string) error {
	s.relations = append(s.relations, storedRelation{relation: relation, chunkID: chunkID})
	return nil
}

func (s *fakeStore) UpsertFact(_ context.Context, fact indexer.Fact, chunkID string) error {
	fact.Citations = cloneCitations(fact.Citations)
	fact.Metadata = cloneMetadata(fact.Metadata)
	s.facts = append(s.facts, storedFact{fact: fact, chunkID: chunkID})
	return nil
}

func (s *fakeStore) UpsertCitation(_ context.Context, citation indexer.Citation, chunkID string) error {
	s.citations = append(s.citations, storedCitation{citation: citation, chunkID: chunkID})
	return nil
}

func (s *fakeStore) DeleteDocument(_ context.Context, documentID string) error {
	s.deletedDocuments = append(s.deletedDocuments, documentID)
	return nil
}

func (s *fakeStore) DeleteChunk(_ context.Context, chunkID string) error {
	s.deleted = append(s.deleted, chunkID)
	return nil
}

func (s *fakeStore) VectorSearch(context.Context, []float32, int, map[string]string) ([]indexer.Chunk, error) {
	return s.chunks, nil
}

func (s *fakeStore) GetNeighborhood(context.Context, string, int) (*indexer.GraphFragment, error) {
	return s.graph, nil
}
