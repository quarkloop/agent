# Quark

[![CI](https://github.com/quarkloop/agent/actions/workflows/ci.yml/badge.svg)](https://github.com/quarkloop/agent/actions/workflows/ci.yml)
[![Go 1.26+](https://img.shields.io/badge/go-1.26+-00ADD8.svg)](https://go.dev/dl/)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

A local operating environment for autonomous AI workspaces — isolated spaces, plugin-defined identities, typed service functions, tool execution, model/provider routing, and a supervisor that owns lifecycle and persistent state.

## Overview

Quark gives agents isolated spaces, plugin-defined identities, typed NATS service functions, tool execution, model/provider routing, and a supervisor that owns lifecycle and persistent state. The project is production-shaped by design: explicit ownership boundaries, NATS-native service-function contracts, supervisor-owned discovery, real supervisor/runtime E2E tests, redacted observability artifacts, and strict data-flow rules.

The core product shape is simple: the agent reasons and coordinates; services execute typed deterministic work; the supervisor owns discovery and lifecycle.

## Features

- **Spaces** — service-owned workspaces with one authoritative `space.json` configuration record
- **Supervisor** — control plane for spaces, sessions, plugin installs, service discovery, readiness, catalogs, and embedded NATS
- **Runtime** — agent loop, profile prompts, LLM/model calls, tool execution, service-function dispatch, permissions, activity, and workflow guards
- **Agents** — required Quark Main coordinator plus installable Knowledge, DevOps, and System specialist profiles
- **Services** — typed NATS service functions for Gateway, Core, Document, Run State, Indexer, Citation, DevOps, System, Space, Secrets, Workflow, and IO
- **Observability** — redacted activity, tool/service timelines, model usage records, diagnostics, and E2E artifacts
- **Plugin system** — tool, service, agent, and skill plugins with manifest-driven installation
- **Multi-provider Gateway** — OpenRouter, OpenAI, and Anthropic support with fallback, usage tracking, and quota enforcement

## Installation

```bash
git clone https://github.com/quarkloop/agent
cd agent
make build
export PATH="$PWD/bin:$PATH"
```

See [Development](docs/development.mdx) for full prerequisites (Go 1.26+, Rust/Cargo for the Harness service, Docker Compose for E2E stacks).

## Quick start

Start the NATS control plane and base services:

```bash
cp .env.example .env
# set OPENROUTER_API_KEY in .env before model-backed agent runs
docker compose -f deploy/compose/quark.yml --profile services --profile gateway --profile runtime up --build -d
export QUARK_NATS_URL=nats://127.0.0.1:4222
export QUARK_NATS_USER=quark-control
export QUARK_NATS_PASSWORD=quark-control-dev
```

Create a space and start a session:

```bash
mkdir /tmp/quark-demo
quark init quark-demo --work-dir /tmp/quark-demo
export QUARK_SPACE=quark-demo
export OPENROUTER_API_KEY=sk-or-v1-...
quark session create --title "Demo"
```

See [Space configuration](docs/space-config.mdx) for Knowledge, DevOps, System, and Gateway configuration examples.

## Documentation

- [Architecture](docs/architecture.mdx) — process model, plugin types, services, catalogs, and strict boundaries
- [Development](docs/development.mdx) — build, test, E2E, provider keys, and debugging
- [Space configuration](docs/space-config.mdx) — authoritative `space.json` configuration examples
- [Release readiness](docs/release.mdx) — release gates and manual checks
- [Agent guide](AGENTS.md) — coding-agent instructions and repository rules
- [Contributing](CONTRIBUTING.md) — contribution expectations and code style
- [Security](SECURITY.md) — vulnerability reporting policy
- [Changelog](CHANGELOG.md) — release history

## Compatibility

| Component | Language | Version |
|---|---|---|
| Supervisor, Runtime, CLI, Services | Go | 1.26+ |
| Harness service | Rust | stable (Cargo) |
| Web UI | TypeScript | Next.js |
| NATS | — | 2.10+ (embedded in supervisor) |
| Dgraph (for Indexer E2E) | — | v25.0.0 (Docker) |

## Contributing

Pull requests are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, commit message conventions, and code style rules. By participating you agree to abide by the [Code of Conduct](CODE_OF_CONDUCT.md).

## License

This project is licensed under the Apache License, Version 2.0 — see the [LICENSE](LICENSE) file for details.
