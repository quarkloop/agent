# service-ingestion

The ingestion service stores durable run records for document ingestion
batches. It lets the agent resume partial work without treating artifacts or
test helpers as the source of truth.

## Agent Workflows

1. Call `ingestion_StartRun` when a user asks to ingest a batch of files.
2. Update each source through `ingestion_UpdateSourceState` after parsing,
   semantic structuring, embedding, indexing, and citation verification.
3. Call `ingestion_GetRun` or `ingestion_ResumeRun` before retrying failed or
   incomplete files.
4. Call `ingestion_ListArtifacts` when an agent needs run artifacts for
   debugging or provenance.

## RPCs

- `StartRun(StartRunRequest) -> StartRunResponse`
  - Generated service function: `ingestion_StartRun`
- `GetRun(GetRunRequest) -> GetRunResponse`
  - Generated service function: `ingestion_GetRun`
- `ResumeRun(ResumeRunRequest) -> ResumeRunResponse`
  - Generated service function: `ingestion_ResumeRun`
- `UpdateSourceState(UpdateSourceStateRequest) -> UpdateSourceStateResponse`
  - Generated service function: `ingestion_UpdateSourceState`
- `ListArtifacts(ListArtifactsRequest) -> ListArtifactsResponse`
  - Generated service function: `ingestion_ListArtifacts`

## Contract Notes

- The agent remains the coordinator. This service does not parse documents,
  call LLMs, embed chunks, call indexer, or call another service.
- Source state records should track parsing, LLM structuring, embedding,
  indexing, citation verification, and last error.
- User directory mutation is not required for ingestion and must remain
  approval-gated elsewhere.
