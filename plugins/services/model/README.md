# Model Service Plugin

The model service plugin declares the agent-facing model service functions for
generation, streaming generation, embeddings, reranking, token counting, model
listing, health, fallback, and usage accounting.

## Service Functions

| Function | RPC method | Risk | Approval | Streaming | Purpose |
| --- | --- | --- | --- | --- | --- |
| `model_Generate` | `quark.model.v1.ModelService/Generate` | read | no | no | Run one generation request and return text, tool calls, and usage. |
| `model_StreamGenerate` | `quark.model.v1.ModelService/StreamGenerate` | read | no | yes | Stream generation deltas/tool calls and final usage. |
| `model_Embed` | `quark.model.v1.ModelService/Embed` | read | no | no | Create embeddings through provider adapters. |
| `model_Rerank` | `quark.model.v1.ModelService/Rerank` | read | no | no | Rerank candidate documents for a query. |
| `model_CountTokens` | `quark.model.v1.ModelService/CountTokens` | read | no | no | Count or estimate model prompt/tool tokens. |
| `model_ListModels` | `quark.model.v1.ModelService/ListModels` | read | no | no | List provider models visible through the service. |
| `model_ProviderHealth` | `quark.model.v1.ModelService/ProviderHealth` | read | no | no | Report provider adapter readiness. |

## Provider Adapters

Provider plugins are adapters behind this service boundary. In-tree adapters:

- OpenRouter
- OpenAI
- Anthropic

The model service boundary is generic for future providers, including Zhipu,
once a provider plugin exists.

## Usage Fields

Every response carries redacted usage:

- `provider`
- `model`
- `inputTokens`
- `outputTokens`
- `reasoningTokens`
- `cachedTokens`
- `embeddingTokens`
- `latencyMillis`
- `costEstimate`
- `fallbackChain`
- `requestId`
- `finishReason`

Usage never contains prompts, response text, tool arguments, API keys, or
provider credentials.

## Persistence Boundary

The model service emits usage to runtime. Runtime accumulates usage per session
or run and records it in the activity/Core persistence path. The model service
must not call space, Core, indexer, embedding, or any other service.

## Health And Readiness

- Health protocol: gRPC health v1.
- Health service: `quark.model.v1.ModelService`.
- Descriptor registry: `quark.service.v1.ServiceRegistry`.
- Readiness is required when the model service plugin is installed. Runtime
  still routes provider calls through the model-service boundary package while
  standalone model-service process work continues.
