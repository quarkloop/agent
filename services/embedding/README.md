# Embedding Service

`services/embedding` provides deterministic local embeddings for tests and
offline development, plus an OpenRouter-backed mode behind the same gRPC
contract. It owns vector generation only; indexing and retrieval stay in the
agent/indexer path.

## Service Functions

| Function | RPC method | Request | Response | Purpose |
| --- | --- | --- | --- | --- |
| `embedding_Embed` | `quark.embedding.v1.EmbeddingService/Embed` | `EmbedRequest` | `EmbedResponse` | Convert input text into a vector and return provider, model, dimensions, and content hash metadata. |

## Ownership Boundaries

- The agent decides which chunks or user queries need embeddings.
- The embedding service generates vectors and validates provider/model/dimension
  behavior.
- Runtime may pass embedding references between service functions so large
  vectors do not need to move through prompts.
- The embedding service does not index, retrieve, parse documents, or call the
  indexer service.

## Configuration

- `--addr`: gRPC listen address, default `127.0.0.1:7304`.
- `--provider`: `local` or `openrouter`, default `local`.
- `--model`: provider model name.
- `--dimensions`: expected embedding dimensions.
- `--openrouter-base-url`: OpenRouter API base URL.
- `OPENROUTER_API_KEY`: required for `openrouter` provider mode.
- `--skill-dir`: directory containing the service plugin `SKILL.md`.

## Health And Readiness

- Health protocol: gRPC health v1.
- Health service: `quark.embedding.v1.EmbeddingService`.
- Descriptor registry: `quark.service.v1.ServiceRegistry`.
- Local mode is deterministic and has no external dependency.
- OpenRouter mode requires configured API key, model, and compatible dimensions.

## Audit Notes

- Provider selection is explicit at service startup.
- Online provider errors are still plain wrapped errors; Task 22 will replace
  brittle provider string handling with structured categories and fallback
  diagnostics.
- Task 17 will decide whether this service remains a compatibility service or
  becomes a local provider adapter under the future model service.
