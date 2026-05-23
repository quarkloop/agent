# Gateway Service Plugin

Gateway declares the agent-facing model service functions for generation,
streaming generation, embeddings, reranking, token counting, model listing,
provider health, fallback, and usage accounting.

Gateway embeds Bifrost as an implementation detail. Runtime, agents, tools, and
other services do not call provider APIs directly.

## Service Functions

| Function | NATS subject | RPC method | Risk | Approval | Streaming | Purpose |
| --- | --- | --- | --- | --- | --- | --- |
| `gateway_Generate` | `svc.gateway.v1.generate` | `quark.model.v1.ModelService/Generate` | read | no | no | Run one generation request and return text, tool calls, and usage. |
| `gateway_StreamGenerate` | `svc.gateway.v1.stream_generate` | `quark.model.v1.ModelService/StreamGenerate` | read | no | yes | Stream generation deltas/tool calls and final usage. |
| `gateway_Embed` | `svc.gateway.v1.embed` | `quark.model.v1.ModelService/Embed` | read | no | no | Create embeddings through Gateway provider policy. |
| `gateway_Rerank` | `svc.gateway.v1.rerank` | `quark.model.v1.ModelService/Rerank` | read | no | no | Rerank candidate documents for a query. |
| `gateway_CountTokens` | `svc.gateway.v1.count_tokens` | `quark.model.v1.ModelService/CountTokens` | read | no | no | Count or estimate model prompt/tool tokens. |
| `gateway_ListModels` | `svc.gateway.v1.list_models` | `quark.model.v1.ModelService/ListModels` | read | no | no | List provider models visible through Gateway. |
| `gateway_ProviderHealth` | `svc.gateway.v1.provider_health` | `quark.model.v1.ModelService/ProviderHealth` | read | no | no | Report provider adapter readiness. |
| `gateway_UsageSummary` | `svc.gateway.v1.usage_summary` | `quark.model.v1.ModelService/UsageSummary` | read | no | no | Return redacted Gateway usage aggregates by provider and model. |
| `gateway_ReloadConfig` | `svc.gateway.v1.reload_config` | `quark.model.v1.ModelService/ReloadConfig` | admin | yes | no | Reload Gateway provider and fallback policy. |

## Usage Fields

Every model response carries redacted usage:

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

Usage never contains prompts, response text, tool arguments, API keys, or
provider credentials.

## Provider Errors And Fallback

Gateway returns structured provider failure categories:

- `auth`
- `rate_limit`
- `model_unavailable`
- `context_overflow`
- `transport`
- `invalid_request`
- `provider_response`
- `providers_exhausted`

Fallback is explicit and ordered. Diagnostics include provider, model, fallback
chain, failure category, and reset time when available.

## Health And Readiness

- Health protocol: gRPC health v1 during the transitional registry period.
- Health service: `quark.model.v1.ModelService`.
- NATS subjects: `svc.gateway.v1.*`.
- Descriptor registry: `quark.service.v1.ServiceRegistry`.
- Readiness is required before runtime receives Gateway in the resolved catalog.
