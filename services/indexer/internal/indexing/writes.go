package indexing

import (
	"context"
	"fmt"
	"strings"

	"github.com/quarkloop/services/indexer/pkg/indexer"
)

func (s *Service) UpsertChunk(ctx context.Context, cmd IndexCommand) error {
	cmd = normalizeIndexCommand(cmd)
	if err := validateIndexCommand(cmd); err != nil {
		return err
	}
	if err := s.rememberVectorDimensions(len(cmd.Vector)); err != nil {
		return err
	}
	if err := s.store.UpsertRecord(ctx, knowledgeRecord(cmd)); err != nil {
		return fmt.Errorf("upsert canonical index record: %w", err)
	}
	return nil
}

func (s *Service) UpsertDocument(ctx context.Context, document indexer.Document) error {
	document = normalizeDocument(document, nil, "")
	if document.ID == "" {
		return invalid("document.id", "is required")
	}
	if err := s.store.UpsertDocument(ctx, document); err != nil {
		return fmt.Errorf("upsert document: %w", err)
	}
	return nil
}

func (s *Service) UpsertEntity(ctx context.Context, entity indexer.Entity) error {
	entities := normalizeEntities([]indexer.Entity{entity})
	if len(entities) == 0 {
		return invalid("entity.id", "or entity.name is required")
	}
	if err := s.store.UpsertEntity(ctx, entities[0]); err != nil {
		return fmt.Errorf("upsert entity: %w", err)
	}
	return nil
}

func (s *Service) UpsertRelation(ctx context.Context, relation indexer.Relation, chunkID string) error {
	relations := normalizeRelations([]indexer.Relation{relation})
	if len(relations) == 0 {
		return invalid("relation", "requires from_id, to_id, and relation")
	}
	if err := s.store.UpsertRelation(ctx, relations[0], strings.TrimSpace(chunkID)); err != nil {
		return fmt.Errorf("upsert relation: %w", err)
	}
	return nil
}

func (s *Service) UpsertFact(ctx context.Context, fact indexer.Fact, chunkID string) error {
	facts := normalizeFacts([]indexer.Fact{fact}, strings.TrimSpace(chunkID), "")
	if len(facts) == 0 || facts[0].Subject == "" || facts[0].Predicate == "" || facts[0].Object == "" {
		return invalid("fact", "requires subject, predicate, and object")
	}
	if !validConfidence(facts[0].Confidence) {
		return invalid("fact.confidence", "must be between 0 and 1")
	}
	if err := s.store.UpsertFact(ctx, facts[0], strings.TrimSpace(chunkID)); err != nil {
		return fmt.Errorf("upsert fact: %w", err)
	}
	return nil
}

func (s *Service) UpsertCitation(ctx context.Context, citation indexer.Citation, chunkID string) error {
	citations := normalizeCitations([]indexer.Citation{citation}, strings.TrimSpace(chunkID), citation.SourceURI)
	if len(citations) == 0 {
		return invalid("citation", "requires source_uri, chunk_id, or text_span")
	}
	citation = citations[0]
	if citation.EndOffset > 0 && citation.StartOffset > citation.EndOffset {
		return invalid("citation.offsets", "start_offset must be less than or equal to end_offset")
	}
	if !validConfidence(citation.Confidence) {
		return invalid("citation.confidence", "must be between 0 and 1")
	}
	if err := s.store.UpsertCitation(ctx, citation, strings.TrimSpace(chunkID)); err != nil {
		return fmt.Errorf("upsert citation: %w", err)
	}
	return nil
}

func (s *Service) DeleteDocument(ctx context.Context, documentID string) error {
	documentID = strings.TrimSpace(documentID)
	if documentID == "" {
		return invalid("document_id", "is required")
	}
	if err := s.store.DeleteDocument(ctx, documentID); err != nil {
		return fmt.Errorf("delete document: %w", err)
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
