# Quark

[![CI](https://github.com/quarkloop/quark/actions/workflows/ci.yml/badge.svg)](https://github.com/quarkloop/quark/actions/workflows/ci.yml)
[![Go 1.26+](https://img.shields.io/badge/go-1.26+-00ADD8.svg)](https://go.dev/dl/)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

Quark is a local operating environment for autonomous AI workspaces. It gives
agents isolated spaces, plugin-defined identities, typed service functions,
tool execution, and a supervisor that owns lifecycle and persistent state.

The project is early, but it is intentionally production-shaped: explicit
ownership boundaries, gRPC service contracts, real supervisor/runtime E2E
tests, and strict data-flow rules.

## What You Get

- Local-first spaces with one `Quarkfile` in the working directory.
- A supervisor control plane for spaces, sessions, plugins, service discovery,
  readiness checks, catalogs, and runtime lifecycle.
- A runtime execution engine for agent profiles, sessions, prompts, LLM calls,
  tool execution, service functions, and activity streams.
- Agent plugins for launch profiles: Quark Knowledge, Quark DevOps, and Quark
  System.
- Service plugins that expose gRPC-backed service functions through the normal
  tool-call loop.
- Knowledge flows where the agent reads files, uses the LLM for semantic
  extraction, calls embeddings, stores canonical index records, and answers
  from retrieved context.

## Quickstart

```bash
git clone https://github.com/quarkloop/quark
cd quark
make build
export PATH="$PWD/bin:$PATH"
```

Start the supervisor:

```bash
supervisor start --port 7200 --runtime ./bin/runtime
```

Create and run a space:

```bash
mkdir /tmp/quark-demo
cd /tmp/quark-demo
quark init --name quark-demo
export OPENROUTER_API_KEY=sk-or-v1-...
quark run
quark session create --title "Demo"
```

The CLI is an HTTP client. The supervisor stores space state under
`$QUARK_SPACES_ROOT` or `~/.quarkloop/spaces`.

## Architecture

```text
quark CLI
  |
  | HTTP
  v
supervisor  -> spaces, sessions, discovery, catalogs, runtime lifecycle
  |
  | launches with resolved catalogs
  v
runtime     -> agent loop, prompts, tools, service functions, activity
  |
  | tool calls and gRPC service functions
  v
plugins/tools/*     services/*     providers/*     plugins/agents/*
```

Core rule: agents coordinate, services execute. Services do not call each
other, and runtime does not rediscover plugins or services once supervisor has
resolved the catalog.

Read the deeper architecture notes in [ARCHITECTURE.md](ARCHITECTURE.md).

## Build And Test

```bash
make build           # cli, supervisor, runtime, tools, services
make build-plugins   # tool .so files and provider .so files
make test            # unit tests across workspace modules
make test-e2e-local  # deterministic E2E subset, no provider key
make test-e2e        # provider-backed E2E suite
make check           # fmt-check, vet, test, arch-check, dead-code-check
make release-check   # release readiness gate with local deterministic E2E
```

See [DEVELOPMENT.md](DEVELOPMENT.md) for setup, E2E requirements, provider
keys, troubleshooting, and release checks.

## Documentation

- [ARCHITECTURE.md](ARCHITECTURE.md) - process model, plugins, services,
  catalogs, and strict boundaries.
- [DEVELOPMENT.md](DEVELOPMENT.md) - build, test, E2E, provider keys, and
  debugging.
- [RELEASE.md](RELEASE.md) - release readiness gates and manual checks.
- [AGENTS.md](AGENTS.md) - coding-agent instructions and repository rules.
- [CONTRIBUTING.md](CONTRIBUTING.md) - contribution expectations.
- [SECURITY.md](SECURITY.md) - security policy.

## Status

Quark is under active development. The supervisor/runtime/plugin/service
foundation is in place, and the Knowledge indexing flow is being hardened with
real service-backed E2E tests.

Issues and PRs are welcome. Please keep changes scoped, add tests that match
the risk, and use conventional commit messages such as
`feat: add service function catalog`.
