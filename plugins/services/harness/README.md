# Harness Service

The Harness service is the single owner of model-context packaging, context
usage reports, included-memory reporting, prompt-material provenance, and
explicit agent-authored memory.
The implementation is a Rust NATS responder conforming to Quark's versioned
service-function envelope.

## Service Functions

| Function | Purpose |
| --- | --- |
| `harness_ComposeContext` | Package supplied prompt materials and history into bounded model messages and persist usage, provenance, and memory-contribution reporting. |
| `harness_GetContextReport` | Retrieve one stored context report for audit and inspection. |
| `harness_StreamContextReports` | Stream recent context reports for a session. |
| `harness_PutMemory` | Persist an explicit memory record with provenance. |
| `harness_GetMemory` | Retrieve one explicit memory record. |
| `harness_SearchMemory` | Search records within a space and scope. |
| `harness_DeleteMemory` | Remove one explicit memory record. |

## Ownership

Harness accepts already-resolved plugin material and runtime facts. It does
not author product prompts, run models, execute tools, or call other services.
Its files are service-owned state, not user workspace content.
