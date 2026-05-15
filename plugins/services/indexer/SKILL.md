# service-indexer

The indexer service stores and retrieves structured GraphRAG data over gRPC.
The production driver is Dgraph, a Go graph database with native
`float32vector` predicates and HNSW vector indexes.

Use `quark.indexer.v1.IndexerService` only after the agent has already parsed
documents, extracted entities/relations, and produced embeddings. The indexer
does not call LLMs, read raw files, or perform OCR.

## Agent Workflows

When the user asks to index PDFs or other documents:

1. Use file tools to read the source content. For PDFs, use `fs` with
   `command=extract_pdf`.
2. Extract a useful, compact chunk for each document or section. Preserve the
   important facts needed for later Q&A.
3. Use the runtime/model LLM path to perform semantic extraction from the
   source evidence: document classification, schema inference, field
   normalization, chunk decisions, stable entities, relationships, facts,
   citations, and source provenance. Entity IDs should be normalized from
   entity names and relation endpoints must reuse the same IDs as the entity
   list.
4. Use the configured embedding service plugin on each chunk and pass the
   returned `embeddingRef` as `embeddingRef`.
5. Call `indexer_IndexDocument`. Include source metadata such as source path
   and filename when available. Prefer first-class `document`,
   `embeddingMetadata`, `facts`, `citations`, and `provenance` fields when you
   have them; otherwise the service will derive the canonical fields it can
   from `sourceMetadata`.

Indexing is not complete after extraction or embedding. Only tell the user a
document is indexed after `IndexDocument` returns a successful response from the
indexer service. When multiple documents are listed, keep the filenames aligned
with successful `IndexDocument` results and do not finish until every listed
document has one successful persistence result.

When the user asks questions about indexed documents:

1. Use the configured embedding service plugin on the user question.
2. Call `indexer_GetContext` with the query vector, a reasonable limit, and
   graph depth.
3. Answer from the returned `reasoning_context` and cite source metadata when
   available.

Do not invent vectors. Do not answer indexed-document questions from memory
when the indexer service is available.

## RPCs

- `IndexDocument(IndexRequest) -> IndexStatus`
  - Generated service function: `indexer_IndexDocument`
  - Required JSON fields: `chunkId`, `textContent`, `embeddingRef`
  - Optional JSON fields: `embedding`, `document`, `embeddingMetadata`,
    `entities`, `relations`, `facts`, `citations`, `provenance`,
    `sourceMetadata`
  - Persists one canonical index record: source document, text chunk,
    embedding vector/metadata, extracted entities, graph relations, facts,
    citations, metadata, and provenance.

- `GetContext(QueryRequest) -> ContextResponse`
  - Generated service function: `indexer_GetContext`
  - Required JSON fields: `queryVectorRef`
  - Optional JSON fields: `queryVector`, `limit`, `depth`, `filters`
  - Returns ranked chunks, a graph fragment, citations, a structured
    `contextPackage`, and a flattened `reasoningContext` string suitable for an
    LLM context window.

- `DeleteChunk(DeleteChunkRequest) -> DeleteChunkResponse`
  - Generated service function: `indexer_DeleteChunk`
  - Required JSON fields: `chunkId`
  - Deletes one canonical chunk by ID. This is approval-gated because it removes
    indexed knowledge.

## Contract Notes

- The indexer owns the canonical storage/query schema. It does not parse files,
  call LLMs, generate embeddings, select extraction profiles, or call other
  services.
- `IndexDocument` receives agent-produced chunks, facts, entities, relations,
  citations, embedding references/metadata, and provenance. It must not receive
  raw PDFs or prompt-only payloads as a substitute for extracted knowledge.
- `IndexDocument` remains a single atomic canonical record upsert instead of a
  collection of separate document/chunk/fact/entity/citation writes. Splitting
  those writes would make graph and vector consistency harder unless the store
  can guarantee one transaction across all record parts.
- Query text must be embedded by the agent before `GetContext`.
- Use `embeddingRef` and `queryVectorRef` from `embedding_Embed` instead of
  manually copying vectors through the LLM.
- Use the same embedding dimensions for document chunks and query text.
- `embeddingMetadata.dimensions` must match the vector length.
- Metadata filters are exact key/value matches.
- Returned chunk scores are normalized into the `[0,1]` range. Use
  `contextPackage.confidence` as the aggregate retrieval confidence signal.
- Entity IDs are stable identifiers; when omitted, the service derives one from
  the entity name.
- Facts should include citations whenever source evidence exists.
- Provenance should identify the original source URI/path, source hash when
  known, producing agent/tool trace, and any RBAC or tenant tags in metadata.
- The service is safe for concurrent calls and honors request cancellation.
