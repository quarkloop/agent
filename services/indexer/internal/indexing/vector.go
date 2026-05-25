package indexing

import "fmt"

const (
	defaultContextLimit = 5
	defaultGraphDepth   = 1
)

func normalizeContextQuery(query ContextQuery) ContextQuery {
	if query.Limit <= 0 {
		query.Limit = defaultContextLimit
	}
	if query.Depth <= 0 {
		query.Depth = defaultGraphDepth
	}
	query.Vector = cloneVector(query.Vector)
	query.Filters = cloneMetadata(query.Filters)
	return query
}

func validateContextQuery(query ContextQuery) error {
	if len(query.Vector) == 0 {
		return invalid("query_vector", "is required")
	}
	return nil
}

func (s *Service) rememberVectorDimensions(dimensions int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.vectorDimensions == 0 {
		s.vectorDimensions = dimensions
		return nil
	}
	if s.vectorDimensions != dimensions {
		return invalid("embedding", fmt.Sprintf("has %d dimensions, but this index uses %d-dimensional embeddings", dimensions, s.vectorDimensions))
	}
	return nil
}

func (s *Service) checkQueryVectorDimensions(dimensions int) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.vectorDimensions == 0 || s.vectorDimensions == dimensions {
		return nil
	}
	return invalid("query_vector", fmt.Sprintf("has %d dimensions, but this index uses %d-dimensional embeddings", dimensions, s.vectorDimensions))
}
