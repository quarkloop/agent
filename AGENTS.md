# Agent Guide

This is the Quark Agent — a Go 1.26 workspace for a local autonomous-agent runtime with supervisor, runtime, CLI, services, and plugins. Your job is to treat this repository as production-oriented software: no shortcuts, no hidden service coupling, no DTO leakage across ownership boundaries, and no commits that mix unrelated scopes.

## Repository

- **Name**: Quark Agent
- **Language**: Go 1.26+ (workspace), Rust (Harness service)
- **License**: Apache 2.0
- **Repo**: [github.com/quarkloop/agent](https://github.com/quarkloop/agent)
- **Guidelines**: [quarkloop/guidelines](https://github.com/quarkloop/guidelines)

## Quick reference

```bash
# Build everything (Go binaries + Rust Harness service)
make build

# Build tool plugins
make build-plugins

# Regenerate protobuf/gRPC Go stubs
make proto

# Unit tests across all modules
make test

# Go vet across all modules
make vet

# Go fmt across all modules (in-place)
make fmt

# Architecture boundary checks (ownership + import rules)
make arch-check

# Dead-code check via staticcheck
make dead-code-check

# Provider-independent E2E (no API key needed)
make test-e2e-local

# Provider-backed E2E (requires OPENROUTER_API_KEY)
make test-e2e

# Full pre-commit gate: fmt-check + vet + test + arch-check + dead-code-check
make check

# Release readiness gate
make release-check
```

Focused package commands:

```bash
# Runtime core packages
cd runtime && go test ./pkg/agent ./pkg/llm ./pkg/services ./pkg/harnessclient ./pkg/workspace
cd runtime && go test ./pkg/activity ./pkg/harnessclient ./pkg/permissions

# Individual services
cd services/indexer && go test ./...
cd services/gateway && go test ./...
cd services/io && go test ./...

# CLI
cd cli && go test ./pkg/commands/services

# E2E (compile-check only, no execution)
cd e2e && go test -tags e2e -run '^$' ./...
```

## Structure

```
agent/
├── cli/                           # Go CLI (quarkctl) — NATS client for supervisor/runtime
│   ├── cmd/quark/main.go          # entry point
│   ├── pkg/commands/              # Cobra commands (chat, services, plugin, audit, etc.)
│   ├── pkg/natsclient/            # NATS client wrappers for supervisor/runtime APIs
│   ├── pkg/buildinfo/             # version info injected at build time
│   ├── pkg/spacecontext/          # space selection (--space or QUARK_SPACE)
│   └── pkg/runtimeconnect/        # NATS connection helper
│
├── supervisor/                    # CONTROL PLANE — Go binary
│   ├── cmd/supervisor/main.go     # entry point
│   ├── pkg/server/                # service catalog, plugin catalog, agent profiles
│   ├── pkg/natshub/               # embedded NATS server, accounts, permissions
│   ├── pkg/natsapi/               # NATS API surface (sessions, spaces, plugins, services)
│   ├── pkg/space/                 # space store (local + remote)
│   ├── pkg/sessions/              # session lifecycle
│   ├── pkg/pluginmanager/         # plugin registry, remote plugin loading
│   ├── pkg/deployment/            # runtime deployment lifecycle
│   ├── pkg/events/                # event types
│   └── pkg/toolkit/               # supervisor-side toolkit
│
├── runtime/                       # EXECUTION ENGINE — Go binary
│   ├── cmd/runtime/main.go        # entry point
│   ├── pkg/agent/                 # thin orchestrator for session routing + lifecycle
│   ├── pkg/llm/                   # bounded LLM/tool loop + streamed tool-call trace events
│   ├── pkg/loop/                  # main agent loop, handler, middleware, message
│   ├── pkg/services/              # supervisor-resolved NATS service catalog + tool executor
│   ├── pkg/execution/             # runtime execution, middleware, mode
│   ├── pkg/workspace/             # approval-gated sidecar + directory mutation policy
│   ├── pkg/pluginmanager/         # runtime loading of supervisor-provided plugin catalog
│   ├── pkg/harnessclient/         # Rust Harness context composition + memory operations
│   ├── pkg/channel/               # request/stream/channel boundaries (NATS, Telegram)
│   ├── pkg/message/               # message types, handle
│   ├── pkg/session/               # session management
│   ├── pkg/activity/              # redacted activity records, store
│   ├── pkg/permissions/           # permission checker
│   ├── pkg/toolpolicy/            # tool policy enforcement
│   ├── pkg/handoff/               # agent handoff policy
│   ├── pkg/hierarchy/             # agent hierarchy, delegation, spawn
│   ├── pkg/dag/                   # DAG executor
│   ├── pkg/workflow/              # workflow detection, state, tracker, policy
│   ├── pkg/approval/              # approval gate, request
│   ├── pkg/plan/                  # plan management
│   ├── pkg/modelusage/            # model usage tracking, provider
│   ├── pkg/sourceid/              # source ID generation
│   ├── pkg/spaceauth/             # space auth credentials
│   ├── pkg/spacelease/            # space lease management
│   ├── pkg/startup/               # startup pipeline (env, catalogs, spaces, channels)
│   ├── pkg/runcontext/            # runtime context
│   ├── pkg/coreevents/            # event recorder
│   ├── pkg/catalogclient/         # catalog client
│   ├── pkg/gatewayclient/         # gateway client (model provider)
│   └── pkg/commands/              # runtime CLI commands (root, env, start)
│
├── services/                      # TYPED NATS SERVICE FUNCTIONS
│   ├── core/                      # health, readiness, audit, artifacts, approval, config
│   ├── gateway/                   # provider adapters, generation, embedding, fallback, usage
│   ├── document/                  # document extraction, PDF parsing
│   ├── indexer/                   # canonical knowledge records, Dgraph backend
│   ├── citation/                  # citation verification
│   ├── runstate/                  # durable run/item state
│   ├── devops/                    # repo, build, test, container, release, deploy
│   ├── system/                    # Linux snapshot, process, network, logs, metrics
│   ├── space/                     # authoritative space.json persistence
│   ├── secrets/                   # OpenBao-backed secret storage
│   ├── workflow/                  # Temporal workflow engine
│   ├── io/                        # file I/O, shell, search, fetch
│   └── harness/                   # Rust Harness service (context composition, memory)
│
├── plugins/                       # INSTALLABLE EXTENSIONS
│   ├── agents/                    # agent profiles (quark-main, quark-knowledge, quark-devops, quark-system)
│   │   └── quark-*/               # each has: manifest.yaml, PROFILE.yaml, SYSTEM.md, SKILL.md
│   └── services/                  # service plugin manifests + SKILL.md guidance
│       └── quark-service-*/       # each has: manifest.yaml, README.md, SKILL.md
│
├── pkg/                           # SHARED PACKAGES
│   ├── boundary/                  # error categories, diagnostics, redaction helpers
│   ├── event/                     # event types
│   ├── natskit/                   # shared NATS transport, subject contracts, envelopes
│   ├── plugin/                    # plugin loading, manifest, registry, provider
│   ├── serviceapi/                # protobuf contracts + NATS service-function helpers
│   │   └── gen/                   # generated protobuf Go stubs (do not edit)
│   ├── space/                     # space config, layout, validation, types
│   └── toolkit/                   # shared toolkit (server, pipe, CLI)
│
├── proto/                         # protobuf definitions
│   └── quark/*/v1/                # per-service .proto files
│
├── e2e/                           # E2E test suite
│   ├── *.go                       # test scenarios
│   ├── utils/                     # test utilities (compose, env, nats, client, etc.)
│   └── testdata/                  # test fixtures (PDFs, documents)
│
├── deploy/                        # deployment configs
│   ├── compose/                   # Docker Compose
│   ├── docker/                    # Dockerfiles
│   ├── systemd/                   # systemd unit files
│   ├── vector/                    # Vector log pipeline
│   └── victoria/                  # VictoriaMetrics config
│
├── web/                           # Next.js web UI
│   ├── src/                       # React components, hooks, providers
│   └── e2e/                       # Playwright tests
│
├── architecture/                  # architecture boundary definitions
│   ├── ownership.json             # package ownership rules (enforced by arch-check)
│   ├── nats-subjects.md           # NATS subject catalog
│   └── service-implementation-map.json  # code-owned service map
│
├── docs/                          # documentation (.mdx, synced to docs portal)
├── scripts/                       # Python scripts (arch-check, service-inventory, etc.)
├── go.work                        # Go workspace definition
├── Makefile                       # all build/test/run commands
└── buf.yaml                       # protobuf linting/breaking config
```

## Rules

### Architecture boundaries

1. `supervisor` owns high-level space/session orchestration, plugin installs, embedded NATS, account setup, runtime leases, and discovery catalogs.
2. `runtime` owns the agent loop, sessions, tool execution, workspace sidecar policy, and consumption of supervisor-resolved catalogs. Harness owns model-context packaging from plugin prompt material and runtime facts.
3. `cli` is a NATS client. It selects a space through `--space` or `QUARK_SPACE` and delegates state operations to supervisor or the resolved runtime.
4. `services/*` own durable domain behavior behind protobuf-backed NATS service-function contracts. Services expose agent-facing service functions and must not call each other.
5. `services/space` owns the authoritative `space.json` record and low-level space configuration persistence.
6. `plugins/tools/*` expose agent-callable tool plugins in lib and/or api mode.
7. `plugins/services/*` contain service plugin manifests and `SKILL.md` guidance for NATS services and their exported service functions.
8. `plugins/agents/*` contain agent plugin manifests, PROFILE.yaml, SYSTEM.md, and SKILL.md files.
9. `pkg/serviceapi` owns protobuf contracts and NATS service-function helpers.
10. `pkg/plugin`, `pkg/space`, `pkg/toolkit`, and `pkg/event` are shared support packages.

### Data-flow redlines

11. Do not pass ingress DTOs into domain packages.
12. Do not import another package only to reuse a data shape.
13. Copy maps and slices when crossing ownership boundaries.
14. Do not mutate user directories during indexing unless the user explicitly approves a separate workspace-organization action.
15. Do not make services call each other.
16. Do not reintroduce a runtime "capability" abstraction. Tool calls are the only agent-callable execution envelope; services are exposed as service functions through that path.
17. Do not hide failures in prompts, tests, or timeout bumps.

### Plugin and service rules

18. Agent plugins own profile identity, SYSTEM.md, SKILL.md, default permissions, handoff rules, and evaluation requirements.
19. Everything agent-callable flows through the runtime tool-call surface.
20. Tool plugins own their schema, implementation, and `SKILL.md`.
21. Service plugins describe NATS service functions; runtime turns their RPC descriptors into generated service functions such as `gateway_Embed` and `indexer_QueryContext`.
22. `quark-main` is the required root coordinator agent plugin. Supervisor resolves its allowed service functions from installed services and any space configuration narrowing; runtime must not select a specialist agent as the root.
23. Knowledge, DevOps, and System agent profiles are delegate plugins.
24. `space.json` is the Space-service-owned override record. Supervisor validates overrides against the installed profile maximum and passes only the resolved catalog to runtime.
25. Service plugins must declare NATS service-function health/readiness requirements. Supervisor validates descriptor version, subject metadata, and exported RPC descriptors before adding a service to the runtime catalog.

### Observability rules

26. Every runtime tool/service-function stream event must preserve redacted correlation fields: `session_id`, `run_id`, `workflow_id`, `service_call_id`, provider `request_id`, and artifact IDs.
27. Diagnostics should use boundary categories instead of raw process noise.
28. Supervisor-owned discovery publishes versioned runtime catalogs through NATS contracts. Runtime must reject unsupported catalog versions with actionable errors and consume catalogs as explicit startup input.
29. Do not add runtime filesystem discovery for supervisor-launched agents.

### Git and commit rules

30. Commit per scope — do not mix unrelated changes.
31. Use conventional messages: `type(scope): description`.
32. Do not include `Co-authored-by` trailers.
33. Inspect staged files before every commit.
34. Never stage `docs/` changes — those are local task tracking only and must stay out of commits.
35. Do not use destructive git commands unless explicitly requested.

### Code style

36. Run `make fmt` before every commit — gofmt all modules.
37. Run `make vet` before every commit — must pass with zero warnings.
38. Exported types and functions require doc comments.
39. Errors are wrapped with `fmt.Errorf("component: %w", err)` — never discarded silently.
40. Panics are reserved for constructors that validate compile-time invariants (the `Must*` pattern).
41. Do not add code under `docs/` — that directory is for documentation only.

## Boundaries

### Module dependency graph

```
cli ──► supervisor (via NATS)
cli ──► runtime (via NATS)

supervisor ──► pkg/natskit, pkg/plugin, pkg/serviceapi, pkg/space, pkg/toolkit
supervisor ──► services/* (via NATS, never direct imports)

runtime ──► pkg/natskit, pkg/plugin, pkg/serviceapi, pkg/space, pkg/toolkit, pkg/boundary
runtime ──► services/* (via NATS, never direct imports)
runtime ──► harness (via NATS)

services/* ──► pkg/natskit, pkg/serviceapi, pkg/boundary
services/* ──► services/* (NEVER — services must not call each other)

plugins/* ──► pkg/plugin (for manifest types only)
```

### Key invariants

- The agent is the coordinator. For document indexing it reads files, uses LLM reasoning for semantic structure, calls service functions for mechanical work, sends canonical records to the indexer, and verifies retrieval.
- The indexer stores/query canonical knowledge records only. It does not parse files, call LLMs, embed text, select schemas, or call another service.
- Supervisor resolves catalogs and passes them to runtime via environment variables (`QUARK_RUNTIME_PLUGIN_CATALOG`, `QUARK_RUNTIME_SERVICE_CATALOG`). Runtime does not rediscover plugins or services.
- Product paths do not create or read legacy working-directory manifests.

### Runtime packages (important ones)

- `runtime/pkg/agent`: thin orchestrator for session routing and lifecycle glue.
- `runtime/pkg/llm`: bounded LLM/tool loop and streamed tool-call trace events.
- `runtime/pkg/services`: supervisor-resolved NATS service catalog and generic service-backed tool executor.
- `runtime/pkg/workspace`: approval-gated sidecar and directory mutation policy.
- `runtime/pkg/pluginmanager`: runtime loading of supervisor-provided plugin catalog entries.
- `runtime/pkg/harnessclient`: runtime boundary for Rust Harness context composition, context reports, and explicit memory operations.
- `runtime/pkg/channel/*`: request, stream, and channel boundaries (NATS, Telegram).
- `pkg/boundary`: boundary error categories, diagnostics, and shared redaction helpers.

## Commit conventions

- Format: `type(scope): short summary`
- Types: `feat`, `fix`, `refactor`, `docs`, `test`, `chore`, `perf`, `build`, `ci`, `style`
- Scopes: `agent`, `llm`, `loop`, `services`, `workspace`, `pluginmanager`, `harnessclient`, `channel`, `supervisor`, `cli`, `gateway`, `indexer`, `document`, `devops`, `system`, `space`, `secrets`, `workflow`, `io`, `harness`, `core`, `citation`, `runstate`, `e2e`, `proto`, `web`, `deploy`
- One scope per commit — do not mix unrelated changes.
- Inspect staged files before every commit: `git diff --cached --stat`
- Never stage: `docs/` changes, `.env`, `node_modules/`, `bin/`, `vendor/`, editor configs.
- Do not include `Co-authored-by` trailers.
- Do not use destructive git commands unless explicitly requested.
- Examples:
  - `feat(agent): add cron session type`
  - `fix(llm): propagate io.ReadAll errors`
  - `refactor(services): tighten DevOps test contracts`
  - `docs(readme): add web UI setup instructions`
  - `test(e2e): rebuild PDF scenario around Compose`

## Testing

### Unit tests

- Run `make test` — executes unit tests across all workspace modules (no API key needed).
- Every new function needs a unit test in the same package (`_test.go` alongside source).
- Every bug fix needs a regression test that would have caught the bug.
- Tests must not pre-extract or pre-index data that the agent is supposed to process — let the agent do the work.

### Architecture checks

- Run `make arch-check` — validates package ownership and import boundaries against `architecture/ownership.json`.
- Run `make dead-code-check` — runs staticcheck U1000 across all modules (requires `staticcheck` installed).
- Run `make check` — the full pre-commit gate: fmt-check + vet + test + arch-check + dead-code-check.

### E2E tests

Provider-independent E2E (no API key needed):

```bash
make test-e2e-local
```

Provider-backed E2E (requires `OPENROUTER_API_KEY`):

```bash
export OPENROUTER_API_KEY=sk-or-v1-...
export OPENROUTER_E2E_MODEL=openrouter/owl-alpha
make test-e2e
```

E2E tests start real supervisor/runtime/service processes. The standard order is:

1. Build binaries/plugins.
2. Start supervisor.
3. Create a space through supervisor APIs.
4. Install plugins/service plugins through supervisor-owned layout.
5. Prepare external dependencies.
6. Start runtime.
7. Create sessions.
8. Send user-style prompts.

Logs must use `[e2e]` prefixes and preserve process ownership. PDF indexing tests must let the agent read PDFs and use services; tests must not pre-extract text or construct index payloads.

### Release readiness

```bash
make release-check
```

This verifies: Go 1.26 is active, formatting/vet/unit tests/architecture/dead-code checks pass, generated protobuf is current, all binaries build, and local E2E passes.

## When you're stuck

- Read `docs/architecture.mdx` for the process model, plugin types, services, and catalogs.
- Read `docs/development.mdx` for build, test, E2E, provider keys, and debugging.
- Read `docs/services.mdx` for the service architecture and NATS subject catalog.
- Read `docs/running-and-debugging.mdx` for practical debugging techniques.
- Read the [AGENTS.md spec](https://github.com/quarkloop/guidelines/blob/main/agents/SPEC.md) for org-wide conventions.
- Run `make arch-check` if you're unsure whether a change violates boundaries.
- Run `quark services doctor` if runtime cannot call a service function.
- Search existing issues and PRs before asking.
- If unsure about a boundary change, open an issue and ask before implementing.

## Common mistakes to avoid

1. **Making services call each other.** Services are isolated by design. If service A needs data from service B, the agent coordinates the call — A does not call B directly. This is the most important boundary in the codebase.

2. **Passing ingress DTOs into domain packages.** Ingress DTOs stay at their boundary and are mapped before execution logic. If a domain package imports an ingress DTO type, that's a boundary violation.

3. **Adding runtime filesystem discovery.** Supervisor owns discovery. Runtime consumes catalogs as explicit startup input (`QUARK_RUNTIME_PLUGIN_CATALOG`, `QUARK_RUNTIME_SERVICE_CATALOG`). Do not add filesystem scanning to runtime.

4. **Selecting a specialist agent as root.** `quark-main` is the required root coordinator. Knowledge, DevOps, and System are delegate plugins — they are never the root.

5. **Committing `docs/` changes.** The `docs/` directory is for local task tracking and drafts. Never stage it. The `.gitignore` does not exclude it because some docs are intentionally tracked — use judgment and inspect staged files before every commit.

6. **Pre-extracting data in tests.** E2E tests must let the agent read PDFs and use services. Tests must not pre-extract text or construct index payloads. If a test builds the answer and passes it to the agent, the test is wrong.

7. **Hiding failures in prompts or timeout bumps.** If a service function fails, surface the error with boundary categories. Do not retry silently, do not bump timeouts to make tests pass, do not inject error context into prompts.

8. **Reintroducing a "capability" abstraction.** Tool calls are the only agent-callable execution envelope. Services are exposed as service functions through that path. Do not add a separate capability layer.

9. **Mutating user directories during indexing.** Directory indexing reads files in place. Sidecars, renames, and restructuring require explicit user approval. Do not auto-create sidecar files or move user content.

10. **Skipping `make arch-check`.** The architecture boundary check catches import violations that would create coupling. Run it before every commit — it's part of `make check`.

## Rust Harness service

The Harness service (`services/harness/`) is the only Rust component in this repo. It owns model-context packaging from plugin prompt material and runtime facts.

- Build: `make build-harness-service` (runs `cargo build --release`)
- Test: `cargo test --manifest-path services/harness/Cargo.toml`
- Lint: `cargo clippy --manifest-path services/harness/Cargo.toml --all-targets -- -D warnings`
- Format: `cargo fmt --manifest-path services/harness/Cargo.toml`

The runtime talks to Harness via NATS (through `runtime/pkg/harnessclient`). Harness owns context composition, context reports, and explicit memory operations. Do not add LLM calls, file parsing, or service-to-service calls to Harness — it is a context packaging service only.
