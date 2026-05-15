# Memory Service Plugin

The memory service owns explicit space-scoped memory records for agents. It is
not a hidden semantic extractor and does not call LLMs or other services.

## Service Functions

| Function | RPC method | Risk | Approval | Idempotent | Purpose |
| --- | --- | --- | --- | --- | --- |
| `memory_Put` | `quark.memory.v1.MemoryService/Put` | write | no | no | Persist an explicit memory record with provenance. |
| `memory_Get` | `quark.memory.v1.MemoryService/Get` | read | no | yes | Fetch one memory record by space, collection, and key. |
| `memory_Search` | `quark.memory.v1.MemoryService/Search` | read | no | yes | Search memory records in a space and collection. |
| `memory_Delete` | `quark.memory.v1.MemoryService/Delete` | write | yes | no | Delete an explicit memory record. |

## Boundary

The agent decides what should be remembered. Runtime/model inference stays in
the agent loop. The memory service stores explicit records and provenance; it
does not infer memories from user prompts, index documents, or query other
services.

## Health And Readiness

- Health protocol: gRPC health v1.
- Health service: `quark.memory.v1.MemoryService`.
- Required readiness: yes, before runtime receives this service in the catalog.
- Minimum descriptor version: `1.0.0`.
- Startup diagnostics cover missing `QUARK_MEMORY_ADDR`, failed health checks,
  descriptor version mismatch, and missing RPC descriptors.
