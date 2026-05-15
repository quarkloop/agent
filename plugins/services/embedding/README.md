# Embedding Service Plugin

The local embedding service creates deterministic text embeddings for tests and
offline development. It owns embedding generation only; it does not index data
or decide document structure.

## Service Functions

| Function | RPC method | Risk | Approval | Idempotent | Purpose |
| --- | --- | --- | --- | --- | --- |
| `embedding_Embed` | `quark.embedding.v1.EmbeddingService/Embed` | read | no | yes | Create a deterministic local embedding for supplied text and return provider/model/dimension metadata. |

Runtime stores large embedding vectors as references where needed so agents do
not copy vector payloads through prompts.

## Health And Readiness

- Health protocol: gRPC health v1.
- Health service: `quark.embedding.v1.EmbeddingService`.
- Required readiness: yes, before runtime receives the service catalog.
- Minimum descriptor version: `1.0.0`.
- Startup diagnostics cover missing `QUARK_EMBEDDING_ADDR`, failed health
  checks, descriptor version mismatch, and missing RPC descriptors.
