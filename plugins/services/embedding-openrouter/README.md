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
