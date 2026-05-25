# Quark

[![CI](https://github.com/quarkloop/quark/actions/workflows/ci.yml/badge.svg)](https://github.com/quarkloop/quark/actions/workflows/ci.yml)
[![Go 1.26+](https://img.shields.io/badge/go-1.26+-00ADD8.svg)](https://go.dev/dl/)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

Quark is a local operating environment for autonomous AI workspaces. It gives
agents isolated spaces, plugin-defined identities, typed service functions,
tool execution, model/provider routing, and a supervisor that owns lifecycle
and persistent state.

The project is production-shaped by design: explicit ownership boundaries,
NATS-native service-function contracts, supervisor-owned discovery, real
supervisor/runtime E2E tests, redacted observability artifacts, and strict
data-flow rules.

## What You Get

| Area | What it does |
| --- | --- |
| Spaces | Service-owned workspaces with one authoritative `space.json` configuration record. |
| Supervisor | Control plane for spaces, sessions, plugin installs, service discovery, readiness, catalogs, and embedded NATS. |
| Runtime | Agent loop, profile prompts, LLM/model calls, tool execution, service-function dispatch, permissions, activity, and workflow guards. |
| Agents | Required Quark Main coordinator plus installable Knowledge, DevOps, and System specialist profiles. |
| Services | Typed NATS service functions for Gateway, core, document, Run State, indexer, citation, DevOps, System, and Space. |
| Observability | Redacted activity, tool/service timelines, model usage records, diagnostics, and E2E artifacts. |

The core product shape is simple: the agent reasons and coordinates; services
execute typed deterministic work; supervisor owns discovery and lifecycle.

## Quickstart

```bash
git clone https://github.com/quarkloop/quark
cd quark
make build
export PATH="$PWD/bin:$PATH"
```

Start the Space service and supervisor:

```bash
space-service --root ~/.quarkloop/spaces
supervisor start --port 7200
```

Create a space and select it for subsequent CLI requests:

```bash
mkdir /tmp/quark-demo
quark init quark-demo --work-dir /tmp/quark-demo
export QUARK_SPACE=quark-demo
export OPENROUTER_API_KEY=sk-or-v1-...
quark session create --title "Demo"
```

The CLI talks to supervisor/runtime contracts through NATS. The Space service
persists the authoritative `space.json` record under `$QUARK_SPACES_ROOT` or
`~/.quarkloop/spaces`; it does not write hidden product state into the working
directory.

See [SPACE-CONFIG.md](SPACE-CONFIG.md) for Knowledge, DevOps, System, and Gateway
configuration examples.

## Architecture

```text
quark CLI
  |
  | NATS request/reply and streams
  v
supervisor  -> orchestration, sessions, discovery, catalogs, embedded NATS
  |
  | publishes resolved catalogs and account credentials
  v
runtime     -> agent loop, prompts, tools, service functions, activity
  |
  | tool calls and NATS service functions
  v
plugins/tools/*     services/* (including Space persistence)     plugins/agents/*
```

Core rule: agents coordinate, services execute. Services do not call each
other, and runtime does not rediscover plugins or services once supervisor has
resolved the catalog.

Read the deeper architecture notes in [ARCHITECTURE.md](ARCHITECTURE.md).

## Build And Test

```bash
make build           # cli, supervisor, runtime, tools, services
make build-plugins   # tool plugin build targets
make test            # unit tests across workspace modules
make test-e2e-local  # configured local E2E scenarios
make test-e2e        # provider-backed E2E suite
make check           # fmt-check, vet, test, arch-check, dead-code-check
make release-check   # release readiness gate
```

See [DEVELOPMENT.md](DEVELOPMENT.md) for setup, E2E requirements, provider
keys, troubleshooting, and release checks.

## Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md) - process model, plugins, services,
  catalogs, and strict boundaries.
- [DEVELOPMENT.md](DEVELOPMENT.md) - build, test, E2E, provider keys, and
  debugging.
- [SPACE-CONFIG.md](SPACE-CONFIG.md) - authoritative space configuration examples.
- [RELEASE.md](RELEASE.md) - release readiness gates and manual checks.
- [AGENTS.md](AGENTS.md) - coding-agent instructions and repository rules.
- [CONTRIBUTING.md](CONTRIBUTING.md) - contribution expectations.
- [SECURITY.md](SECURITY.md) - security policy.

## Status

Quark is under active development. The supervisor/runtime/plugin/service
foundation is in place, with service-backed Knowledge, DevOps, and System
flows covered by product-level tests and a final E2E verification gate.

Issues and PRs are welcome. Please keep changes scoped, add tests that match
the risk, and use conventional commit messages such as
`feat: add service function catalog`.
