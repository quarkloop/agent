package indexing

import (
	"fmt"
	"math"
	"strings"

	"github.com/quarkloop/services/indexer/pkg/indexer"
)

func ReasoningContext(chunks []indexer.Chunk, graph *indexer.GraphFragment) string {
	var b strings.Builder
	for _, chunk := range chunks {
		fmt.Fprintf(&b, "Chunk %s (score %.4f):\n%s\n\n", chunk.ID, chunk.Score, chunk.Text)
		if chunk.Document.SourceURI != "" || chunk.Provenance.SourceURI != "" {
			fmt.Fprintf(&b, "Source: %s\n\n", firstNonEmpty(chunk.Provenance.SourceURI, chunk.Document.SourceURI))
		}
		if len(chunk.Facts) > 0 {
			b.WriteString("Facts:\n")
			for _, fact := range chunk.Facts {
				fmt.Fprintf(&b, "- %s %s %s", fact.Subject, fact.Predicate, fact.Object)
				if fact.Confidence > 0 {
					fmt.Fprintf(&b, " (confidence %.2f)", fact.Confidence)
				}
				b.WriteString("\n")
			}
			b.WriteString("\n")
		}
	}
	if graph != nil && len(graph.Edges) > 0 {
		b.WriteString("Graph relationships:\n")
		for _, edge := range graph.Edges {
			fmt.Fprintf(&b, "- %s -[%s]-> %s\n", edge.FromID, edge.Relation, edge.ToID)
		}
	}
	return strings.TrimSpace(b.String())
}

func Citations(chunks []indexer.Chunk) []string {
	out := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		for _, citation := range chunk.Citations {
			if citation.SourceURI != "" {
				out = append(out, citation.SourceURI)
			} else if citation.ChunkID != "" {
				out = append(out, citation.ChunkID)
			}
		}
		if len(chunk.Citations) > 0 {
			continue
		}
		if source := chunk.Provenance.SourceURI; source != "" {
			out = append(out, source)
			continue
		}
		if source := chunk.Document.SourceURI; source != "" {
			out = append(out, source)
			continue
		}
		if source := chunk.Metadata["source"]; source != "" {
			out = append(out, source)
			continue
		}
		if path := chunk.Metadata["path"]; path != "" {
			out = append(out, path)
			continue
		}
		out = append(out, chunk.ID)
	}
	return out
}

func BuildContextPackage(chunks []indexer.Chunk, graph *indexer.GraphFragment) indexer.ContextPackage {
	chunks = cloneChunks(chunks)
	return indexer.ContextPackage{
		Chunks:     chunks,
		Facts:      contextFacts(chunks),
		Citations:  contextCitations(chunks),
		Provenance: contextProvenance(chunks),
		Graph:      cloneGraphFragment(graph),
		Confidence: contextConfidence(chunks),
	}
}

func normalizeScores(chunks []indexer.Chunk) []indexer.Chunk {
	out := cloneChunks(chunks)
	if len(out) == 0 {
		return out
	}
	minScore := float32(math.MaxFloat32)
	maxScore := float32(-math.MaxFloat32)
	for _, chunk := range out {
		if chunk.Score < minScore {
			minScore = chunk.Score
		}
		if chunk.Score > maxScore {
			maxScore = chunk.Score
		}
	}
	if minScore >= 0 && maxScore <= 1 {
		for i := range out {
			out[i] = cloneChunkWithScore(out[i], clamp01(out[i].Score))
		}
		return out
	}
	if maxScore == minScore {
		for i := range out {
			out[i] = cloneChunkWithScore(out[i], 1)
		}
		return out
	}
	spread := maxScore - minScore
	for i := range out {
		out[i] = cloneChunkWithScore(out[i], clamp01((out[i].Score-minScore)/spread))
	}
	return out
}

func contextFacts(chunks []indexer.Chunk) []indexer.Fact {
	seen := map[string]struct{}{}
	out := make([]indexer.Fact, 0)
	for _, chunk := range chunks {
		for _, fact := range chunk.Facts {
			key := firstNonEmpty(fact.ID, fact.Subject+"|"+fact.Predicate+"|"+fact.Object)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, cloneFacts([]indexer.Fact{fact})[0])
		}
	}
	return out
}

func contextCitations(chunks []indexer.Chunk) []indexer.Citation {
	seen := map[string]struct{}{}
	out := make([]indexer.Citation, 0)
	for _, chunk := range chunks {
		for _, citation := range chunk.Citations {
			key := firstNonEmpty(citation.ID, citation.SourceURI+"|"+citation.ChunkID+"|"+citation.TextSpan)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, citation)
		}
	}
	return out
}

func contextProvenance(chunks []indexer.Chunk) []indexer.Provenance {
	seen := map[string]struct{}{}
	out := make([]indexer.Provenance, 0)
	for _, chunk := range chunks {
		provenance := chunk.Provenance
		key := firstNonEmpty(provenance.TraceID, provenance.SourceURI+"|"+provenance.SourceHash)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, cloneProvenance(provenance))
	}
	return out
}

func contextConfidence(chunks []indexer.Chunk) float32 {
	if len(chunks) == 0 {
		return 0
	}
	var total float32
	for _, chunk := range chunks {
		total += clamp01(chunk.Score)
	}
	return clamp01(total / float32(len(chunks)))
}

func clamp01(value float32) float32 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
