package dgraph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dgraph-io/dgo/v250/protos/api"
	"github.com/quarkloop/services/indexer/pkg/indexer"
)

func (d *Driver) UpsertDocument(ctx context.Context, document indexer.Document) error {
	if strings.TrimSpace(document.ID) == "" {
		return nil
	}
	return d.doMutation(ctx, fmt.Sprintf("upsert document %s", document.ID), func() *api.Request {
		return &api.Request{
			Query: `query document($id: string) {
  d as var(func: eq(quark.document_id, $id))
}`,
			Vars: map[string]string{"$id": document.ID},
			Mutations: []*api.Mutation{{
				SetNquads: []byte(documentMutationNQuads("d", document)),
			}},
			CommitNow: true,
		}
	})
}

func (d *Driver) UpsertEntity(ctx context.Context, entity indexer.Entity) error {
	entity = normalizeEntity(entity)
	if strings.TrimSpace(entity.ID) == "" {
		return nil
	}
	return d.doMutation(ctx, fmt.Sprintf("upsert entity %s", entity.ID), func() *api.Request {
		return &api.Request{
			Query: `query entity($id: string) {
  e as var(func: eq(quark.entity_id, $id))
}`,
			Vars: map[string]string{"$id": entity.ID},
			Mutations: []*api.Mutation{{
				SetNquads: []byte(entityMutationNQuads("e", entity)),
			}},
			CommitNow: true,
		}
	})
}

func (d *Driver) UpsertRelation(ctx context.Context, relation indexer.Relation, chunkID string) error {
	if relation.FromID == "" || relation.ToID == "" || relation.Relation == "" {
		return nil
	}
	id := standaloneRelationID(relation)
	return d.doMutation(ctx, fmt.Sprintf("upsert relation %s", id), func() *api.Request {
		return &api.Request{
			Query: `query relation($id: string, $from: string, $to: string, $chunk: string) {
  r as var(func: eq(quark.relation_id, $id))
  from as var(func: eq(quark.entity_id, $from))
  to as var(func: eq(quark.entity_id, $to))
  c as var(func: eq(quark.chunk_id, $chunk))
}`,
			Vars: map[string]string{"$id": id, "$from": relation.FromID, "$to": relation.ToID, "$chunk": strings.TrimSpace(chunkID)},
			Mutations: []*api.Mutation{{
				SetNquads: []byte(
					entityMutationNQuads("from", indexer.Entity{ID: relation.FromID, Name: relation.FromID, Type: "UNKNOWN"}) +
						entityMutationNQuads("to", indexer.Entity{ID: relation.ToID, Name: relation.ToID, Type: "UNKNOWN"}) +
						relationMutationNQuads("r", id, relation.Relation, "from", "to") +
						relationChunkLinkNQuads(chunkID, "r"),
				),
			}},
			CommitNow: true,
		}
	})
}

func (d *Driver) UpsertFact(ctx context.Context, fact indexer.Fact, chunkID string) error {
	if fact.ID == "" {
		fact.ID = indexer.EntityIDFromName(fact.Subject + "|" + fact.Predicate + "|" + fact.Object)
	}
	if fact.ID == "" {
		return nil
	}
	metaJSON, err := json.Marshal(fact.Metadata)
	if err != nil {
		return fmt.Errorf("marshal fact metadata: %w", err)
	}
	return d.doMutation(ctx, fmt.Sprintf("upsert fact %s", fact.ID), func() *api.Request {
		return &api.Request{
			Query: `query fact($id: string, $chunk: string) {
  f as var(func: eq(quark.fact_id, $id))
  c as var(func: eq(quark.chunk_id, $chunk))
}`,
			Vars: map[string]string{"$id": fact.ID, "$chunk": strings.TrimSpace(chunkID)},
			Mutations: []*api.Mutation{{
				SetNquads: []byte(factMutationNQuads("f", fact, strings.TrimSpace(chunkID), string(metaJSON)) + factChunkLinkNQuads(chunkID, "f")),
			}},
			CommitNow: true,
		}
	})
}

