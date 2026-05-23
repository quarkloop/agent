# Embedding Service Plugin

The local embedding service creates deterministic text embeddings for tests and
offline development. It owns embedding generation only; it does not index data
or decide document structure.

This plugin is the compatibility service-function surface for local
`embedding_Embed` flows. Future provider-backed embedding functions should be
implemented through the Quark Model Service boundary while preserving this
contract for deterministic local runs.

## Service Functions

| Function | RPC method | Risk | Approval | Idempotent | Purpose |
| --- | --- | --- | --- | --- | --- |
| `embedding_Embed` | `quark.embedding.v1.EmbeddingService/Embed` | read | no | yes | Create a deterministic local embedding for supplied text and return provider/model/dimension/content-hash metadata. |

Runtime stores large embedding vectors as references where needed so agents do
not copy vector payloads through prompts.

## Contract

Every response carries:

- `provider`: `local`
- `model`: configured local model name, default `local-hash-v1`
- `dimensions`: actual vector length
- `contentHash`: SHA-256 hash of the input text

The local provider is deterministic and has no external dependency. It is the
fallback provider used by local E2E flows and reproducible development tests.

## Health And Readiness

- Health protocol: NATS service-function readiness.
- Health service: `quark.embedding.v1.EmbeddingService`.
- Required readiness: yes, before runtime receives the service catalog.
- Minimum descriptor version: `1.0.0`.
- Startup diagnostics cover missing NATS endpoints, failed service-function
  readiness, descriptor version mismatch, and missing RPC descriptors.
