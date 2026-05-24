package dgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/quarkloop/services/indexer/pkg/indexer"
)

func (d *Driver) UpsertRecord(ctx context.Context, record indexer.KnowledgeRecord) error {
	chunk := record.Chunk
	metadataKeys, err := d.metadataKeysForUpsert(ctx, chunk.ID, chunk.Metadata)
	if err != nil {
		return err
	}
	if err := d.ensureMetadataPredicates(ctx, metadataKeys.asMap()); err != nil {
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
	cleanupNQuads := []byte(recordCleanupNQuads(metadataKeys))
	setNQuads := []byte(recordMutationNQuads(record, string(metaJSON), string(canonicalJSON)))
	return d.doMutation(ctx, fmt.Sprintf("upsert canonical record %s", chunk.ID), func() *api.Request {
		return &api.Request{
			Query: query,
			Vars:  vars,
			Mutations: []*api.Mutation{{
				DelNquads: cleanupNQuads,
			}, {
				SetNquads: setNQuads,
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
			Query: `query chunk($id: string) {
  c as var(func: eq(quark.chunk_id, $id)) {
    r as quark.chunk_relation
    f as ~quark.fact_chunk
    cite as ~quark.citation_chunk
  }
}`,
			Vars: map[string]string{"$id": chunkID},
			Mutations: []*api.Mutation{{
				DelNquads: []byte(deleteChunkNQuads()),
			}},
			CommitNow: true,
		}
	})
}

func recordUpsertQuery(record indexer.KnowledgeRecord) (string, map[string]string) {
	var query strings.Builder
	vars := map[string]string{"$chunk": record.Chunk.ID, "$document": record.Chunk.Document.ID}
	query.WriteString("query record($chunk: string, $document: string")
	for i, entity := range record.Entities {
		name := fmt.Sprintf("$entity%d", i)
		vars[name] = entity.ID
		fmt.Fprintf(&query, ", %s: string", name)
	}
	for i, relation := range record.Relations {
		name := fmt.Sprintf("$relation%d", i)
		vars[name] = relationID(record.Chunk.ID, relation)
		fmt.Fprintf(&query, ", %s: string", name)
	}
	query.WriteString(`) {
  c as var(func: eq(quark.chunk_id, $chunk)) {
    oldRelations as quark.chunk_relation
  }
  d as var(func: eq(quark.document_id, $document))
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

func recordCleanupNQuads(metadataKeys metadataKeySet) string {
	var nquads strings.Builder
	nquads.WriteString("uid(c) <quark.chunk_entity> * .\n")
	nquads.WriteString("uid(c) <quark.chunk_relation> * .\n")
	nquads.WriteString("uid(oldRelations) * * .\n")
	nquads.WriteString("uid(c) <quark.text_content> * .\n")
	nquads.WriteString("uid(c) <quark.embedding> * .\n")
	nquads.WriteString("uid(c) <quark.metadata_json> * .\n")
	nquads.WriteString("uid(c) <quark.canonical_json> * .\n")
	nquads.WriteString("uid(c) <quark.chunk_document> * .\n")
	for _, key := range metadataKeys.sorted() {
		fmt.Fprintf(&nquads, "uid(c) <%s> * .\n", metadataPredicate(key))
	}
	return nquads.String()
}

func deleteChunkNQuads() string {
	return "uid(c) <quark.chunk_entity> * .\n" +
		"uid(c) <quark.chunk_relation> * .\n" +
		"uid(r) * * .\n" +
		"uid(f) * * .\n" +
		"uid(cite) * * .\n" +
		"uid(c) * * .\n"
}

func recordMutationNQuads(record indexer.KnowledgeRecord, metaJSON, canonicalJSON string) string {
	var nquads strings.Builder
	if record.Chunk.Document.ID != "" {
		nquads.WriteString(documentMutationNQuads("d", record.Chunk.Document))
	}
	nquads.WriteString(chunkMutationNQuads(record.Chunk, metaJSON, canonicalJSON))
	if record.Chunk.Document.ID != "" {
		nquads.WriteString("uid(c) <quark.chunk_document> uid(d) .\n")
	}
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
		relationVar := fmt.Sprintf("r%d", i)
		nquads.WriteString(relationMutationNQuads(relationVar, relationID(record.Chunk.ID, relation), relation.Relation, fromVar, toVar))
		fmt.Fprintf(&nquads, "uid(c) <quark.chunk_relation> uid(%s) .\n", relationVar)
	}
	return nquads.String()
}

func relationID(chunkID string, relation indexer.Relation) string {
	return chunkID + "|" + relation.FromID + "|" + relation.Relation + "|" + relation.ToID
}

type metadataKeySet map[string]struct{}

func (s metadataKeySet) addMap(values map[string]string) {
	for key := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		s[key] = struct{}{}
	}
}

func (s metadataKeySet) asMap() map[string]string {
	out := make(map[string]string, len(s))
	for key := range s {
		out[key] = ""
	}
	return out
}

func (s metadataKeySet) sorted() []string {
	out := make([]string, 0, len(s))
	for key := range s {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func (d *Driver) metadataKeysForUpsert(ctx context.Context, chunkID string, next map[string]string) (metadataKeySet, error) {
	keys := make(metadataKeySet)
	keys.addMap(next)
	previous, err := d.metadataForChunk(ctx, chunkID)
	if err != nil {
		return nil, err
	}
	keys.addMap(previous)
	return keys, nil
}

func (d *Driver) metadataForChunk(ctx context.Context, chunkID string) (map[string]string, error) {
	if strings.TrimSpace(chunkID) == "" {
		return nil, nil
	}
	resp, err := d.client.NewReadOnlyTxn().QueryWithVars(ctx, chunkMetadataQuery, map[string]string{"$id": chunkID})
	if err != nil {
		return nil, fmt.Errorf("read existing chunk metadata: %w", err)
	}
	var payload struct {
		Chunks []struct {
			MetadataJSON string `json:"quark.metadata_json"`
		} `json:"chunks"`
	}
	if err := json.Unmarshal(resp.GetJson(), &payload); err != nil {
		return nil, fmt.Errorf("decode existing chunk metadata: %w", err)
	}
	if len(payload.Chunks) == 0 || payload.Chunks[0].MetadataJSON == "" {
		return nil, nil
	}
	metadata := map[string]string{}
	if err := json.Unmarshal([]byte(payload.Chunks[0].MetadataJSON), &metadata); err != nil {
		return nil, fmt.Errorf("decode existing metadata json: %w", err)
	}
	return metadata, nil
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

func documentMutationNQuads(uidVar string, document indexer.Document) string {
	metaJSON, _ := json.Marshal(document.Metadata)
	sourcesJSON, _ := json.Marshal(document.Sources)
	return fmt.Sprintf(`uid(%s) <dgraph.type> "QuarkDocument" .
uid(%s) <quark.document_id> %s .
uid(%s) <quark.document_name> %s .
uid(%s) <quark.document_type> %s .
uid(%s) <quark.document_source_uri> %s .
uid(%s) <quark.document_metadata_json> %s .
uid(%s) <quark.document_sources_json> %s .
`, uidVar, uidVar, quote(document.ID), uidVar, quote(document.Name), uidVar, quote(document.Type), uidVar, quote(document.SourceURI), uidVar, quote(string(metaJSON)), uidVar, quote(string(sourcesJSON)))
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
