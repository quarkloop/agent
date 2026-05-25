package indexing

import (
	"context"
	"fmt"
	"sync"

	"github.com/quarkloop/services/indexer/pkg/indexer"
)

type Store interface {
	UpsertRecord(context.Context, indexer.KnowledgeRecord) error
	UpsertDocument(context.Context, indexer.Document) error
	UpsertEntity(context.Context, indexer.Entity) error
	UpsertRelation(context.Context, indexer.Relation, string) error
	UpsertFact(context.Context, indexer.Fact, string) error
	UpsertCitation(context.Context, indexer.Citation, string) error
	DeleteDocument(context.Context, string) error
	DeleteChunk(context.Context, string) error
	VectorSearch(context.Context, []float32, int, map[string]string) ([]indexer.Chunk, error)
	GetNeighborhood(context.Context, string, int) (*indexer.GraphFragment, error)
}

type Service struct {
	store            Store
	mu               sync.RWMutex
	vectorDimensions int
}

func New(store Store) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("indexing store is required")
	}
	return &Service{store: store}, nil
}
