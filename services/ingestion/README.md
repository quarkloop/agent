# Ingestion Service

`services/ingestion` is the planned durable run-state boundary for document
ingestion. It does not parse documents, call LLMs, embed chunks, index records,
or call other services. The agent coordinates those steps and records progress
here so partial batches can resume cleanly.

## Service Functions

| Function | RPC method | Request | Response | Purpose |
| --- | --- | --- | --- | --- |
| `ingestion_StartRun` | `quark.ingestion.v1.IngestionService/StartRun` | `StartRunRequest` | `StartRunResponse` | Create a durable ingestion run with per-source state. |
| `ingestion_GetRun` | `quark.ingestion.v1.IngestionService/GetRun` | `GetRunRequest` | `GetRunResponse` | Return run status and per-source progress. |
| `ingestion_ResumeRun` | `quark.ingestion.v1.IngestionService/ResumeRun` | `ResumeRunRequest` | `ResumeRunResponse` | Reopen an incomplete run for agent-coordinated resume. |
| `ingestion_UpdateSourceState` | `quark.ingestion.v1.IngestionService/UpdateSourceState` | `UpdateSourceStateRequest` | `UpdateSourceStateResponse` | Update one source phase, status, artifact reference, and last error. |
| `ingestion_ListIncompleteSources` | `quark.ingestion.v1.IngestionService/ListIncompleteSources` | `ListIncompleteSourcesRequest` | `ListIncompleteSourcesResponse` | List pending or failed sources that the agent can resume. |
| `ingestion_ListArtifacts` | `quark.ingestion.v1.IngestionService/ListArtifacts` | `ListArtifactsRequest` | `ListArtifactsResponse` | List run or source artifacts. |

## Source State

Each source tracks:

- file path
- source hash
- extraction state
- LLM structuring state
- embedding state
- indexing state
- citation state
- last error

Resume operations must skip succeeded sources and return only pending, running,
or failed sources unless the user explicitly asks to re-index.

## Ownership Boundaries

- The agent is the coordinator and decides what should resume.
- Document, model, embedding, indexer, and citation services remain stateless
  participants unless they own their own durable domain.
- Artifacts are references and metadata, not the authoritative source of run
  state.
- User directory mutation is not required for ingestion and must remain
  approval-gated through Core workspace mutation plans.
