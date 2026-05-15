# Indexer Service Plugin

The indexer service stores and retrieves agent-produced Knowledge records. It
does not parse documents, call LLMs, create embeddings, or call another service.

## Service Functions

| Function | RPC method | Risk | Approval | Idempotent | Purpose |
| --- | --- | --- | --- | --- | --- |
| `indexer_IndexDocument` | `quark.indexer.v1.IndexerService/IndexDocument` | write | no | yes | Persist one canonical index record with document, chunk, embedding metadata, facts, entities, relations, citations, and provenance. |
| `indexer_GetContext` | `quark.indexer.v1.IndexerService/GetContext` | read | no | yes | Retrieve vector and graph context for an agent-provided query embedding. |

Runtime exposes these functions through the shared tool-call path using the
supervisor-resolved service catalog.
