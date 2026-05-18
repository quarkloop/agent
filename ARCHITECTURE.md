# Architecture

Quark is a local agent operating environment.

```text
Supervisor = control plane
Runtime    = execution engine
Agents     = reasoning coordinators
Services   = typed kernel capabilities
Plugins    = installable extension units
Spaces     = isolated workspaces
Quarkfile  = space-level override/config layer
```

## Process Model

The supervisor is the long-running daemon. It owns persistent state, space
metadata, sessions, plugin installation, service discovery, service readiness,
catalog generation, and runtime lifecycle.

The runtime is launched by the supervisor. It consumes supervisor-resolved
plugin and service catalogs, executes the selected agent profile, manages
sessions, assembles prompts, calls the model service/provider path, dispatches
tools and service functions, and emits activity.

The CLI is a thin HTTP client. It reads or writes only the local `Quarkfile` and
delegates everything else to the supervisor or the resolved runtime.

## Plugin Types

| Type | Purpose |
| --- | --- |
| `tool` | Executable/lib-mode tools such as `fs`, `bash`, `web-search`, and build-release compatibility. |
| `service` | gRPC service descriptors, `SKILL.md`, readiness rules, and service function metadata. |
| `agent` | Agent profile, system prompt, skills, permissions, handoff rules, and eval expectations. |
| `provider` | Model provider adapters, migrating behind the model service boundary. |
| `skill` | Guidance-only extension content. |

Agent prompts and personalities belong to agent plugins, not runtime source
code. Launch profiles are Quark Knowledge, Quark DevOps, and Quark System.

## Services

Service functions are agent-facing callable operations. RPC methods are the
gRPC transport implementation. Tool calls are the runtime execution envelope
used by the LLM/function-calling loop.

Services execute deterministic typed work and must not call each other. The
agent coordinates multi-step flows.

Initial service stacks:

- Quark Knowledge: document extraction, ingestion state, embedding, indexer,
  citation, Core artifacts/audit, and model-mediated semantic extraction.
- Quark DevOps: repo, build, test, container, release, deploy, and policy
  functions.
- Quark System: Linux/system snapshot, process, network, logs, metrics, and
  policy-gated admin functions.
- Quark Core: health, readiness, audit, artifacts, approval, config, events,
  policy, scheduler, and workspace mutation plans.
- Quark Model: provider adapters, generation, embedding, fallback, usage, and
  provider diagnostics.

## Knowledge Flow

The indexer stores canonical knowledge records only. It does not parse files,
call LLMs, generate embeddings, choose schemas, or answer the user.

The agent-owned ingestion flow is:

1. read or extract source content with tools/services,
2. track batch/source state with ingestion service functions,
3. use LLM reasoning through the model boundary to classify and structure
   facts, entities, relations, and citations,
4. call `embedding_Embed`,
5. call canonical indexer upsert functions for documents, chunks, facts,
   entities, relations, and citations,
6. query with `embedding_Embed` and `indexer_QueryContext` or
   `indexer_GetContext`,
7. verify grounding with citation functions,
8. answer from returned context and citations.

Directory indexing reads files in place. Sidecars, renames, and restructuring
are optional workspace-organization actions that require explicit approval.

## Catalogs

Supervisor passes runtime startup contracts through:

- `QUARK_RUNTIME_PLUGIN_CATALOG`
- `QUARK_RUNTIME_SERVICE_CATALOG`

Catalogs are versioned. Runtime rejects unsupported versions and does not fall
back to filesystem discovery for supervisor-launched agents.

## Observability

Runtime emits redacted activity records for messages, workflow detection,
tool/service-function starts and results, model usage, policy denials, and
errors. Core can persist those records when the Core service is present.

Correlation fields:

- `session_id` identifies the user-visible conversation.
- `run_id` identifies one user-message execution.
- `workflow_id` identifies runtime workflow guard state.
- `service_call_id` identifies one service-function/tool call.
- `request_id` is provider-owned model request identity when available.

E2E artifacts include prompt hashes, tool timelines, service timelines,
model-usage timelines, diagnostics, and redacted manual verification files.

## Boundaries

Architecture boundary checks live in `architecture/ownership.json` and are
enforced by:

```bash
make arch-check
```

Strict data-flow rules:

- ingress DTOs stay at their boundary and are mapped before execution logic,
- maps, slices, raw JSON, and bytes are copied across ownership boundaries,
- packages must not import another package only to reuse a data shape,
- services must not call each other,
- runtime must not own supervisor discovery,
- tests must not pre-extract or pre-index data that the agent is supposed to
  process.
