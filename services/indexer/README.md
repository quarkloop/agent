# Indexer Service

`services/indexer` is Quark Knowledge's canonical storage and retrieval service.
It stores agent-produced chunks, facts, entities, relations, citations,
embedding metadata, and provenance. It does not parse raw documents, call LLMs,
create embeddings, or call other services.

## Service Functions

| Function | RPC method | Request | Response | Purpose |
| --- | --- | --- | --- | --- |
| `indexer_UpsertDocument` | `quark.indexer.v1.IndexerService/UpsertDocument` | `UpsertDocumentRequest` | `IndexStatus` | Persist one canonical source document record. |
| `indexer_UpsertChunk` | `quark.indexer.v1.IndexerService/UpsertChunk` | `UpsertChunkRequest` | `IndexStatus` | Persist one canonical knowledge chunk with text, embedding vector or runtime embedding reference, graph data, facts, citations, and provenance. |
| `indexer_UpsertFact` | `quark.indexer.v1.IndexerService/UpsertFact` | `UpsertFactRequest` | `IndexStatus` | Persist one canonical fact record. |
| `indexer_UpsertEntity` | `quark.indexer.v1.IndexerService/UpsertEntity` | `UpsertEntityRequest` | `IndexStatus` | Persist one canonical entity record. |
| `indexer_UpsertRelation` | `quark.indexer.v1.IndexerService/UpsertRelation` | `UpsertRelationRequest` | `IndexStatus` | Persist one canonical relation record. |
| `indexer_UpsertCitation` | `quark.indexer.v1.IndexerService/UpsertCitation` | `UpsertCitationRequest` | `IndexStatus` | Persist one canonical citation record. |
| `indexer_QueryContext` | `quark.indexer.v1.IndexerService/QueryContext` | `QueryRequest` | `ContextResponse` | Return vector and graph context packages for an agent-provided query embedding. |
| `indexer_IndexDocument` | `quark.indexer.v1.IndexerService/IndexDocument` | `IndexRequest` | `IndexStatus` | Compatibility alias for the canonical chunk upsert path. |
| `indexer_GetContext` | `quark.indexer.v1.IndexerService/GetContext` | `QueryRequest` | `ContextResponse` | Compatibility alias for `QueryContext`. |
| `indexer_DeleteDocument` | `quark.indexer.v1.IndexerService/DeleteDocument` | `DeleteDocumentRequest` | `DeleteDocumentResponse` | Delete one canonical document and document-owned chunks by document ID. |
| `indexer_DeleteChunk` | `quark.indexer.v1.IndexerService/DeleteChunk` | `DeleteChunkRequest` | `DeleteChunkResponse` | Delete one canonical chunk and chunk-owned graph edges by chunk ID. |

## Ownership Boundaries

- The agent owns document reading, semantic extraction, schema inference, and
  deciding what knowledge should be indexed.
- The embedding service owns embedding generation.
- The indexer owns canonical storage, vector search, graph linking, filtering,
  context package construction, and storage-level validation.
- The service API layer maps protobuf DTOs into indexer domain commands before
  calling indexing logic.
- `UpsertDocument` stores source document records, while `UpsertChunk` stores
  searchable chunk records and their canonical evidence. They are storage
  functions, not an ingestion pipeline. Re-indexing the same chunk replaces
  prior chunk-owned metadata predicates, document links, entity links, and
  relation nodes before writing the new canonical record.
- `IndexDocument` remains only as a compatibility alias for the canonical chunk
  upsert path.
- `DeleteDocument` deletes the document, linked chunks, and chunk-owned
  relation nodes. Shared entity nodes remain because other chunks may still
  reference the same entities.
- `DeleteChunk` deletes the chunk and chunk-owned relation nodes. Shared entity
  nodes remain because other chunks may still reference the same entities.

## Configuration

- `--addr`: gRPC listen address, default `127.0.0.1:7301`.
- `--dgraph`: Dgraph Alpha gRPC address, default `127.0.0.1:9080`.
- `--skill-dir`: directory containing the service plugin `SKILL.md`.

## Health And Readiness

- Health protocol: gRPC health v1.
- Health service: `quark.indexer.v1.IndexerService`.
- Descriptor registry: `quark.service.v1.ServiceRegistry`.
- Required dependency: reachable Dgraph with the required schema installed.

## Audit Notes

- The service boundary already rejects raw PDF/document parsing and LLM work.
- Mapper tests cover protobuf-to-domain and domain-to-proto copying; storage
  tests cover canonical normalization, dimension validation, update cleanup,
  duplicate chunks, delete cleanup, graph/vector consistency, and owned copies.
