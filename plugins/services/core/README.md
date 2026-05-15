# Core Service Plugin

The Core service plugin declares Quark's operational backbone service
functions. Core centralizes approval state, audit trails, artifacts,
configuration, secret references, event streams, policy decisions, workspace
mutation plans, schedules, and evaluations.

## Service Functions

| Function | RPC method | Risk | Approval | Purpose |
| --- | --- | --- | --- | --- |
| `core_CheckHealth` | `quark.core.v1.CoreService/CheckHealth` | read | no | Return Core health diagnostics. |
| `core_CheckReadiness` | `quark.core.v1.CoreService/CheckReadiness` | read | no | Return Core readiness diagnostics. |
| `core_RecordAuditEvent` | `quark.core.v1.CoreService/RecordAuditEvent` | write | no | Record a redacted audit event. |
| `core_ListAuditEvents` | `quark.core.v1.CoreService/ListAuditEvents` | read | no | List redacted audit events. |
| `core_PutArtifact` | `quark.core.v1.CoreService/PutArtifact` | write | no | Persist a redacted artifact reference. |
| `core_GetArtifact` | `quark.core.v1.CoreService/GetArtifact` | read | no | Return a redacted artifact reference. |
| `core_RequestApproval` | `quark.core.v1.CoreService/RequestApproval` | write | no | Create an approval request. |
| `core_RecordApprovalDecision` | `quark.core.v1.CoreService/RecordApprovalDecision` | write | no | Record approval or denial. |
| `core_GetConfig` | `quark.core.v1.CoreService/GetConfig` | read | no | Read scoped configuration. |
| `core_SetConfig` | `quark.core.v1.CoreService/SetConfig` | write | yes | Write scoped configuration. |
| `core_GetSecretRef` | `quark.core.v1.CoreService/GetSecretRef` | read | no | Return a secret reference without revealing the value. |
| `core_PublishEvent` | `quark.core.v1.CoreService/PublishEvent` | write | no | Publish an ordered redacted event. |
| `core_ListEvents` | `quark.core.v1.CoreService/ListEvents` | read | no | List ordered redacted events. |
| `core_EvaluatePolicy` | `quark.core.v1.CoreService/EvaluatePolicy` | read | no | Return policy denial reasons or required approvals. |
| `core_CreateWorkspaceMutationPlan` | `quark.core.v1.CoreService/CreateWorkspaceMutationPlan` | write | yes | Create an approval-gated workspace mutation plan. |
| `core_ApproveWorkspaceMutationPlan` | `quark.core.v1.CoreService/ApproveWorkspaceMutationPlan` | write | yes | Bind approval to a mutation plan. |
| `core_ScheduleRun` | `quark.core.v1.CoreService/ScheduleRun` | write | yes | Create or update scheduled runs. |
| `core_ListSchedules` | `quark.core.v1.CoreService/ListSchedules` | read | no | List scheduled runs. |
| `core_RecordEvaluation` | `quark.core.v1.CoreService/RecordEvaluation` | write | no | Record an evaluation result. |
| `core_GetEvaluation` | `quark.core.v1.CoreService/GetEvaluation` | read | no | Return an evaluation result. |

## Ownership Split

Supervisor-owned Core functions: persisted approvals, audit, artifacts, config,
secret references, event logs, policy decisions, workspace mutation plans,
schedules, and evaluations.

Runtime-owned execution features: tool execution, service function dispatch,
inference loop events, in-memory activity fan-out, and approval enforcement at
execution time.

## Redaction And Ordering

Artifacts, audit events, and events carry explicit redaction flags and reasons.
Event streams carry monotonically increasing `sequence` values so E2E and
debugging tools can reason about ordering without parsing logs.

## Health And Readiness

- Health protocol: gRPC health v1.
- Health service: `quark.core.v1.CoreService`.
- Required readiness: yes, before runtime receives the service catalog.
- Minimum descriptor version: `1.0.0`.
