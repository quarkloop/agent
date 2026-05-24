# Run State Service

`services/runstate` is the universal state-recording boundary for durable
agent-coordinated work. It stores generic runs, items, phases, artifacts and
references in service-owned files. Active ownership leases are stored in the
supervisor-provisioned `runstate_leases` NATS KV bucket using CAS semantics.

It does not execute workflow operations, parse files, call models, produce
embeddings, index data, or call another service.

## Service Functions

| Function | RPC method | Purpose |
| --- | --- | --- |
| `runstate_StartRun` | `RunStateService/StartRun` | Create a durable execution run. |
| `runstate_GetRun` | `RunStateService/GetRun` | Return one run and its item state. |
| `runstate_ListRuns` | `RunStateService/ListRuns` | Filter runs by space, kind or status. |
| `runstate_ResumeRun` | `RunStateService/ResumeRun` | Re-open incomplete items for coordination. |
| `runstate_UpdateItemState` | `RunStateService/UpdateItemState` | Record an arbitrary item phase and service references. |
| `runstate_AppendArtifact` | `RunStateService/AppendArtifact` | Attach an artifact reference. |
| `runstate_AppendReference` | `RunStateService/AppendReference` | Attach an audit or service-call reference. |
| `runstate_MarkFailed` | `RunStateService/MarkFailed` | Record failed run or item state. |
| `runstate_MarkComplete` | `RunStateService/MarkComplete` | Record completed run or item state. |
| `runstate_CancelRun` | `RunStateService/CancelRun` | Cancel a run while retaining its history. |
| `runstate_ListIncompleteItems` | `RunStateService/ListIncompleteItems` | List resumable items. |
| `runstate_ListArtifacts` | `RunStateService/ListArtifacts` | List recorded artifacts. |
| `runstate_AcquireLease` | `RunStateService/AcquireLease` | Acquire active work ownership. |
| `runstate_RenewLease` | `RunStateService/RenewLease` | Renew an owned lease. |
| `runstate_ReleaseLease` | `RunStateService/ReleaseLease` | Release an owned lease. |

## Storage Boundary

- `runstate-records.json` is the durable record ledger and includes retention
  timestamps for later cleanup policy enforcement.
- `runstate_leases` is ephemeral coordination state; lease expiry or deletion
  cannot remove durable history.
- On first startup, an existing `ingestion-state.json` is imported into the
  universal item model and marked as migrated data.
