# Run State Service Plugin

Run State exposes generic durable execution records and NATS KV-backed active
leases through `quark.runstate.v1.RunStateService`.

## Service Functions

| Function | Purpose |
| --- | --- |
| `runstate_StartRun` | Create a durable run with generic items and retention metadata. |
| `runstate_GetRun`, `runstate_ListRuns`, `runstate_ResumeRun` | Inspect or resume durable run state. |
| `runstate_UpdateItemState` | Record arbitrary workflow phases on an item. |
| `runstate_AppendArtifact`, `runstate_AppendReference` | Persist artifacts or returned audit lookup `reference_id` values. |
| `runstate_MarkFailed`, `runstate_MarkComplete`, `runstate_CancelRun` | Record terminal run/item state. |
| `runstate_ListIncompleteItems`, `runstate_ListArtifacts` | Retrieve resumable work and artifacts. |
| `runstate_AcquireLease`, `runstate_RenewLease`, `runstate_ReleaseLease` | Coordinate active ownership through CAS-backed NATS KV leases. |

Durable records and active leases are separate stores. The service records
state only; the runtime agent remains responsible for orchestration.
For service-call evidence, `service_call_refs` stores the response
`reference_id`, not transport invocation IDs or payload contents.
