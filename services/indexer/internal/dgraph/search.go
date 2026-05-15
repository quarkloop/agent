package dgraph

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/quarkloop/services/indexer/pkg/indexer"
)

func (d *Driver) VectorSearch(ctx context.Context, queryVector []float32, limit int, filters map[string]string) ([]indexer.Chunk, error) {
	if limit <= 0 {
		limit = 5
	}
	if err := d.ensureMetadataPredicates(ctx, filters); err != nil {
		return nil, err
	}
	candidateLimit := vectorCandidateLimit(limit, filters)
	resp, err := d.client.NewReadOnlyTxn().QueryWithVars(ctx, vectorSearchQuery(candidateLimit, filters), map[string]string{
		"$vec": vectorLiteral(queryVector),
	})
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	var payload vectorSearchPayload
	if err := json.Unmarshal(resp.GetJson(), &payload); err != nil {
		return nil, fmt.Errorf("decode vector search: %w", err)
	}
	chunks := payload.chunks()
	if len(filters) > 0 && len(chunks) < limit {
		exact, err := d.MetadataSearch(ctx, limit, filters)
		if err != nil {
			return nil, err
		}
		chunks = mergeChunks(chunks, exact)
	}
	return trimChunks(chunks, limit), nil
}

func (d *Driver) MetadataSearch(ctx context.Context, limit int, filters map[string]string) ([]indexer.Chunk, error) {
	if limit <= 0 {
		limit = 5
	}
	if len(filters) == 0 {
		return nil, nil
	}
	if err := d.ensureMetadataPredicates(ctx, filters); err != nil {
		return nil, err
	}
	resp, err := d.client.NewReadOnlyTxn().Query(ctx, metadataSearchQuery(limit, filters))
	if err != nil {
		return nil, fmt.Errorf("metadata search: %w", err)
	}
	var payload vectorSearchPayload
	if err := json.Unmarshal(resp.GetJson(), &payload); err != nil {
		return nil, fmt.Errorf("decode metadata search: %w", err)
	}
	return payload.chunks(), nil
}

func vectorCandidateLimit(limit int, filters map[string]string) int {
	if limit <= 0 {
		limit = 5
	}
	if len(filters) == 0 {
		return limit
	}
	candidateLimit := limit * 20
	if candidateLimit < 100 {
		candidateLimit = 100
	}
	return candidateLimit
}

func trimChunks(chunks []indexer.Chunk, limit int) []indexer.Chunk {
	if limit <= 0 || len(chunks) <= limit {
		return chunks
	}
	return chunks[:limit]
}

func mergeChunks(primary, fallback []indexer.Chunk) []indexer.Chunk {
	if len(primary) == 0 {
		return fallback
	}
	seen := make(map[string]struct{}, len(primary)+len(fallback))
	out := make([]indexer.Chunk, 0, len(primary)+len(fallback))
	for _, chunk := range primary {
		if chunk.ID == "" {
			continue
		}
		if _, ok := seen[chunk.ID]; ok {
			continue
		}
		seen[chunk.ID] = struct{}{}
		out = append(out, chunk)
	}
	for _, chunk := range fallback {
		if chunk.ID == "" {
			continue
		}
		if _, ok := seen[chunk.ID]; ok {
			continue
		}
		seen[chunk.ID] = struct{}{}
		out = append(out, chunk)
	}
	return out
}

type vectorSearchPayload struct {
	Chunks []struct {
		ID            string  `json:"quark.chunk_id"`
		Text          string  `json:"quark.text_content"`
		MetadataJSON  string  `json:"quark.metadata_json"`
		CanonicalJSON string  `json:"quark.canonical_json"`
		Score         float32 `json:"score"`
	} `json:"chunks"`
}

func (p vectorSearchPayload) chunks() []indexer.Chunk {
	out := make([]indexer.Chunk, 0, len(p.Chunks))
	for _, row := range p.Chunks {
		meta := map[string]string{}
		if row.MetadataJSON != "" {
			_ = json.Unmarshal([]byte(row.MetadataJSON), &meta)
		}
		var canonical canonicalChunk
		if row.CanonicalJSON != "" {
			_ = json.Unmarshal([]byte(row.CanonicalJSON), &canonical)
		}
		out = append(out, indexer.Chunk{
			ID:                row.ID,
			Text:              row.Text,
			Metadata:          meta,
			Document:          canonical.Document,
			EmbeddingMetadata: canonical.EmbeddingMetadata,
			Facts:             canonical.Facts,
			Citations:         canonical.Citations,
			Provenance:        canonical.Provenance,
			Score:             row.Score,
		})
	}
	return out
}
