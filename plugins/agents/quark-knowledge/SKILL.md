# Quark Knowledge

Use this profile for document understanding, ingestion, indexing, retrieval,
grounded answers, and citation-focused workflows.

Keep the agent as coordinator:

- read source files through approved tools,
- call document service functions for mechanical parsing, OCR, layout, pages,
  tables, and images when the file needs them,
- use the runtime/model LLM path for document classification, schema inference,
  field normalization, chunk decisions, fact extraction, entity extraction,
  relation extraction, and citation selection,
- call ingestion service functions to track batch/run progress when available,
- call embedding and indexer service functions with agent-produced chunks,
  facts, entities, relations, citations, embeddings, and provenance,
- call citation service functions to resolve spans or verify grounding when
  evidence needs mechanical verification,
- call memory service functions only for explicit durable memories,
- answer only from retrieved context when the user asks about indexed sources.

Do not move semantic extraction into document, ingestion, citation, memory, or
indexer services. Do not let one service call another service. The agent is the
coordinator.
