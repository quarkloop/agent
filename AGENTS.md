# Quark Agent Instructions

Quark is a Go 1.26 workspace for a local autonomous-agent runtime. Treat this
repository as production-oriented software: no shortcuts, no hidden service
coupling, no DTO leakage across ownership boundaries, and no commits that mix
unrelated scopes.

## Architecture Boundaries

- `supervisor` owns high-level space/session orchestration, plugin installs,
  embedded NATS, account setup, runtime leases, and discovery catalogs.
- `runtime` owns the agent loop, sessions, tool execution, workspace sidecar
  policy, and consumption of supervisor-resolved catalogs. Harness owns
  model-context packaging from plugin prompt material and runtime facts.
- `cli` is a NATS client. It selects a space through `--space` or
  `QUARK_SPACE` and delegates state operations to supervisor or the resolved
  runtime.
- `services/*` own durable domain behavior behind protobuf-backed NATS
  service-function contracts.
  Services expose agent-facing service functions and must not call each other.
- `services/space` owns the authoritative `space.json` record and low-level
  space configuration persistence.
- `plugins/tools/*` expose agent-callable tool plugins in lib and/or api mode.
- `plugins/services/*` contain service plugin manifests and `SKILL.md`
  guidance for NATS services and their exported service functions.
- `plugins/agents/*` contain agent plugin manifests, PROFILE.yaml, SYSTEM.md,
  and SKILL.md files.
- `pkg/serviceapi` owns protobuf contracts and NATS service-function helpers.
- `pkg/plugin`, `pkg/space`, `pkg/toolkit`, and `pkg/event` are shared support
  packages.

The agent is the coordinator. For document indexing it reads files, uses LLM
reasoning for semantic structure, calls service functions for mechanical work,
sends canonical records to the indexer, and verifies retrieval. The indexer
stores/query canonical knowledge records only; it does not parse files, call
LLMs, embed text, select schemas, or call another service.

## Modules

The workspace modules are listed in `go.work`:

- `cli`
- `supervisor`
- `runtime`
- `e2e`
- `pkg/boundary`, `pkg/event`, `pkg/natskit`, `pkg/plugin`,
  `pkg/serviceapi`, `pkg/space`, `pkg/toolkit`
- `services/citation`, `services/core`, `services/devops`,
  `services/document`, `services/indexer`, `services/runstate`,
  `services/gateway`, `services/secrets`, `services/space`,
  `services/system`, `services/workflow`
- `services/io`
- `plugins/agents/quark-knowledge`, `plugins/agents/quark-devops`,
  `plugins/agents/quark-system`

## Runtime Packages

Important runtime packages:

- `runtime/pkg/agent`: thin orchestrator for session routing and lifecycle glue.
- `runtime/pkg/llm`: bounded LLM/tool loop and streamed tool-call trace events.
- `runtime/pkg/services`: supervisor-resolved NATS service catalog and generic
  service-backed tool executor.
- `supervisor/pkg/runtime/launchenv`: process environment builder for runtime
  launch specifications.
- `runtime/pkg/workspace`: approval-gated sidecar and directory mutation policy.
- `runtime/pkg/pluginmanager`: runtime loading of supervisor-provided plugin
  catalog entries.
- `runtime/pkg/harnessclient`: runtime boundary for Rust Harness context
  composition, context reports, and explicit memory operations.
- `runtime/pkg/message`, `runtime/pkg/api`, `runtime/pkg/channel/*`: request,
  stream, and channel boundaries.
- `pkg/boundary`: boundary error categories, diagnostics, and shared redaction
  helpers.

## Plugins And Services

Formal terms:

- `service function`: agent-facing callable service operation.
- `RPC method`: protobuf method descriptor implementing a service function.
- `tool call`: runtime execution envelope used by the LLM/function-calling loop.

Agent plugins own profile identity, SYSTEM.md, SKILL.md, default permissions,
handoff rules, and evaluation requirements. Everything agent-callable flows
through the runtime tool-call surface. Tool plugins own their schema,
implementation, and `SKILL.md`. Service plugins describe NATS service functions; runtime
turns their RPC descriptors into generated service functions such as
`gateway_Embed` and `indexer_QueryContext`.

`quark-main` is the required root coordinator agent plugin. Supervisor resolves
its allowed service functions from installed services and any space configuration
narrowing; runtime must not select a specialist agent as the root. Knowledge,
DevOps, and System agent profiles are delegate plugins.

`space.json` is the Space-service-owned override record for installed agent
profiles. It may select enabled profiles, model/provider overrides,
service/tool narrowing, approval policy, and memory scope. Supervisor obtains
it through Space service functions, validates overrides against the installed
profile maximum, and passes only the resolved catalog to runtime. Product
paths do not create or read legacy working-directory manifests.

Service plugins must declare NATS service-function health/readiness
requirements. Supervisor validates descriptor version, subject metadata, and
exported RPC descriptors before adding a service to the runtime catalog.

Every runtime tool/service-function stream event must preserve redacted
correlation fields where available: `session_id`, `run_id`, `workflow_id`,
`service_call_id`, provider `request_id`, and artifact IDs. Diagnostics should
use boundary categories instead of raw process noise.

Supervisor-owned discovery publishes versioned runtime catalogs through NATS
contracts. Runtime must reject unsupported catalog versions with actionable
errors and consume catalogs as explicit startup input. Do not add runtime
filesystem discovery for supervisor-launched agents.

## Strict Redlines

- Follow `docs/stricts.md` for data-flow ownership.
- Do not pass ingress DTOs into domain packages.
- Do not import another package only to reuse a data shape.
- Copy maps and slices when crossing ownership boundaries.
- Do not mutate user directories during indexing unless the user explicitly
  approves a separate workspace-organization action.
- Do not make services call each other.
- Do not reintroduce a runtime "capability" abstraction. Tool calls are the
  only agent-callable execution envelope; services are exposed as service
  functions through that path.
- Do not hide failures in prompts, tests, or timeout bumps.
- Do not commit changes under `docs/`. The local task tracker and docs drafts
  can change in the workspace, but they must stay out of commits.

## Build And Test

From the repository root:

```bash
make build
make build-plugins
make proto
make test
make vet
make fmt
make arch-check
make dead-code-check
make test-e2e-local
```

Common focused commands:

```bash
cd runtime && go test ./pkg/agent ./pkg/llm ./pkg/services ./pkg/harnessclient ./pkg/workspace
cd runtime && go test ./pkg/activity ./pkg/harnessclient ./pkg/permissions
cd services/indexer && go test ./...
cd services/gateway && go test ./...
cd services/io && go test ./...
cd cli && go test ./pkg/commands/services
cd e2e && go test -tags e2e -run '^$' ./...
```

Full E2E belongs at the final verification gate after implementation work:

```bash
make test-e2e
go test -count=1 -tags e2e -v -timeout 10m ./e2e
```

## E2E Expectations

E2E tests start real supervisor/runtime/service processes. The standard order
is:

1. build binaries/plugins,
2. start supervisor,
3. create a space through supervisor APIs,
4. install plugins/service plugins through supervisor-owned layout,
5. prepare external dependencies,
6. start runtime,
7. create sessions,
8. send user-style prompts.

Logs must use `[e2e]` prefixes and preserve process ownership. PDF indexing
tests must let the agent read PDFs and use services; tests must not pre-extract
text or construct index payloads.

## Git Rules

- Commit per scope.
- Use conventional messages: `{type}: {description}`.
- Do not include `Co-authored-by`.
- Inspect staged files before every commit.
- Never stage `docs/` changes.
- Do not use destructive git commands unless explicitly requested.
