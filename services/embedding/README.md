# Embedding Service

`services/embedding` provides deterministic local embeddings for tests and
offline development, plus an OpenRouter-backed mode behind the same service-function
contract. It owns vector generation only; indexing and retrieval stay in the
agent/indexer path.

This service remains the compatibility embedding service for existing
`embedding_Embed` service-function flows. New model-provider work should live
behind the Quark Model Service boundary; this service can then become the local
embedding adapter without changing agent-facing contracts.

## Service Functions

| Function | RPC method | Request | Response | Purpose |
| --- | --- | --- | --- | --- |
| `embedding_Embed` | `quark.embedding.v1.EmbeddingService/Embed` | `EmbedRequest` | `EmbedResponse` | Convert input text into a vector and return provider, model, dimensions, and content-hash metadata. |

## Ownership Boundaries

- The agent decides which chunks or user queries need embeddings.
- The embedding service generates vectors and validates provider/model/dimension
  behavior.
- Runtime may pass embedding references between service functions so large
  vectors do not need to move through prompts.
- The embedding service does not index, retrieve, parse documents, or call the
  indexer service.
- The service does not call the model service. The agent/runtime chooses which
  embedding service function to call.

## Configuration

- `--nats-url`: NATS server URL used for service-function subjects.
- `--provider`: `local` or `openrouter`, default `local`.
- `--model`: provider model name.
- `--dimensions`: expected embedding dimensions.
- `--fallbacks`: ordered fallback providers in
  `provider|model|dimensions,provider|model|dimensions` format.
- `--openrouter-base-url`: OpenRouter API base URL.
- `OPENROUTER_API_KEY`: required for `openrouter` provider mode.
- `--skill-dir`: directory containing the service plugin `SKILL.md`.

Example OpenRouter primary with deterministic local fallback:

```bash
embedding-service \
  --provider openrouter \
  --model nvidia/llama-nemotron-embed-vl-1b-v2:free \
  --dimensions 2048 \
  --fallbacks local||32
```

The service records the actual provider, model, returned dimensions, and SHA-256
content hash on every response. Consumers must store those fields with indexed
chunks and use the same dimension shape for query embeddings.

## Provider Errors And Fallback

Provider errors are categorized before crossing the service boundary:

- `auth`: missing/invalid provider credentials or authorization failure.
- `quota`: provider rate limit or quota exhaustion.
- `model_unavailable`: configured model is missing or unavailable.
- `transport`: network or provider 5xx failure.
- `provider_response`: malformed or empty provider response.
- `dimension_mismatch`: configured/requested dimensions do not match the
  provider vector shape.
- `invalid_config`: unsupported provider or missing required provider model.
- `providers_exhausted`: every allowed fallback provider failed.

Fallback is explicit and ordered. Runtime quota/auth/model-unavailable/transport
failures can move to the next configured provider. Dimension mismatches and
invalid configuration are terminal because falling back would hide incompatible
vector shapes or a bad service configuration.

## Health And Readiness

- Health protocol: NATS service-function readiness.
- Health service: `quark.embedding.v1.EmbeddingService`.
- Descriptor registry: `quark.service.v1.ServiceRegistry`.
- Local mode is deterministic and has no external dependency.
- OpenRouter mode requires configured API key, model, and compatible dimensions.
