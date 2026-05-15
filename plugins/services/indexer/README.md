# Indexer Service Plugin

The indexer service stores and retrieves agent-produced Knowledge records. It
does not parse documents, call LLMs, create embeddings, or call another service.
The agent supplies chunks, facts, entities, relations, citations, embedding
metadata, and provenance after semantic extraction in the runtime/model path.

## Service Functions

| Function | RPC method | Risk | Approval | Idempotent | Purpose |
| --- | --- | --- | --- | --- | --- |
| `indexer_IndexDocument` | `quark.indexer.v1.IndexerService/IndexDocument` | write | no | yes | Persist one canonical index record with document, chunk, embedding metadata, facts, entities, relations, citations, and provenance. |
| `indexer_GetContext` | `quark.indexer.v1.IndexerService/GetContext` | read | no | yes | Retrieve vector and graph context for an agent-provided query embedding. |
| `indexer_DeleteChunk` | `quark.indexer.v1.IndexerService/DeleteChunk` | admin | yes | no | Delete one indexed chunk and its chunk-owned relation nodes by canonical chunk ID. |

Runtime exposes these functions through the shared tool-call path using the
supervisor-resolved service catalog.

## Health And Readiness

- Health protocol: gRPC health v1.
- Health service: `quark.indexer.v1.IndexerService`.
- Required readiness: yes, before runtime receives the service catalog.
- Minimum descriptor version: `1.0.0`.
- Startup diagnostics cover missing `QUARK_INDEXER_ADDR`, failed health checks,
  descriptor version mismatch, and missing RPC descriptors.
