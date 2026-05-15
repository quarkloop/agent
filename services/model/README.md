# Model Service

`services/model` is the Quark model boundary. It centralizes model generation,
streaming generation, embedding, reranking, token counting, model listing,
provider health, provider fallback, and usage accounting.

Provider plugins are implementation adapters behind this service boundary. The
agent/runtime coordinates model calls and receives structured usage with every
response.

## Service Functions

| Function | RPC method | Request | Response | Purpose |
| --- | --- | --- | --- | --- |
| `model_Generate` | `quark.model.v1.ModelService/Generate` | `GenerateRequest` | `GenerateResponse` | Run one generation request and return text, tool calls, and usage. |
| `model_StreamGenerate` | `quark.model.v1.ModelService/StreamGenerate` | `StreamGenerateRequest` | `StreamGenerateResponse` stream | Stream generation deltas/tool calls and final usage. |
| `model_Embed` | `quark.model.v1.ModelService/Embed` | `EmbedRequest` | `EmbedResponse` | Create embeddings through provider adapters. |
| `model_Rerank` | `quark.model.v1.ModelService/Rerank` | `RerankRequest` | `RerankResponse` | Rerank candidate documents for a query. |
| `model_CountTokens` | `quark.model.v1.ModelService/CountTokens` | `CountTokensRequest` | `CountTokensResponse` | Count or estimate prompt/tool tokens without generation. |
| `model_ListModels` | `quark.model.v1.ModelService/ListModels` | `ListModelsRequest` | `ListModelsResponse` | List provider models visible through the service. |
| `model_ProviderHealth` | `quark.model.v1.ModelService/ProviderHealth` | `ProviderHealthRequest` | `ProviderHealthResponse` | Report provider adapter readiness, auth, and reachability. |

## Provider Adapters

Current in-tree provider plugins become model-service adapters:

- `openrouter`
- `openai`
- `anthropic`

The adapter contract is provider-agnostic so future providers such as `zhipu`
can be added without changing runtime inference code.

## Usage Fields

Every model response returns redacted usage:

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

Usage must never contain prompt text, response text, tool arguments, API keys,
headers, or provider credentials.

## Ownership Boundaries

- Model service owns provider dispatch, fallback chain recording, usage
  accounting, provider health, and provider diagnostics.
- Runtime owns session/run accumulation and persistence through activity/Core
  storage. Model service emits usage to runtime and never calls another service.
- Provider adapters own provider-specific HTTP wire formats and error mapping.
- Services such as indexer, document, ingestion, and citation do not call model
  service. The agent is the coordinator.
