# service-embedding

The embedding service converts text into deterministic local vectors over
gRPC. It is suitable for development, tests, and repeatable local indexing
flows. It is the compatibility service-function surface for local embedding;
provider-backed model embeddings belong behind the Quark Model Service boundary.

## Agent Workflows

When indexing documents:

1. Call `embedding_Embed` for every chunk that will be stored in the index.
2. Pass the returned `embeddingRef` to `indexer_UpsertChunk.embeddingRef`.
3. Preserve `model`, `provider`, `dimensions`, and `contentHash` as source or
   embedding metadata when useful.

When querying indexed documents:

1. Call `embedding_Embed` for the user query using the same dimensions used for
   indexed chunks.
2. Pass the returned `embeddingRef` to `indexer_QueryContext.queryVectorRef`.

## RPCs

- `Embed(EmbedRequest) -> EmbedResponse`
  - Generated service function: `embedding_Embed`
  - Required JSON fields: `input`
  - Optional JSON fields: `model`, `dimensions`
  - The gRPC service returns `vector`, `provider`, `model`, `dimensions`, and
    `contentHash`; the runtime service function returns an `embeddingRef`
    instead of exposing the raw vector to the LLM.

## Contract Notes

- Use consistent dimensions for all chunks and query vectors in the same
  index. The local service owns its configured dimension; leave `dimensions`
  unset unless the user explicitly asks for a configured dimension.
- The local implementation is deterministic and does not call external
  services.
- Do not invent vectors or manually copy vectors; call this service before
  indexing or retrieval and pass the returned reference onward.
- Keep `provider`, `model`, `dimensions`, and `contentHash` with indexer
  records so query-time embeddings can use a compatible vector shape.
