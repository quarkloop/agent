# Workflow Service Plugin

Workflow declares durable orchestration service functions. Temporal is isolated
inside the Workflow service; runtime, agents, and other services use NATS
service-function calls.

## Service Functions

| Function | NATS subject | RPC method | Risk | Approval | Streaming | Purpose |
| --- | --- | --- | --- | --- | --- | --- |
| `workflow_Start` | `svc.workflow.v1.start` | `quark.workflow.v1.WorkflowService/Start` | write | no | no | Start a durable workflow. |
| `workflow_Signal` | `svc.workflow.v1.signal` | `quark.workflow.v1.WorkflowService/Signal` | write | no | no | Send a durable signal to a workflow. |
| `workflow_Query` | `svc.workflow.v1.query` | `quark.workflow.v1.WorkflowService/Query` | read | no | no | Query workflow state. |
| `workflow_Cancel` | `svc.workflow.v1.cancel` | `quark.workflow.v1.WorkflowService/Cancel` | write | yes | no | Cancel a workflow. |
| `workflow_Describe` | `svc.workflow.v1.describe` | `quark.workflow.v1.WorkflowService/Describe` | read | no | no | Describe one workflow execution. |
| `workflow_List` | `svc.workflow.v1.list` | `quark.workflow.v1.WorkflowService/List` | read | no | no | List workflow executions. |
| `workflow_StreamEvents` | `svc.workflow.v1.stream_events` | `quark.workflow.v1.WorkflowService/StreamEvents` | read | no | yes | Stream workflow progress events. |

## Document Ingestion Workflow

The first workflow type is `document_ingestion_indexing`. It accepts sources
and durable checkpoints. The agent performs service-function calls itself and
sends `checkpoint_completed` or `checkpoint_failed` signals with the
checkpoint ID; Workflow never invokes a domain service.

## Operational Requirements

- Temporal frontend must be reachable by the Workflow service.
- NATS must be reachable for Workflow service-function endpoints.
- Runtime and agents must not embed Temporal SDK code.
- Services must not call Workflow or each other.
