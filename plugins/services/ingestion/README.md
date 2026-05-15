# Ingestion Service Plugin

The ingestion service owns durable run state for Knowledge ingestion batches.
It tracks progress and artifacts; it does not perform parsing, LLM semantic
extraction, embedding, indexing, or service-to-service orchestration.

## Service Functions

| Function | RPC method | Risk | Approval | Idempotent | Purpose |
| --- | --- | --- | --- | --- | --- |
| `ingestion_StartRun` | `quark.ingestion.v1.IngestionService/StartRun` | write | no | no | Create a durable ingestion run with per-source state. |
| `ingestion_GetRun` | `quark.ingestion.v1.IngestionService/GetRun` | read | no | yes | Return run status and per-source progress. |
| `ingestion_ResumeRun` | `quark.ingestion.v1.IngestionService/ResumeRun` | write | no | no | Reopen an incomplete run for agent-coordinated resume. |
| `ingestion_UpdateSourceState` | `quark.ingestion.v1.IngestionService/UpdateSourceState` | write | no | no | Update one source phase, status, artifact reference, and last error. |
| `ingestion_ListArtifacts` | `quark.ingestion.v1.IngestionService/ListArtifacts` | read | no | yes | List run or source artifacts. |

## Boundary

The agent owns orchestration and LLM semantic work. Services are participants:
document extraction produces source evidence, model/embedding produces vectors,
indexer persists canonical knowledge, citation verifies grounding, and
ingestion records what happened.

## Health And Readiness

- Health protocol: gRPC health v1.
- Health service: `quark.ingestion.v1.IngestionService`.
- Required readiness: yes, before runtime receives this service in the catalog.
- Minimum descriptor version: `1.0.0`.
- Startup diagnostics cover missing `QUARK_INGESTION_ADDR`, failed health
  checks, descriptor version mismatch, and missing RPC descriptors.
