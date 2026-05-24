# OpenRouter Embedding Service Plugin

The OpenRouter embedding service calls a configured OpenRouter embedding model
and returns vector metadata using the same NATS service-function contract as the local embedding
service. It is a compatibility service plugin for existing embedding flows while
provider-backed embedding work migrates behind the Quark Model Service boundary.

## Service Functions

| Function | RPC method | Risk | Approval | Idempotent | Purpose |
| --- | --- | --- | --- | --- | --- |
| `embedding_Embed` | `quark.embedding.v1.EmbeddingService/Embed` | read | no | yes | Create an OpenRouter-backed embedding for supplied text and return provider/model/dimension/content-hash metadata. |

Provider credentials are resolved through environment/configuration outside the
service function payload. Runtime records model usage through the gateway-service
usage path when model calls are involved.

## Configuration

Typical OpenRouter embedding run:

```bash
embedding-service \
  --provider openrouter \
  --model nvidia/llama-nemotron-embed-vl-1b-v2:free \
  --dimensions 2048
```

The API key must be supplied through `OPENROUTER_API_KEY`.

Optional fallback is ordered and explicit:

```bash
embedding-service \
  --provider openrouter \
  --model nvidia/llama-nemotron-embed-vl-1b-v2:free \
  --dimensions 2048 \
  --fallbacks local||32
```

Every successful response returns `provider`, `model`, actual `dimensions`, and
`contentHash`. Store these fields with indexed records and use compatible
dimensions for query-time embeddings.

## Provider Errors

- `auth`: missing/invalid provider credentials or authorization failure.
- `quota`: provider rate limit or quota exhaustion.
- `model_unavailable`: configured model is missing or unavailable.
- `transport`: network or provider 5xx failure.
- `provider_response`: malformed or empty provider response.
- `dimension_mismatch`: configured/requested dimensions do not match the vector
  shape. This is terminal and does not fall back.
- `invalid_config`: unsupported provider or missing required model.
- `providers_exhausted`: every allowed fallback provider failed.

## Health And Readiness

- Health protocol: NATS service-function readiness.
- Health service: `quark.embedding.v1.EmbeddingService`.
- Required readiness: yes, before runtime receives the service catalog.
- Minimum descriptor version: `1.0.0`.
- Startup diagnostics cover missing `QUARK_EMBEDDING_ADDR`, failed health
  checks, descriptor version mismatch, and missing RPC descriptors.
