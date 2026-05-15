# service-model

The model service is the single boundary for LLM generation, embeddings,
reranking, token counting, model listing, provider health, fallback, and usage
accounting. Provider plugins are adapters behind this service boundary.

## Agent Rules

1. Use the runtime model path for normal reasoning. Do not call provider plugins
   directly.
2. Treat model usage as structured accounting data. Never place prompts, raw
   response text, tool arguments, API keys, or provider credentials inside
   usage records.
3. Preserve `provider`, `model`, token counts, latency, cost estimate,
   fallback chain, request ID, failure category, reset time, and finish reason
   when reporting diagnostics.
4. If a fallback provider is used, explain the fallback chain without hiding the
   original provider failure.
5. Services must not call this service directly. The agent/runtime coordinates
   model calls and passes resulting data to other services when appropriate.

## Service Functions

- `Generate(GenerateRequest) -> GenerateResponse`
  - Generated service function: `model_Generate`
  - Runs one non-streaming generation and returns text, tool calls, and usage.

- `StreamGenerate(StreamGenerateRequest) -> stream StreamGenerateResponse`
  - Generated service function: `model_StreamGenerate`
  - Streams deltas/tool calls and emits final structured usage.

- `Embed(EmbedRequest) -> EmbedResponse`
  - Generated service function: `model_Embed`
  - Creates provider-backed embeddings with provider/model/dimension metadata.

- `Rerank(RerankRequest) -> RerankResponse`
  - Generated service function: `model_Rerank`
  - Scores candidate documents for one query.

- `CountTokens(CountTokensRequest) -> CountTokensResponse`
  - Generated service function: `model_CountTokens`
  - Counts or estimates prompt/tool tokens without generation.

- `ListModels(ListModelsRequest) -> ListModelsResponse`
  - Generated service function: `model_ListModels`
  - Lists models exposed by provider adapters.

- `ProviderHealth(ProviderHealthRequest) -> ProviderHealthResponse`
  - Generated service function: `model_ProviderHealth`
  - Reports provider adapter readiness, auth, and reachability.

## Usage Fields

Every model response must include redacted usage:

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
- `failureCategory`
- `failureResetAt`

## Boundaries

- The model service owns provider dispatch, fallback ordering, model response
  usage accounting, and provider adapter diagnostics.
- Runtime owns session/run accumulation and persistence through runtime/Core
  activity storage. The model service emits usage to runtime; it does not call
  space, Core, indexer, embedding, or any other service.
- Provider adapters own provider-specific HTTP wire formats and error mapping.
- Agent prompts and service prompts must never embed provider credentials or
  direct provider API wire payloads.
