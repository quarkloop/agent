# service-embedding-openrouter

The OpenRouter embedding service converts text into provider-backed vectors
over gRPC using the same `EmbeddingService` contract as the local embedding
service. It is a compatibility service-function surface while provider-backed
embeddings move behind the Quark Model Service boundary.

## Agent Workflows

When indexing documents:

1. Call `embedding_Embed` for every chunk that will be stored in the index.
2. Pass the returned `embeddingRef` to `indexer_IndexDocument.embeddingRef`.
3. Preserve `model`, `provider`, `dimensions`, and `contentHash` as embedding
   metadata when useful.

When querying indexed documents:

1. Call `embedding_Embed` for the user query using the same OpenRouter model
   used for indexed chunks.
2. Pass the returned `embeddingRef` to `indexer_GetContext.queryVectorRef`.

## RPCs

- `Embed(EmbedRequest) -> EmbedResponse`
  - Generated service function: `embedding_Embed`
  - Required JSON fields: `input`
  - Optional JSON fields: `model`, `dimensions`
  - The runtime service function returns an `embeddingRef` instead of exposing
    the raw vector to the LLM.

## Configuration

Run the embedding service with:

```bash
embedding-service --provider openrouter \
  --model nvidia/llama-nemotron-embed-vl-1b-v2:free \
  --dimensions 2048
```

The API key must be supplied through `OPENROUTER_API_KEY`.

Optional ordered fallback:

```bash
embedding-service --provider openrouter \
  --model nvidia/llama-nemotron-embed-vl-1b-v2:free \
  --dimensions 2048 \
  --fallbacks local||32
```

## Contract Notes

- Use the same OpenRouter model for document chunks and query text.
- Do not invent vectors or manually copy vectors; call this service before
  indexing or retrieval and pass the returned reference onward.
- If the provider returns dimensions different from the configured dimensions,
  the service fails instead of storing mixed vector shapes.
- Fallback is allowed for runtime auth, quota, model-unavailable, and transport
  failures. Dimension mismatch and invalid configuration are terminal because
  they would hide broken index/query vector compatibility.
- Preserve `provider`, `model`, `dimensions`, and `contentHash` in indexer
  records so query-time embeddings can be compared safely.
