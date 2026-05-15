package dgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/quarkloop/services/indexer/pkg/indexer"
)

func (d *Driver) UpsertRecord(ctx context.Context, record indexer.KnowledgeRecord) error {
	chunk := record.Chunk
	if err := d.ensureMetadataPredicates(ctx, chunk.Metadata); err != nil {
		return err
	}
	metaJSON, err := json.Marshal(chunk.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	canonicalJSON, err := json.Marshal(canonicalChunk{
		Document:          chunk.Document,
		EmbeddingMetadata: chunk.EmbeddingMetadata,
		Facts:             chunk.Facts,
		Citations:         chunk.Citations,
		Provenance:        chunk.Provenance,
	})
	if err != nil {
		return fmt.Errorf("marshal canonical chunk: %w", err)
	}

	query, vars := recordUpsertQuery(record)
	nquads := []byte(recordMutationNQuads(record, string(metaJSON), string(canonicalJSON)))
	return d.doMutation(ctx, fmt.Sprintf("upsert canonical record %s", chunk.ID), func() *api.Request {
		return &api.Request{
			Query: query,
			Vars:  vars,
			Mutations: []*api.Mutation{{
				SetNquads: nquads,
			}},
			CommitNow: true,
		}
	})
}

func (d *Driver) DeleteChunk(ctx context.Context, chunkID string) error {
	if chunkID == "" {
		return nil
	}
	return d.doMutation(ctx, fmt.Sprintf("delete chunk %s", chunkID), func() *api.Request {
		return &api.Request{
			Query: `query chunk($id: string) { c as var(func: eq(quark.chunk_id, $id)) }`,
			Vars:  map[string]string{"$id": chunkID},
			Mutations: []*api.Mutation{{
				DelNquads: []byte("uid(c) * * .\n"),
			}},
			CommitNow: true,
		}
	})
}

func recordUpsertQuery(record indexer.KnowledgeRecord) (string, map[string]string) {
	var query strings.Builder
	vars := map[string]string{"$chunk": record.Chunk.ID}
	query.WriteString("query record($chunk: string")
	for i, entity := range record.Entities {
		name := fmt.Sprintf("$entity%d", i)
		vars[name] = entity.ID
		fmt.Fprintf(&query, ", %s: string", name)
	}
	for i, relation := range record.Relations {
		name := fmt.Sprintf("$relation%d", i)
		vars[name] = relationID(relation)
		fmt.Fprintf(&query, ", %s: string", name)
	}
	query.WriteString(`) {
  c as var(func: eq(quark.chunk_id, $chunk))
`)
	for i := range record.Entities {
		fmt.Fprintf(&query, "  e%d as var(func: eq(quark.entity_id, $entity%d))\n", i, i)
	}
	for i := range record.Relations {
		fmt.Fprintf(&query, "  r%d as var(func: eq(quark.relation_id, $relation%d))\n", i, i)
	}
	query.WriteString("}")
	return query.String(), vars
}

func recordMutationNQuads(record indexer.KnowledgeRecord, metaJSON, canonicalJSON string) string {
	var nquads strings.Builder
	nquads.WriteString(chunkMutationNQuads(record.Chunk, metaJSON, canonicalJSON))
	entityVars := make(map[string]string, len(record.Entities))
	for i, entity := range record.Entities {
		entity = normalizeEntity(entity)
		if entity.ID == "" {
			continue
		}
		varName := fmt.Sprintf("e%d", i)
		entityVars[entity.ID] = varName
		nquads.WriteString(entityMutationNQuads(varName, entity))
		fmt.Fprintf(&nquads, "uid(c) <quark.chunk_entity> uid(%s) .\n", varName)
	}
	for i, relation := range record.Relations {
		fromVar := entityVars[relation.FromID]
		toVar := entityVars[relation.ToID]
		if fromVar == "" || toVar == "" {
			continue
		}
		nquads.WriteString(relationMutationNQuads(fmt.Sprintf("r%d", i), relationID(relation), relation.Relation, fromVar, toVar))
	}
	return nquads.String()
}

func relationID(relation indexer.Relation) string {
	return relation.FromID + "|" + relation.Relation + "|" + relation.ToID
}

func normalizeEntity(entity indexer.Entity) indexer.Entity {
	if entity.ID == "" {
		entity.ID = indexer.EntityIDFromName(entity.Name)
	}
	if entity.Name == "" {
		entity.Name = entity.ID
	}
	if entity.Type == "" {
		entity.Type = "UNKNOWN"
	}
	return entity
}

type canonicalChunk struct {
	Document          indexer.Document          `json:"document,omitempty"`
	EmbeddingMetadata indexer.EmbeddingMetadata `json:"embedding_metadata,omitempty"`
	Facts             []indexer.Fact            `json:"facts,omitempty"`
	Citations         []indexer.Citation        `json:"citations,omitempty"`
	Provenance        indexer.Provenance        `json:"provenance,omitempty"`
}

func chunkMutationNQuads(chunk indexer.Chunk, metaJSON, canonicalJSON string) string {
	var nquads strings.Builder
	fmt.Fprintf(&nquads, `uid(c) <dgraph.type> "QuarkChunk" .`+"\n")
	fmt.Fprintf(&nquads, "uid(c) <quark.chunk_id> %s .\n", quote(chunk.ID))
	fmt.Fprintf(&nquads, "uid(c) <quark.text_content> %s .\n", quote(chunk.Text))
	fmt.Fprintf(&nquads, "uid(c) <quark.embedding> %s .\n", quote(vectorLiteral(chunk.Vector)))
	fmt.Fprintf(&nquads, "uid(c) <quark.metadata_json> %s .\n", quote(metaJSON))
	fmt.Fprintf(&nquads, "uid(c) <quark.canonical_json> %s .\n", quote(canonicalJSON))
	for key, value := range chunk.Metadata {
		fmt.Fprintf(&nquads, "uid(c) <%s> %s .\n", metadataPredicate(key), quote(value))
	}
	return nquads.String()
}

func entityMutationNQuads(uidVar string, entity indexer.Entity) string {
	return fmt.Sprintf(`uid(%s) <dgraph.type> "QuarkEntity" .
uid(%s) <quark.entity_id> %s .
uid(%s) <quark.entity_name> %s .
uid(%s) <quark.entity_type> %s .
`, uidVar, uidVar, quote(entity.ID), uidVar, quote(entity.Name), uidVar, quote(entity.Type))
}

func relationMutationNQuads(uidVar, relationID, name, fromVar, toVar string) string {
	return fmt.Sprintf(`uid(%s) <dgraph.type> "QuarkRelation" .
uid(%s) <quark.relation_id> %s .
uid(%s) <quark.relation_name> %s .
uid(%s) <quark.relation_from> uid(%s) .
uid(%s) <quark.relation_to> uid(%s) .
`, uidVar, uidVar, quote(relationID), uidVar, quote(name), uidVar, fromVar, uidVar, toVar)
}
