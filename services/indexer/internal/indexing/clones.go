package indexing

import "github.com/quarkloop/services/indexer/pkg/indexer"

func cloneVector(in []float32) []float32 {
	if len(in) == 0 {
		return nil
	}
	out := make([]float32, len(in))
	copy(out, in)
	return out
}

func cloneMetadata(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneChunks(in []indexer.Chunk) []indexer.Chunk {
	out := make([]indexer.Chunk, len(in))
	for i, chunk := range in {
		out[i] = cloneChunk(chunk)
	}
	return out
}

func cloneChunk(chunk indexer.Chunk) indexer.Chunk {
	return indexer.Chunk{
		ID:                chunk.ID,
		Text:              chunk.Text,
		Vector:            cloneVector(chunk.Vector),
		Metadata:          cloneMetadata(chunk.Metadata),
		Document:          cloneDocument(chunk.Document),
		EmbeddingMetadata: cloneEmbeddingMetadata(chunk.EmbeddingMetadata),
		Facts:             cloneFacts(chunk.Facts),
		Citations:         cloneCitations(chunk.Citations),
		Provenance:        cloneProvenance(chunk.Provenance),
		Score:             chunk.Score,
	}
}

func cloneChunkWithScore(chunk indexer.Chunk, score float32) indexer.Chunk {
	cloned := cloneChunk(chunk)
	cloned.Score = score
	return cloned
}

func cloneDocument(in indexer.Document) indexer.Document {
	return indexer.Document{
		ID:        in.ID,
		Name:      in.Name,
		Type:      in.Type,
		SourceURI: in.SourceURI,
		Metadata:  cloneMetadata(in.Metadata),
		Sources:   cloneSourceReferences(in.Sources),
	}
}

func cloneFacts(in []indexer.Fact) []indexer.Fact {
	out := make([]indexer.Fact, len(in))
	for i, fact := range in {
		out[i] = indexer.Fact{
			ID:         fact.ID,
			Subject:    fact.Subject,
			Predicate:  fact.Predicate,
			Object:     fact.Object,
			Confidence: fact.Confidence,
			Citations:  cloneCitations(fact.Citations),
			Metadata:   cloneMetadata(fact.Metadata),
		}
	}
	return out
}

func cloneCitations(in []indexer.Citation) []indexer.Citation {
	out := make([]indexer.Citation, len(in))
	copy(out, in)
	return out
}

func cloneProvenance(in indexer.Provenance) indexer.Provenance {
	return indexer.Provenance{
		SourceURI:  in.SourceURI,
		SourceHash: in.SourceHash,
		IngestedAt: in.IngestedAt,
		ProducedBy: in.ProducedBy,
		TraceID:    in.TraceID,
		Metadata:   cloneMetadata(in.Metadata),
		Sources:    cloneSourceReferences(in.Sources),
	}
}

func cloneSourceReferences(in []indexer.SourceReference) []indexer.SourceReference {
	out := make([]indexer.SourceReference, len(in))
	for i, source := range in {
		out[i] = source
		out[i].Metadata = cloneMetadata(source.Metadata)
	}
	return out
}

func cloneEmbeddingMetadata(in indexer.EmbeddingMetadata) indexer.EmbeddingMetadata {
	in.Modalities = append([]string(nil), in.Modalities...)
	return in
}

func cloneGraphFragment(in *indexer.GraphFragment) *indexer.GraphFragment {
	if in == nil {
		return nil
	}
	out := &indexer.GraphFragment{
		Nodes: make([]indexer.GraphNode, len(in.Nodes)),
		Edges: make([]indexer.GraphEdge, len(in.Edges)),
	}
	copy(out.Nodes, in.Nodes)
	copy(out.Edges, in.Edges)
	return out
}
