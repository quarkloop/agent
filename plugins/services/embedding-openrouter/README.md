# OpenRouter Embedding Service Plugin

The OpenRouter embedding service calls a configured OpenRouter embedding model
and returns vector metadata using the same gRPC contract as the local embedding
service.

## Service Functions

| Function | RPC method | Risk | Approval | Idempotent | Purpose |
| --- | --- | --- | --- | --- | --- |
| `embedding_Embed` | `quark.embedding.v1.EmbeddingService/Embed` | read | no | yes | Create an OpenRouter-backed embedding for supplied text and return provider/model/dimension metadata. |

Provider credentials are resolved through environment/configuration outside the
service function payload. Runtime should record usage once model-service
accounting is introduced.

## Health And Readiness

- Health protocol: gRPC health v1.
- Health service: `quark.embedding.v1.EmbeddingService`.
- Required readiness: yes, before runtime receives the service catalog.
- Minimum descriptor version: `1.0.0`.
- Startup diagnostics cover missing `QUARK_EMBEDDING_ADDR`, failed health
  checks, descriptor version mismatch, and missing RPC descriptors.
