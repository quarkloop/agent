package indexing

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/quarkloop/services/indexer/pkg/indexer"
)

const (
	defaultContextLimit = 5
	defaultGraphDepth   = 1
	unknownEntityType   = "UNKNOWN"
)

type Store interface {
	UpsertRecord(ctx context.Context, record indexer.KnowledgeRecord) error
	DeleteChunk(ctx context.Context, chunkID string) error
	VectorSearch(ctx context.Context, queryVector []float32, limit int, filters map[string]string) ([]indexer.Chunk, error)
	GetNeighborhood(ctx context.Context, nodeID string, depth int) (*indexer.GraphFragment, error)
}

type Service struct {
	store Store
}

func New(store Store) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("indexing store is required")
	}
	return &Service{store: store}, nil
}

func (s *Service) IndexDocument(ctx context.Context, cmd IndexCommand) error {
	cmd = normalizeIndexCommand(cmd)
	if err := validateIndexCommand(cmd); err != nil {
		return err
	}

	if err := s.store.UpsertRecord(ctx, knowledgeRecord(cmd)); err != nil {
		return fmt.Errorf("upsert canonical index record: %w", err)
	}
	return nil
}

func (s *Service) DeleteChunk(ctx context.Context, chunkID string) error {
	chunkID = strings.TrimSpace(chunkID)
	if chunkID == "" {
		return invalid("chunk_id", "is required")
	}
	if err := s.store.DeleteChunk(ctx, chunkID); err != nil {
		return fmt.Errorf("delete chunk: %w", err)
	}
	return nil
}

