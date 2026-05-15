# Core Service

`services/core` is the planned Quark operational backbone. It centralizes
health/readiness diagnostics, audit, artifacts, approvals, config, secret
references, events, policy, workspace mutation plans, scheduler records, and
evaluations.

## Service Functions

| Function | RPC method | Request | Response | Purpose |
| --- | --- | --- | --- | --- |
| `core_CheckHealth` | `quark.core.v1.CoreService/CheckHealth` | `CheckHealthRequest` | `CheckHealthResponse` | Return Core health diagnostics. |
| `core_CheckReadiness` | `quark.core.v1.CoreService/CheckReadiness` | `CheckReadinessRequest` | `CheckReadinessResponse` | Return Core readiness diagnostics. |
| `core_RecordAuditEvent` | `quark.core.v1.CoreService/RecordAuditEvent` | `RecordAuditEventRequest` | `RecordAuditEventResponse` | Record a redacted audit event. |
| `core_ListAuditEvents` | `quark.core.v1.CoreService/ListAuditEvents` | `ListAuditEventsRequest` | `ListAuditEventsResponse` | List redacted audit events. |
| `core_PutArtifact` | `quark.core.v1.CoreService/PutArtifact` | `PutArtifactRequest` | `PutArtifactResponse` | Persist a redacted artifact reference. |
| `core_GetArtifact` | `quark.core.v1.CoreService/GetArtifact` | `GetArtifactRequest` | `GetArtifactResponse` | Return a redacted artifact reference. |
| `core_RequestApproval` | `quark.core.v1.CoreService/RequestApproval` | `RequestApprovalRequest` | `RequestApprovalResponse` | Create an approval request. |
| `core_RecordApprovalDecision` | `quark.core.v1.CoreService/RecordApprovalDecision` | `RecordApprovalDecisionRequest` | `RecordApprovalDecisionResponse` | Record approval or denial. |
| `core_GetConfig` | `quark.core.v1.CoreService/GetConfig` | `GetConfigRequest` | `GetConfigResponse` | Read scoped configuration. |
| `core_SetConfig` | `quark.core.v1.CoreService/SetConfig` | `SetConfigRequest` | `SetConfigResponse` | Write scoped configuration. |
| `core_GetSecretRef` | `quark.core.v1.CoreService/GetSecretRef` | `GetSecretRefRequest` | `GetSecretRefResponse` | Return a secret reference without the value. |
| `core_PublishEvent` | `quark.core.v1.CoreService/PublishEvent` | `PublishEventRequest` | `PublishEventResponse` | Publish an ordered redacted event. |
| `core_ListEvents` | `quark.core.v1.CoreService/ListEvents` | `ListEventsRequest` | `ListEventsResponse` | List ordered redacted events. |
| `core_EvaluatePolicy` | `quark.core.v1.CoreService/EvaluatePolicy` | `EvaluatePolicyRequest` | `EvaluatePolicyResponse` | Return policy denial reasons or required approvals. |
| `core_CreateWorkspaceMutationPlan` | `quark.core.v1.CoreService/CreateWorkspaceMutationPlan` | `CreateWorkspaceMutationPlanRequest` | `CreateWorkspaceMutationPlanResponse` | Create an approval-gated workspace mutation plan. |
| `core_ApproveWorkspaceMutationPlan` | `quark.core.v1.CoreService/ApproveWorkspaceMutationPlan` | `ApproveWorkspaceMutationPlanRequest` | `ApproveWorkspaceMutationPlanResponse` | Bind approval to a mutation plan. |
| `core_ScheduleRun` | `quark.core.v1.CoreService/ScheduleRun` | `ScheduleRunRequest` | `ScheduleRunResponse` | Create or update scheduled runs. |
| `core_ListSchedules` | `quark.core.v1.CoreService/ListSchedules` | `ListSchedulesRequest` | `ListSchedulesResponse` | List scheduled runs. |
| `core_RecordEvaluation` | `quark.core.v1.CoreService/RecordEvaluation` | `RecordEvaluationRequest` | `RecordEvaluationResponse` | Record an evaluation result. |
| `core_GetEvaluation` | `quark.core.v1.CoreService/GetEvaluation` | `GetEvaluationRequest` | `GetEvaluationResponse` | Return an evaluation result. |

## Ownership Boundaries

- Supervisor-owned Core service persists operational state and exposes typed
  governance functions.
- Runtime owns the execution loop, service dispatch, model/tool activity, and
  local approval enforcement.
- Other services do not invent private approval, audit, artifact, config, or
  policy systems.
- User directory mutation goes through workspace mutation plans and explicit
  approval.

## Redaction Rules

Audit events, artifacts, config values, secret references, and events must never
include raw secrets. Records include `redacted` flags and redaction reasons so
debugging artifacts remain useful without leaking sensitive data.
