# Indexer Service

`services/indexer` is Quark Knowledge's canonical storage and retrieval service.
It stores agent-produced chunks, facts, entities, relations, citations,
embedding metadata, and provenance. It does not parse raw documents, call LLMs,
create embeddings, or call other services.

## Service Functions

| Function | RPC method | Request | Response | Purpose |
| --- | --- | --- | --- | --- |
| `indexer_IndexDocument` | `quark.indexer.v1.IndexerService/IndexDocument` | `IndexRequest` | `IndexStatus` | Persist one canonical knowledge record with chunk text, embedding vector or runtime embedding reference, graph data, facts, citations, and provenance. |
| `indexer_GetContext` | `quark.indexer.v1.IndexerService/GetContext` | `QueryRequest` | `ContextResponse` | Return vector and graph context packages for an agent-provided query embedding. |

## Ownership Boundaries

- The agent owns document reading, semantic extraction, schema inference, and
  deciding what knowledge should be indexed.
- The embedding service owns embedding generation.
- The indexer owns canonical storage, vector search, graph linking, filtering,
  context package construction, and storage-level validation.
- The service API layer maps protobuf DTOs into indexer domain commands before
  calling indexing logic.

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
- Mapper tests cover protobuf-to-domain copying; storage tests cover canonical
  normalization, dimension validation, duplicate graph writes, and owned copies.
- Follow-up: Task 15 will decide whether `IndexDocument` should split into
  smaller document, chunk, fact, entity, relation, and citation functions.