func (d *Driver) UpsertCitation(ctx context.Context, citation indexer.Citation, chunkID string) error {
	if citation.ID == "" {
		citation.ID = indexer.EntityIDFromName(citation.SourceURI + "|" + citation.ChunkID + "|" + citation.TextSpan)
	}
	if citation.ID == "" {
		return nil
	}
	if citation.ChunkID == "" {
		citation.ChunkID = strings.TrimSpace(chunkID)
	}
	return d.doMutation(ctx, fmt.Sprintf("upsert citation %s", citation.ID), func() *api.Request {
		return &api.Request{
			Query: `query citation($id: string, $chunk: string) {
  cite as var(func: eq(quark.citation_id, $id))
  c as var(func: eq(quark.chunk_id, $chunk))
}`,
			Vars: map[string]string{"$id": citation.ID, "$chunk": citation.ChunkID},
			Mutations: []*api.Mutation{{
				SetNquads: []byte(citationMutationNQuads("cite", citation) + citationChunkLinkNQuads(citation.ChunkID, "cite")),
			}},
			CommitNow: true,
		}
	})
}

func (d *Driver) DeleteDocument(ctx context.Context, documentID string) error {
	if documentID == "" {
		return nil
	}
	return d.doMutation(ctx, fmt.Sprintf("delete document %s", documentID), func() *api.Request {
		return &api.Request{
			Query: `query document($id: string) {
  d as var(func: eq(quark.document_id, $id)) {
    linkedChunks as ~quark.chunk_document {
      r as quark.chunk_relation
      f as ~quark.fact_chunk
      cite as ~quark.citation_chunk
    }
  }
}`,
			Vars: map[string]string{"$id": documentID},
			Mutations: []*api.Mutation{{
				DelNquads: []byte("uid(linkedChunks) <quark.chunk_entity> * .\nuid(linkedChunks) <quark.chunk_relation> * .\nuid(r) * * .\nuid(f) * * .\nuid(cite) * * .\nuid(linkedChunks) * * .\nuid(d) * * .\n"),
			}},
			CommitNow: true,
		}
	})
}

func standaloneRelationID(relation indexer.Relation) string {
	return relation.FromID + "|" + relation.Relation + "|" + relation.ToID
}

func relationChunkLinkNQuads(chunkID, relationVar string) string {
	if strings.TrimSpace(chunkID) == "" {
		return ""
	}
	return fmt.Sprintf("uid(c) <quark.chunk_relation> uid(%s) .\n", relationVar)
}

func factChunkLinkNQuads(chunkID, factVar string) string {
	if strings.TrimSpace(chunkID) == "" {
		return ""
	}
	return fmt.Sprintf("uid(%s) <quark.fact_chunk> uid(c) .\n", factVar)
}

func citationChunkLinkNQuads(chunkID, citationVar string) string {
	if strings.TrimSpace(chunkID) == "" {
		return ""
	}
	return fmt.Sprintf("uid(%s) <quark.citation_chunk> uid(c) .\n", citationVar)
}

func factMutationNQuads(uidVar string, fact indexer.Fact, chunkID, metaJSON string) string {
	return fmt.Sprintf(`uid(%s) <dgraph.type> "QuarkFact" .
uid(%s) <quark.fact_id> %s .
uid(%s) <quark.fact_subject> %s .
uid(%s) <quark.fact_predicate> %s .
uid(%s) <quark.fact_object> %s .
uid(%s) <quark.fact_confidence> %f .
uid(%s) <quark.fact_chunk_id> %s .
uid(%s) <quark.fact_metadata_json> %s .
`, uidVar, uidVar, quote(fact.ID), uidVar, quote(fact.Subject), uidVar, quote(fact.Predicate), uidVar, quote(fact.Object), uidVar, fact.Confidence, uidVar, quote(chunkID), uidVar, quote(metaJSON))
}

func citationMutationNQuads(uidVar string, citation indexer.Citation) string {
	return fmt.Sprintf(`uid(%s) <dgraph.type> "QuarkCitation" .
uid(%s) <quark.citation_id> %s .
uid(%s) <quark.citation_source_uri> %s .
uid(%s) <quark.citation_chunk_id> %s .
uid(%s) <quark.citation_text_span> %s .
uid(%s) <quark.citation_start_offset> %d .
uid(%s) <quark.citation_end_offset> %d .
uid(%s) <quark.citation_confidence> %f .
`, uidVar, uidVar, quote(citation.ID), uidVar, quote(citation.SourceURI), uidVar, quote(citation.ChunkID), uidVar, quote(citation.TextSpan), uidVar, citation.StartOffset, uidVar, citation.EndOffset, uidVar, citation.Confidence)
}
