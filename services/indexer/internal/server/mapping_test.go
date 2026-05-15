package server

import (
	"testing"

	indexerv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/indexer/v1"
	"github.com/quarkloop/services/indexer/pkg/indexer"
)

func TestIndexCommandMapsProtoToOwnedDomainCommand(t *testing.T) {
	req := &indexerv1.IndexRequest{
		ChunkId:     "chunk-1",
		TextContent: "Quark indexes knowledge.",
		Embedding:   []float32{0.1, 0.2},
		SourceMetadata: map[string]string{
			"filename": "source.pdf",
		},
		Document:          &indexerv1.Document{Id: "doc-1", Name: "source.pdf", SourceUri: "file:///source.pdf", Metadata: map[string]string{"kind": "paper"}},
		EmbeddingMetadata: &indexerv1.EmbeddingMetadata{Provider: "local", Model: "local-hash-v1", Dimensions: 2, ContentHash: "hash"},
		Entities:          []*indexerv1.Entity{{Id: "quark", Name: "Quark", Type: "PROJECT"}},
		Relations:         []*indexerv1.Relation{{FromId: "quark", ToId: "knowledge", Relation: "INDEXES"}},
		Facts:             []*indexerv1.Fact{{Id: "fact-1", Subject: "Quark", Predicate: "indexes", Object: "knowledge", Metadata: map[string]string{"source": "llm"}}},
		Citations:         []*indexerv1.Citation{{Id: "cite-1", SourceUri: "file:///source.pdf", ChunkId: "chunk-1", TextSpan: "Quark indexes knowledge."}},
		Provenance:        &indexerv1.Provenance{SourceUri: "file:///source.pdf", SourceHash: "sha256:abc", TraceId: "trace-1", Metadata: map[string]string{"agent": "knowledge"}},
	}

	cmd := indexCommand(req)
	req.Embedding[0] = 9
	req.SourceMetadata["filename"] = "mutated"
	req.Document.Metadata["kind"] = "mutated"
	req.Facts[0].Metadata["source"] = "mutated"
	req.Provenance.Metadata["agent"] = "mutated"

	if cmd.Vector[0] != 0.1 {
		t.Fatalf("vector was not copied: %+v", cmd.Vector)
	}
	if cmd.Metadata["filename"] != "source.pdf" {
		t.Fatalf("metadata was not copied: %+v", cmd.Metadata)
	}
	if cmd.Document.Metadata["kind"] != "paper" {
		t.Fatalf("document metadata was not copied: %+v", cmd.Document.Metadata)
	}
	if cmd.Facts[0].Metadata["source"] != "llm" {
		t.Fatalf("fact metadata was not copied: %+v", cmd.Facts[0].Metadata)
	}
	if cmd.Provenance.Metadata["agent"] != "knowledge" {
		t.Fatalf("provenance metadata was not copied: %+v", cmd.Provenance.Metadata)
	}
}

func TestContextResponseMapsDomainToOwnedProtoResponse(t *testing.T) {
	chunks := []indexer.Chunk{{
		ID:       "chunk-1",
		Text:     "Quark indexes knowledge.",
		Metadata: map[string]string{"filename": "source.pdf"},
		Document: indexer.Document{ID: "doc-1", Metadata: map[string]string{"kind": "paper"}},
		Facts: []indexer.Fact{{
			ID:       "fact-1",
			Subject:  "Quark",
			Metadata: map[string]string{"source": "llm"},
		}},
		Citations:  []indexer.Citation{{ID: "cite-1", SourceURI: "file:///source.pdf"}},
		Provenance: indexer.Provenance{SourceURI: "file:///source.pdf", Metadata: map[string]string{"agent": "knowledge"}},
	}}

	protoChunks := toProtoChunks(chunks)
	chunks[0].Metadata["filename"] = "mutated"
	chunks[0].Document.Metadata["kind"] = "mutated"
	chunks[0].Facts[0].Metadata["source"] = "mutated"
	chunks[0].Provenance.Metadata["agent"] = "mutated"
	protoChunks[0].Metadata["filename"] = "proto-mutated"

	if protoChunks[0].Metadata["filename"] != "proto-mutated" {
		t.Fatalf("sanity check failed: proto mutation did not apply")
	}
	if chunks[0].Metadata["filename"] != "mutated" {
		t.Fatalf("domain chunk metadata unexpectedly changed: %+v", chunks[0].Metadata)
	}
	if protoChunks[0].Document.Metadata["kind"] != "paper" {
		t.Fatalf("document metadata was not copied to proto: %+v", protoChunks[0].Document.Metadata)
	}
	if protoChunks[0].Facts[0].Metadata["source"] != "llm" {
		t.Fatalf("fact metadata was not copied to proto: %+v", protoChunks[0].Facts[0].Metadata)
	}
	if protoChunks[0].Provenance.Metadata["agent"] != "knowledge" {
		t.Fatalf("provenance metadata was not copied to proto: %+v", protoChunks[0].Provenance.Metadata)
	}
}
