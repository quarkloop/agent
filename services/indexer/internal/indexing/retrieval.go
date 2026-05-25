package indexing

import (
	"context"
	"fmt"
)

func (s *Service) QueryContext(ctx context.Context, query ContextQuery) (*ContextResult, error) {
	query = normalizeContextQuery(query)
	if err := validateContextQuery(query); err != nil {
		return nil, err
	}
	if err := s.checkQueryVectorDimensions(len(query.Vector)); err != nil {
		return nil, err
	}
	chunks, err := s.store.VectorSearch(ctx, query.Vector, query.Limit, query.Filters)
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	if len(chunks) == 0 {
		return &ContextResult{}, nil
	}
	chunks = normalizeScores(chunks)
	graph, err := s.store.GetNeighborhood(ctx, chunks[0].ID, query.Depth)
	if err != nil {
		return nil, fmt.Errorf("graph traversal: %w", err)
	}
	return &ContextResult{
		ReasoningContext: ReasoningContext(chunks, graph),
		Citations:        Citations(chunks),
		Chunks:           cloneChunks(chunks),
		Graph:            cloneGraphFragment(graph),
		Package:          BuildContextPackage(chunks, graph),
	}, nil
}