func (s *Service) GetContext(ctx context.Context, query ContextQuery) (*ContextResult, error) {
	query = normalizeContextQuery(query)
	if err := validateContextQuery(query); err != nil {
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

func knowledgeRecord(cmd IndexCommand) indexer.KnowledgeRecord {
	return indexer.KnowledgeRecord{
		Chunk: indexer.Chunk{
			ID:                cmd.ChunkID,
			Text:              cmd.Text,
			Vector:            cloneVector(cmd.Vector),
			Metadata:          cloneMetadata(cmd.Metadata),
			Document:          cloneDocument(cmd.Document),
			EmbeddingMetadata: cmd.EmbeddingMetadata,
			Facts:             cloneFacts(cmd.Facts),
			Citations:         cloneCitations(cmd.Citations),
			Provenance:        cloneProvenance(cmd.Provenance),
		},
		Entities:  primaryEntities(cmd),
		Relations: uniqueRelations(cmd.Relations),
	}
}

func normalizeIndexCommand(cmd IndexCommand) IndexCommand {
	cmd.ChunkID = strings.TrimSpace(cmd.ChunkID)
	cmd.Text = strings.TrimSpace(cmd.Text)
	cmd.Vector = cloneVector(cmd.Vector)
	cmd.Metadata = cloneMetadata(cmd.Metadata)
	cmd.Document = normalizeDocument(cmd.Document, cmd.Metadata, cmd.ChunkID)
	cmd.EmbeddingMetadata = normalizeEmbeddingMetadata(cmd.EmbeddingMetadata, cmd.Metadata, len(cmd.Vector))
	cmd.Entities = normalizeEntities(cmd.Entities)
	cmd.Relations = normalizeRelations(cmd.Relations)
	cmd.Provenance = normalizeProvenance(cmd.Provenance, cmd.Metadata, cmd.Document.SourceURI, cmd.Text)
	cmd.Citations = normalizeCitations(cmd.Citations, cmd.ChunkID, cmd.Provenance.SourceURI)
	cmd.Facts = normalizeFacts(cmd.Facts, cmd.ChunkID, cmd.Provenance.SourceURI)
	return cmd
}

func validateIndexCommand(cmd IndexCommand) error {
	if cmd.ChunkID == "" {
		return invalid("chunk_id", "is required")
	}
	if cmd.Text == "" {
		return invalid("text_content", "is required")
	}
	if len(cmd.Vector) == 0 {
		return invalid("embedding", "is required")
	}
	if cmd.EmbeddingMetadata.Dimensions > 0 && cmd.EmbeddingMetadata.Dimensions != len(cmd.Vector) {
		return invalid("embedding_metadata.dimensions", fmt.Sprintf("is %d but embedding has %d values", cmd.EmbeddingMetadata.Dimensions, len(cmd.Vector)))
	}
	for _, fact := range cmd.Facts {
		if !validConfidence(fact.Confidence) {
			return invalid("facts.confidence", "must be between 0 and 1")
		}
	}
	for _, citation := range cmd.Citations {
		if !validConfidence(citation.Confidence) {
			return invalid("citations.confidence", "must be between 0 and 1")
		}
		if citation.EndOffset > 0 && citation.StartOffset > citation.EndOffset {
			return invalid("citations.offsets", "start_offset must be less than or equal to end_offset")
		}
	}
	return nil
}

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

func normalizeEntities(entities []indexer.Entity) []indexer.Entity {
	out := make([]indexer.Entity, 0, len(entities))
	for _, entity := range entities {
		entity.ID = strings.TrimSpace(entity.ID)
		entity.Name = strings.TrimSpace(entity.Name)
		entity.Type = strings.TrimSpace(entity.Type)
		if entity.ID == "" {
			entity.ID = indexer.EntityIDFromName(entity.Name)
		}
		if entity.Name == "" {
			entity.Name = entity.ID
		}
		if entity.Type == "" {
			entity.Type = unknownEntityType
		}
		if entity.ID == "" {
			continue
		}
		out = append(out, entity)
	}
	return out
}

func normalizeRelations(relations []indexer.Relation) []indexer.Relation {
	out := make([]indexer.Relation, 0, len(relations))
	for _, relation := range relations {
		relation.FromID = strings.TrimSpace(relation.FromID)
		relation.ToID = strings.TrimSpace(relation.ToID)
		relation.Relation = strings.TrimSpace(relation.Relation)
		if relation.FromID == "" || relation.ToID == "" || relation.Relation == "" {
			continue
		}
		out = append(out, relation)
	}
	return out
}

func normalizeDocument(document indexer.Document, metadata map[string]string, chunkID string) indexer.Document {
	document.ID = strings.TrimSpace(document.ID)
	document.Name = strings.TrimSpace(document.Name)
	document.Type = strings.TrimSpace(document.Type)
	document.SourceURI = strings.TrimSpace(document.SourceURI)
	document.Metadata = cloneMetadata(document.Metadata)
	if document.SourceURI == "" {
		document.SourceURI = firstMetadata(metadata, "source_uri", "source", "path")
	}
	if document.Name == "" {
		document.Name = firstMetadata(metadata, "filename", "document_name", "source")
	}
	if document.Type == "" {
		document.Type = firstMetadata(metadata, "document_type", "type")
	}
	if document.ID == "" {
		document.ID = firstMetadata(metadata, "document_id")
	}
	if document.ID == "" {
		document.ID = indexer.EntityIDFromName(firstNonEmpty(document.SourceURI, document.Name, chunkID))
	}
	return document
}

func normalizeEmbeddingMetadata(embedding indexer.EmbeddingMetadata, metadata map[string]string, vectorLength int) indexer.EmbeddingMetadata {
	embedding.Provider = strings.TrimSpace(embedding.Provider)
	embedding.Model = strings.TrimSpace(embedding.Model)
	embedding.ContentHash = strings.TrimSpace(embedding.ContentHash)
	embedding.Version = strings.TrimSpace(embedding.Version)
	if embedding.Provider == "" {
		embedding.Provider = firstMetadata(metadata, "embedding_provider", "embeddingProvider")
	}
	if embedding.Model == "" {
		embedding.Model = firstMetadata(metadata, "embedding_model", "embeddingModel")
	}
	if embedding.ContentHash == "" {
		embedding.ContentHash = firstMetadata(metadata, "embedding_content_hash", "embeddingContentHash", "content_hash", "contentHash")
	}
	if embedding.Version == "" {
		embedding.Version = firstMetadata(metadata, "embedding_version", "embeddingVersion")
	}
	if embedding.Dimensions <= 0 {
		if parsed, ok := parsePositiveInt(firstMetadata(metadata, "embedding_dimensions", "embeddingDimensions", "dimensions")); ok {
			embedding.Dimensions = parsed
		}
	}
	if embedding.Dimensions <= 0 {
		embedding.Dimensions = vectorLength
	}
	return embedding
}

func normalizeProvenance(provenance indexer.Provenance, metadata map[string]string, sourceURI, sourceText string) indexer.Provenance {
	provenance.SourceURI = strings.TrimSpace(provenance.SourceURI)
	provenance.SourceHash = strings.TrimSpace(provenance.SourceHash)
	provenance.IngestedAt = strings.TrimSpace(provenance.IngestedAt)
	provenance.ProducedBy = strings.TrimSpace(provenance.ProducedBy)
	provenance.TraceID = strings.TrimSpace(provenance.TraceID)
	provenance.Metadata = cloneMetadata(provenance.Metadata)
	if provenance.SourceURI == "" {
		provenance.SourceURI = firstNonEmpty(sourceURI, firstMetadata(metadata, "source_uri", "source", "path"))
	}
	if provenance.SourceHash == "" {
		provenance.SourceHash = firstMetadata(metadata, "source_hash", "content_hash")
	}
	if provenance.SourceHash == "" {
		provenance.SourceHash = indexer.SourceHashFromText(sourceText)
	}
	if provenance.TraceID == "" {
		provenance.TraceID = firstMetadata(metadata, "trace_id", "session_id")
	}
	return provenance
}

func normalizeCitations(citations []indexer.Citation, chunkID, sourceURI string) []indexer.Citation {
	out := make([]indexer.Citation, 0, len(citations)+1)
	for _, citation := range citations {
		citation.ID = strings.TrimSpace(citation.ID)
		citation.SourceURI = strings.TrimSpace(citation.SourceURI)
		citation.ChunkID = strings.TrimSpace(citation.ChunkID)
		citation.TextSpan = strings.TrimSpace(citation.TextSpan)
		if citation.ChunkID == "" {
			citation.ChunkID = chunkID
		}
		if citation.SourceURI == "" {
			citation.SourceURI = sourceURI
		}
		if citation.ID == "" {
			citation.ID = indexer.EntityIDFromName(firstNonEmpty(citation.SourceURI, citation.ChunkID, citation.TextSpan))
		}
		if citation.Confidence == 0 {
			citation.Confidence = 1
		}
		if citation.SourceURI == "" && citation.ChunkID == "" && citation.TextSpan == "" {
			continue
		}
		out = append(out, citation)
	}
	if len(out) == 0 && sourceURI != "" {
		out = append(out, indexer.Citation{
			ID:         indexer.EntityIDFromName(sourceURI + "#" + chunkID),
			SourceURI:  sourceURI,
			ChunkID:    chunkID,
			Confidence: 1,
		})
	}
	return out
}

func normalizeFacts(facts []indexer.Fact, chunkID, sourceURI string) []indexer.Fact {
	out := make([]indexer.Fact, 0, len(facts))
	for _, fact := range facts {
		fact.ID = strings.TrimSpace(fact.ID)
		fact.Subject = strings.TrimSpace(fact.Subject)
		fact.Predicate = strings.TrimSpace(fact.Predicate)
		fact.Object = strings.TrimSpace(fact.Object)
		fact.Metadata = cloneMetadata(fact.Metadata)
		if fact.Subject == "" || fact.Predicate == "" || fact.Object == "" {
			continue
		}
		if fact.ID == "" {
			fact.ID = indexer.EntityIDFromName(fact.Subject + "|" + fact.Predicate + "|" + fact.Object)
		}
		if fact.Confidence == 0 {
			fact.Confidence = 1
		}
		fact.Citations = normalizeCitations(fact.Citations, chunkID, sourceURI)
		out = append(out, fact)
	}
	return out
}

func relationEndpoints(relation indexer.Relation) []indexer.Entity {
	if !completeRelation(relation) {
		return nil
	}
	return []indexer.Entity{
		{ID: relation.FromID, Name: relation.FromID, Type: unknownEntityType},
		{ID: relation.ToID, Name: relation.ToID, Type: unknownEntityType},
	}
}

func primaryEntities(cmd IndexCommand) []indexer.Entity {
	entities := make([]indexer.Entity, 0, len(cmd.Entities)+len(cmd.Relations)*2)
	seen := make(map[string]int)
	add := func(entity indexer.Entity) {
		entity = normalizeEntityForWrite(entity)
		if entity.ID == "" {
			return
		}
		if existing, ok := seen[entity.ID]; ok {
			if entities[existing].Type == unknownEntityType && entity.Type != unknownEntityType {
				entities[existing] = entity
			}
			return
		}
		seen[entity.ID] = len(entities)
		entities = append(entities, entity)
	}
	for _, entity := range cmd.Entities {
		add(entity)
	}
	for _, relation := range cmd.Relations {
		for _, endpoint := range relationEndpoints(relation) {
			add(endpoint)
		}
	}
	return entities
}

func linkedEntityIDs(entities []indexer.Entity) []string {
	out := make([]string, 0, len(entities))
	seen := make(map[string]struct{}, len(entities))
	for _, entity := range entities {
		entityID := normalizedEntityID(entity)
		if entityID == "" {
			continue
		}
		if _, ok := seen[entityID]; ok {
			continue
		}
		seen[entityID] = struct{}{}
		out = append(out, entityID)
	}
	return out
}

func uniqueRelations(relations []indexer.Relation) []indexer.Relation {
	out := make([]indexer.Relation, 0, len(relations))
	seen := make(map[string]struct{}, len(relations))
	for _, relation := range relations {
		if !completeRelation(relation) {
			continue
		}
		key := relation.FromID + "\x00" + relation.Relation + "\x00" + relation.ToID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, relation)
	}
	return out
}

func normalizeEntityForWrite(entity indexer.Entity) indexer.Entity {
	entity.ID = strings.TrimSpace(entity.ID)
	entity.Name = strings.TrimSpace(entity.Name)
	entity.Type = strings.TrimSpace(entity.Type)
	if entity.ID == "" {
		entity.ID = indexer.EntityIDFromName(entity.Name)
	}
	if entity.Name == "" {
		entity.Name = entity.ID
	}
	if entity.Type == "" {
		entity.Type = unknownEntityType
	}
	return entity
}

func completeRelation(relation indexer.Relation) bool {
	return relation.FromID != "" && relation.ToID != "" && relation.Relation != ""
}

func normalizedEntityID(entity indexer.Entity) string {
	if entity.ID != "" {
		return entity.ID
	}
	return indexer.EntityIDFromName(entity.Name)
}

func firstMetadata(metadata map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(metadata[key]); value != "" {
			return value
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parsePositiveInt(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return parsed, true
}

func validConfidence(value float32) bool {
	return value >= 0 && value <= 1
}

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
		out[i] = indexer.Chunk{
			ID:                chunk.ID,
			Text:              chunk.Text,
			Vector:            cloneVector(chunk.Vector),
			Metadata:          cloneMetadata(chunk.Metadata),
			Document:          cloneDocument(chunk.Document),
			EmbeddingMetadata: chunk.EmbeddingMetadata,
			Facts:             cloneFacts(chunk.Facts),
			Citations:         cloneCitations(chunk.Citations),
			Provenance:        cloneProvenance(chunk.Provenance),
			Score:             chunk.Score,
		}
	}
	return out
}

func cloneDocument(in indexer.Document) indexer.Document {
	in.Metadata = cloneMetadata(in.Metadata)
	return in
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
	in.Metadata = cloneMetadata(in.Metadata)
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

func cloneContextPackage(in indexer.ContextPackage) indexer.ContextPackage {
	return indexer.ContextPackage{
		Chunks:     cloneChunks(in.Chunks),
		Facts:      cloneFacts(in.Facts),
		Citations:  cloneCitations(in.Citations),
		Provenance: cloneProvenanceList(in.Provenance),
		Graph:      cloneGraphFragment(in.Graph),
		Confidence: in.Confidence,
	}
}

func cloneProvenanceList(in []indexer.Provenance) []indexer.Provenance {
	out := make([]indexer.Provenance, len(in))
	for i, provenance := range in {
		out[i] = cloneProvenance(provenance)
	}
	return out
}
