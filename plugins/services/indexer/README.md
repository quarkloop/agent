# Indexer Service Plugin

The indexer service stores and retrieves agent-produced Knowledge records. It
does not parse documents, call LLMs, create embeddings, or call another service.
The agent supplies chunks, facts, entities, relations, citations, embedding
metadata, and provenance after semantic extraction in the runtime/model path.

## Service Functions

| Function | RPC method | Risk | Approval | Idempotent | Purpose |
| --- | --- | --- | --- | --- | --- |
| `indexer_UpsertDocument` | `quark.indexer.v1.IndexerService/UpsertDocument` | write | no | yes | Persist an explicit metadata-only source document update; routine indexing persists nested document metadata with chunks. |
| `indexer_UpsertChunk` | `quark.indexer.v1.IndexerService/UpsertChunk` | write | no | yes | Persist one canonical chunk with embedding modality metadata, facts, entities, relations, citations, and provenance. |
| `indexer_UpsertFact` | `quark.indexer.v1.IndexerService/UpsertFact` | write | no | yes | Persist one canonical fact record. |
| `indexer_UpsertEntity` | `quark.indexer.v1.IndexerService/UpsertEntity` | write | no | yes | Persist one canonical entity record. |
| `indexer_UpsertRelation` | `quark.indexer.v1.IndexerService/UpsertRelation` | write | no | yes | Persist one canonical relation record. |
| `indexer_UpsertCitation` | `quark.indexer.v1.IndexerService/UpsertCitation` | write | no | yes | Persist one canonical citation record. |
| `indexer_QueryContext` | `quark.indexer.v1.IndexerService/QueryContext` | read | no | yes | Retrieve vector and graph context for an agent-provided query embedding. |
| `indexer_DeleteDocument` | `quark.indexer.v1.IndexerService/DeleteDocument` | admin | yes | no | Delete one indexed document and document-owned chunks by canonical document ID. |
| `indexer_DeleteChunk` | `quark.indexer.v1.IndexerService/DeleteChunk` | admin | yes | no | Delete one indexed chunk and its chunk-owned relation nodes by canonical chunk ID. |

Runtime exposes these functions through the shared tool-call path using the
supervisor-resolved service catalog.
Canonical request/reply subjects are owned by this plugin under
`svc.indexer.v1.*`, including `svc.indexer.v1.upsert_chunk` and
`svc.indexer.v1.query_context`.

## Health And Readiness

- Health protocol: NATS service-function readiness.
- Health service: `quark.indexer.v1.IndexerService`.
- Required readiness: yes, before runtime receives the service catalog.
- Minimum descriptor version: `1.0.0`.
- Startup diagnostics cover missing NATS endpoints, failed service-function
  readiness, descriptor version mismatch, and missing RPC descriptors.
