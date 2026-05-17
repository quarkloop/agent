package indexer

import "context"

type Chunk struct {
	ID                string            `json:"id,omitempty"`
	Text              string            `json:"text,omitempty"`
	Vector            []float32         `json:"vector,omitempty"`
	Metadata          map[string]string `json:"metadata,omitempty"`
	Document          Document          `json:"document,omitempty"`
	EmbeddingMetadata EmbeddingMetadata `json:"embedding_metadata,omitempty"`
	Facts             []Fact            `json:"facts,omitempty"`
	Citations         []Citation        `json:"citations,omitempty"`
	Provenance        Provenance        `json:"provenance,omitempty"`
	Score             float32           `json:"score,omitempty"`
}

type KnowledgeRecord struct {
	Chunk     Chunk      `json:"chunk,omitempty"`
	Entities  []Entity   `json:"entities,omitempty"`
	Relations []Relation `json:"relations,omitempty"`
}

type Document struct {
	ID        string            `json:"id,omitempty"`
	Name      string            `json:"name,omitempty"`
	Type      string            `json:"type,omitempty"`
	SourceURI string            `json:"source_uri,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type EmbeddingMetadata struct {
	Provider    string `json:"provider,omitempty"`
	Model       string `json:"model,omitempty"`
	Dimensions  int    `json:"dimensions,omitempty"`
	ContentHash string `json:"content_hash,omitempty"`
	Version     string `json:"version,omitempty"`
}

type Entity struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
	Type string `json:"type,omitempty"`
}

type Relation struct {
	FromID   string `json:"from_id,omitempty"`
	ToID     string `json:"to_id,omitempty"`
	Relation string `json:"relation,omitempty"`
}

type Fact struct {
	ID         string            `json:"id,omitempty"`
	Subject    string            `json:"subject,omitempty"`
	Predicate  string            `json:"predicate,omitempty"`
	Object     string            `json:"object,omitempty"`
	Confidence float32           `json:"confidence,omitempty"`
	Citations  []Citation        `json:"citations,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type Citation struct {
	ID          string  `json:"id,omitempty"`
	SourceURI   string  `json:"source_uri,omitempty"`
	ChunkID     string  `json:"chunk_id,omitempty"`
	TextSpan    string  `json:"text_span,omitempty"`
	StartOffset int     `json:"start_offset,omitempty"`
	EndOffset   int     `json:"end_offset,omitempty"`
	Confidence  float32 `json:"confidence,omitempty"`
}

type Provenance struct {
	SourceURI  string            `json:"source_uri,omitempty"`
	SourceHash string            `json:"source_hash,omitempty"`
	IngestedAt string            `json:"ingested_at,omitempty"`
	ProducedBy string            `json:"produced_by,omitempty"`
	TraceID    string            `json:"trace_id,omitempty"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type GraphNode struct {
	ID    string `json:"id,omitempty"`
	Label string `json:"label,omitempty"`
	Type  string `json:"type,omitempty"`
}

type GraphEdge struct {
	FromID   string `json:"from_id,omitempty"`
	ToID     string `json:"to_id,omitempty"`
	Relation string `json:"relation,omitempty"`
}

type GraphFragment struct {
	Nodes []GraphNode `json:"nodes,omitempty"`
	Edges []GraphEdge `json:"edges,omitempty"`
}

type ContextPackage struct {
	Chunks     []Chunk        `json:"chunks,omitempty"`
	Facts      []Fact         `json:"facts,omitempty"`
	Citations  []Citation     `json:"citations,omitempty"`
	Provenance []Provenance   `json:"provenance,omitempty"`
	Graph      *GraphFragment `json:"graph,omitempty"`
	Confidence float32        `json:"confidence,omitempty"`
}

// GraphVectorDriver is the storage seam for the indexer service. Implementors
// own persistence, vector search, graph writes, lifecycle, and concurrency.
type GraphVectorDriver interface {
	UpsertRecord(ctx context.Context, record KnowledgeRecord) error
	UpsertDocument(ctx context.Context, document Document) error
	UpsertEntity(ctx context.Context, entity Entity) error
	UpsertRelation(ctx context.Context, relation Relation, chunkID string) error
	UpsertFact(ctx context.Context, fact Fact, chunkID string) error
	UpsertCitation(ctx context.Context, citation Citation, chunkID string) error
	DeleteDocument(ctx context.Context, documentID string) error
	DeleteChunk(ctx context.Context, chunkID string) error
	VectorSearch(ctx context.Context, queryVector []float32, limit int, filters map[string]string) ([]Chunk, error)
	GetNeighborhood(ctx context.Context, nodeID string, depth int) (*GraphFragment, error)
	Ping(ctx context.Context) error
	Close() error
}
