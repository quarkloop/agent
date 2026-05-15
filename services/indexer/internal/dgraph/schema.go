package dgraph

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dgraph-io/dgo/v250/protos/api"
)

const baseSchema = `
quark.chunk_id: string @index(exact) @upsert .
quark.text_content: string .
quark.embedding: float32vector @index(hnsw(metric:"cosine")) .
quark.metadata_json: string .
quark.canonical_json: string .
quark.entity_id: string @index(exact) @upsert .
quark.entity_name: string @index(term, exact) .
quark.entity_type: string @index(exact) .
quark.chunk_entity: [uid] @reverse .
quark.chunk_relation: [uid] @reverse .
quark.relation_id: string @index(exact) @upsert .
quark.relation_name: string @index(exact) .
quark.relation_from: uid @reverse .
quark.relation_to: uid @reverse .

type QuarkChunk {
	quark.chunk_id
	quark.text_content
	quark.embedding
	quark.metadata_json
	quark.canonical_json
	quark.chunk_entity
	quark.chunk_relation
}

type QuarkEntity {
	quark.entity_id
	quark.entity_name
	quark.entity_type
}

type QuarkRelation {
	quark.relation_id
	quark.relation_name
	quark.relation_from
	quark.relation_to
}
`

func (d *Driver) ensureBaseSchemaWithRetry(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last error
	for {
		if err := d.ensureBaseSchema(ctx); err != nil {
			last = err
		} else {
			return nil
		}
		if time.Now().After(deadline) {
			return last
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (d *Driver) ensureBaseSchema(ctx context.Context) error {
	if err := d.client.Alter(ctx, &api.Operation{Schema: baseSchema}); err != nil {
		return fmt.Errorf("ensure dgraph schema: %w", err)
	}
	return nil
}

func (d *Driver) ensureMetadataPredicates(ctx context.Context, meta map[string]string) error {
	if len(meta) == 0 {
		return nil
	}
	d.metaMu.Lock()
	defer d.metaMu.Unlock()

	var schema strings.Builder
	for key := range meta {
		predicate := metadataPredicate(key)
		if _, ok := d.metaPredicate[key]; ok {
			continue
		}
		fmt.Fprintf(&schema, "%s: string @index(exact) .\n", predicate)
		d.metaPredicate[key] = predicate
	}
	if schema.Len() == 0 {
		return nil
	}
	if err := d.client.Alter(ctx, &api.Operation{Schema: schema.String()}); err != nil {
		return fmt.Errorf("ensure dgraph metadata schema: %w", err)
	}
	return nil
}
