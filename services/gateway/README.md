# Gateway Service

`services/gateway` builds the Quark Gateway service binary. Gateway is the single
agent-facing model boundary. It owns Bifrost lifecycle, provider routing,
fallback policy, embeddings, reranking, token counting, provider health, and
redacted usage accounting.

Runtime and agents must not call provider adapters directly. They call Gateway
through NATS service-function subjects, and Gateway performs provider-specific
work internally.

Embedding calls require an explicitly configured real provider and embedding
model. For OpenRouter, configure `QUARK_GATEWAY_EMBEDDING_PROVIDER=openrouter`
and `OPENROUTER_EMBEDDING_MODEL` alongside its API credential. Gateway has no
local synthetic embedding fallback and does not silently reuse a chat model.

## Service Functions

| Function | NATS subject | RPC method | Purpose |
| --- | --- | --- | --- |
| `gateway_Generate` | `svc.gateway.v1.generate` | `quark.gateway.v1.GatewayService/Generate` | Run one generation request and return text, tool calls, and usage. |
| `gateway_StreamGenerate` | `svc.gateway.v1.stream_generate` | `quark.gateway.v1.GatewayService/StreamGenerate` | Stream generation deltas/tool calls and final usage. |
| `gateway_Embed` | `svc.gateway.v1.embed` | `quark.gateway.v1.GatewayService/Embed` | Create provider-backed embeddings. |
| `gateway_Rerank` | `svc.gateway.v1.rerank` | `quark.gateway.v1.GatewayService/Rerank` | Rerank candidate documents for a query. |
| `gateway_CountTokens` | `svc.gateway.v1.count_tokens` | `quark.gateway.v1.GatewayService/CountTokens` | Count or estimate prompt/tool tokens. |
| `gateway_ListModels` | `svc.gateway.v1.list_models` | `quark.gateway.v1.GatewayService/ListModels` | List models visible through provider policy. |
| `gateway_ProviderHealth` | `svc.gateway.v1.provider_health` | `quark.gateway.v1.GatewayService/ProviderHealth` | Report provider adapter readiness. |
| `gateway_UsageSummary` | `svc.gateway.v1.usage_summary` | `quark.gateway.v1.GatewayService/UsageSummary` | Return redacted usage aggregates by provider/model. |
| `gateway_ReloadConfig` | `svc.gateway.v1.reload_config` | `quark.gateway.v1.GatewayService/ReloadConfig` | Reload provider and fallback policy after approval. |

## Usage Fields

Every model response returns redacted usage:

- `provider`
- `model`
- `inputTokens`
- `outputTokens`
- `embeddingTokens`
- `latencyMillis`
- `costEstimate`
- `fallbackChain`
- `requestId`
- `finishReason`

Usage must never contain prompt text, response text, tool arguments, API keys,
headers, or provider credentials.

## Boundaries

- Gateway owns provider dispatch, Bifrost lifecycle, fallback chain recording,
  usage accounting, provider health, and provider diagnostics.
- Runtime owns session/run accumulation and persistence.
- Services such as indexer, document, ingestion, and citation do not call
  Gateway. The agent is the coordinator.
