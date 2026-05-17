# service-indexer

The indexer service stores and retrieves structured GraphRAG data over gRPC.
The production driver is Dgraph, a Go graph database with native
`float32vector` predicates and HNSW vector indexes.

Use `quark.indexer.v1.IndexerService` only after the agent has already parsed
documents, extracted entities/relations, and produced embeddings. The indexer
does not call LLMs, read raw files, or perform OCR.

## Agent Workflows

When the user asks to index PDFs or other documents:

1. Use file tools for plain text files and the document service for parser
   backed sources. For PDFs, call `document_ExtractText` or `document_GetPages`
   with the source path so PDF parsing stays in the document service.
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
5. Call canonical indexer functions. Use `indexer_UpsertDocument` for source
   document records and `indexer_UpsertChunk` for each searchable text chunk.
   Include `document`, `embeddingMetadata`, `facts`, `citations`,
   `provenance`, `entities`, `relations`, and source metadata whenever those
   records are known. Use `indexer_UpsertFact`, `indexer_UpsertEntity`,
   `indexer_UpsertRelation`, and `indexer_UpsertCitation` when updating one
   canonical record independently of a chunk upsert.

Indexing is not complete after extraction or embedding. Only tell the user a
document is indexed after `UpsertDocument` and at least one related
`UpsertChunk` return successful responses from the indexer service. When
multiple documents are listed, keep the filenames aligned with successful
canonical upserts and do not finish until every listed document has a successful
persistence result.

When the user asks questions about indexed documents:

1. Use the configured embedding service plugin on the user question.
2. Call `indexer_QueryContext` with the query vector, a reasonable limit, and
   graph depth.
3. Answer from the returned `reasoning_context` and cite source metadata when
   available.

Do not invent vectors. Do not answer indexed-document questions from memory
when the indexer service is available.

## RPCs

- `UpsertDocument(UpsertDocumentRequest) -> IndexStatus`
  - Generated service function: `indexer_UpsertDocument`
  - Required JSON fields: `document.id`
  - Persists one canonical source document record.

- `UpsertChunk(UpsertChunkRequest) -> IndexStatus`
  - Generated service function: `indexer_UpsertChunk`
  - Required JSON fields: `chunkId`, `textContent` or `textContentRef`,
    `embeddingRef`
  - Optional JSON fields: `embedding`, `document`, `embeddingMetadata`,
    `entities`, `relations`, `facts`, `citations`, `provenance`,
    `sourceMetadata`
  - Persists one canonical index record: source document, text chunk,
    embedding vector/metadata, extracted entities, graph relations, facts,
    citations, metadata, and provenance.

- `UpsertFact(UpsertFactRequest) -> IndexStatus`
  - Generated service function: `indexer_UpsertFact`
  - Required JSON fields: `fact.subject`, `fact.predicate`, `fact.object`
  - Persists or replaces one canonical fact record.

- `UpsertEntity(UpsertEntityRequest) -> IndexStatus`
  - Generated service function: `indexer_UpsertEntity`
  - Required JSON fields: `entity.id` or `entity.name`
  - Persists or replaces one canonical entity record.

- `UpsertRelation(UpsertRelationRequest) -> IndexStatus`
  - Generated service function: `indexer_UpsertRelation`
  - Required JSON fields: `relation.fromId`, `relation.toId`,
    `relation.relation`
  - Persists or replaces one canonical relation record.

- `UpsertCitation(UpsertCitationRequest) -> IndexStatus`
  - Generated service function: `indexer_UpsertCitation`
  - Required JSON fields: source URI, chunk ID, or text span
  - Persists or replaces one canonical citation record.

- `QueryContext(QueryRequest) -> ContextResponse`
  - Generated service function: `indexer_QueryContext`
  - Required JSON fields: `queryVectorRef`
  - Optional JSON fields: `queryVector`, `limit`, `depth`, `filters`
  - Returns ranked chunks, a graph fragment, citations, a structured
    `contextPackage`, and a flattened `reasoningContext` string suitable for an
    LLM context window.

- `GetContext(QueryRequest) -> ContextResponse`
  - Generated service function: `indexer_GetContext`
  - Compatibility alias for `QueryContext`.

- `IndexDocument(IndexRequest) -> IndexStatus`
  - Generated service function: `indexer_IndexDocument`
  - Compatibility alias for `UpsertChunk`.

- `DeleteDocument(DeleteDocumentRequest) -> DeleteDocumentResponse`
  - Generated service function: `indexer_DeleteDocument`
  - Required JSON fields: `documentId`
  - Deletes one canonical document and document-owned chunks. This is
    approval-gated because it removes indexed knowledge.

- `DeleteChunk(DeleteChunkRequest) -> DeleteChunkResponse`
  - Generated service function: `indexer_DeleteChunk`
  - Required JSON fields: `chunkId`
  - Deletes one canonical chunk by ID. This is approval-gated because it removes
    indexed knowledge.

## Contract Notes

- The indexer owns the canonical storage/query schema. It does not parse files,
  call LLMs, generate embeddings, select extraction profiles, or call other
  services.
- `UpsertChunk` receives agent-produced chunks, facts, entities, relations,
  citations, embedding references/metadata, and provenance. It must not receive
  raw PDFs or prompt-only payloads as a substitute for extracted knowledge.
- `IndexDocument` is a compatibility alias for the canonical chunk upsert path.
  Prefer `UpsertDocument` and `UpsertChunk` in new workflows. Re-indexing the
  same chunk replaces previous chunk-owned metadata predicates, entity links,
  document links, and relation nodes before writing the new canonical record.
- `DeleteDocument` removes the document, linked chunks, and chunk-owned
  relation nodes. Shared entity nodes remain available for other chunks that
  still reference them.
- `DeleteChunk` removes the chunk and chunk-owned relation nodes. Shared entity
  nodes remain available for other chunks that still reference them.
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
