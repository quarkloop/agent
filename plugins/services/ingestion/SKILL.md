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
4. Call `ingestion_ListRuns` when the user asks for recent ingestion work in a
   space.
5. Call `ingestion_ListIncompleteSources` to identify only pending or failed
   files before resuming a batch.
6. Call `ingestion_AppendArtifact` when a step produces an artifact reference
   that should be attached to the durable run state.
7. Call `ingestion_MarkFailed`, `ingestion_MarkComplete`, or
   `ingestion_CancelRun` to close explicit terminal states.
8. Call `ingestion_ListArtifacts` when an agent needs run artifacts for
   debugging or provenance.

## RPCs

- `StartRun(StartRunRequest) -> StartRunResponse`
  - Generated service function: `ingestion_StartRun`
- `GetRun(GetRunRequest) -> GetRunResponse`
  - Generated service function: `ingestion_GetRun`
- `ListRuns(ListRunsRequest) -> ListRunsResponse`
  - Generated service function: `ingestion_ListRuns`
- `ResumeRun(ResumeRunRequest) -> ResumeRunResponse`
  - Generated service function: `ingestion_ResumeRun`
- `UpdateSourceState(UpdateSourceStateRequest) -> UpdateSourceStateResponse`
  - Generated service function: `ingestion_UpdateSourceState`
- `AppendArtifact(AppendArtifactRequest) -> AppendArtifactResponse`
  - Generated service function: `ingestion_AppendArtifact`
- `MarkFailed(MarkFailedRequest) -> MarkFailedResponse`
  - Generated service function: `ingestion_MarkFailed`
- `MarkComplete(MarkCompleteRequest) -> MarkCompleteResponse`
  - Generated service function: `ingestion_MarkComplete`
- `CancelRun(CancelRunRequest) -> CancelRunResponse`
  - Generated service function: `ingestion_CancelRun`
- `ListIncompleteSources(ListIncompleteSourcesRequest) -> ListIncompleteSourcesResponse`
  - Generated service function: `ingestion_ListIncompleteSources`
- `ListArtifacts(ListArtifactsRequest) -> ListArtifactsResponse`
  - Generated service function: `ingestion_ListArtifacts`

## Contract Notes

- The agent remains the coordinator. This service does not parse documents,
  call LLMs, embed chunks, call indexer, or call another service.
- Source state records should track parsing, LLM structuring, embedding,
  indexing, citation verification, and last error.
- Resume only sources whose extraction, structuring, embedding, indexing, or
  citation state is pending, running, or failed. Do not redo succeeded sources
  without explicit user intent.
- User directory mutation is not required for ingestion and must remain
  approval-gated elsewhere.
