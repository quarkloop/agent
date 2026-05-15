# Quark

[![Go 1.26+](https://img.shields.io/badge/go-1.26+-00ADD8.svg)](https://go.dev/dl/)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

Quark is a local runtime for autonomous AI workspaces. It gives an agent a
space, tools, service-backed knowledge, and a supervisor that owns lifecycle and
state, while keeping user files on your machine.

The project is early but intentionally production-shaped: typed service
contracts, explicit ownership boundaries, real end-to-end tests, and a runtime
designed to coordinate tools rather than hide shortcuts inside services.

## Why Quark

- Local-first spaces with a single `Quarkfile` in your working directory.
- Supervisor-owned discovery for tools, providers, service plugins, skills, and
  runtime catalogs.
- Agent plugins for launch profiles such as Quark Knowledge, Quark DevOps, and
  Quark System.
- Tool-call-only invocation: filesystem, shell, web search, and service
  functions all flow through one runtime execution envelope.
- gRPC services for durable platform behavior such as indexing, embeddings,
  release automation, and space metadata.
- Agent-led document ingestion: read files, extract structure, embed chunks,
  index canonical records, then query grounded context.

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

The CLI talks to the supervisor over HTTP. The supervisor owns persistent space
state under `$QUARK_SPACES_ROOT` or `~/.quarkloop/spaces`.

## Architecture

```text
quark CLI
  |
  | HTTP
  v
supervisor
  |  owns spaces, sessions, plugin installs, runtime lifecycle, catalogs
  |
  | launches with resolved plugin/service catalogs
  v
runtime
  |  owns agent loop, sessions, prompts, tool execution, extraction profiles
  |
  | tool calls and service functions
  v
plugins/tools/*       services/* over gRPC       providers/* over lib plugins
```

Important boundaries:

- The supervisor owns discovery. Runtime consumes resolved catalogs and does
  not infer installed state for supervisor-launched agents.
- Supervisor-to-runtime plugin and service catalogs are versioned startup
  contracts and unsupported versions fail at runtime startup.
- Service plugins declare gRPC health/readiness requirements; supervisor checks
  endpoint health, descriptor version, and RPC coverage before runtime launch.
- Services do not call each other. The agent is the coordinator.
- Service functions are the agent-facing callable service capabilities; RPC
  methods are only the gRPC transport implementation.
- Agent prompts and personalities live in agent plugins. Runtime executes the
  supervisor-resolved profile instead of owning launch-agent identities.
- `Quarkfile` may narrow installed agent profiles and override model,
  provider, approval, memory, and enabled-agent selection; supervisor validates
  those overrides before runtime launch.
- The indexer owns canonical storage/query records only; it does not parse
  files, call LLMs, generate embeddings, or choose extraction schemas.
- Embeddings are service plugins. Local deterministic and OpenRouter-backed
  embeddings use the same gRPC contract.
- Directory indexing reads files in place. Sidecars and renames are optional
  user-approved workspace organization, not indexing dependencies.

## Repository Layout

| Path | Purpose |
| --- | --- |
| `cli` | `quark` CLI, an HTTP client for supervisor/runtime APIs |
| `supervisor` | daemon that owns spaces, sessions, plugins, runtime lifecycle |
| `runtime` | agent loop, tools, service catalog consumption, extraction profiles |
| `pkg/plugin` | shared plugin manifest and interface contracts |
| `pkg/serviceapi` | protobuf/gRPC contracts and service helpers |
| `pkg/space` | shared Quarkfile schema and space model |
| `pkg/toolkit` | common toolkit for tool plugin CLI/API/lib modes |
| `plugins/tools/*` | bash, fs, web-search, build-release tool plugins |
| `plugins/agents/*` | Quark Knowledge, Quark DevOps, and Quark System agent profiles |
| `plugins/providers/*` | OpenRouter, OpenAI, Anthropic provider plugins |
| `plugins/services/*` | service plugin manifests and `SKILL.md` files |
| `services/*` | gRPC services: indexer, embedding, build-release, space |
| `e2e` | real supervisor/runtime/service E2E tests |

## Services And Knowledge

The indexer service stores canonical GraphRAG records:

- document metadata and source provenance
- text chunks and embedding metadata
- entities, relations, facts, and citations
- normalized retrieval scores and structured context packages

The runtime agent coordinates ingestion by calling tools, LLM reasoning, and
service functions in order. For PDFs, the expected flow is: read/extract file
content, use the LLM to classify and structure facts/entities/relations,
`embedding_Embed`, `indexer_IndexDocument`, then `embedding_Embed` plus
`indexer_GetContext` to answer questions from indexed evidence.

## Build And Test

```bash
make build           # binaries for cli, supervisor, runtime, tools, services
make build-plugins   # tool .so files and provider .so files
make proto           # regenerate protobuf/gRPC stubs
make test            # unit tests across workspace modules
make test-e2e        # real E2E tests, requires provider credentials
make vet
make fmt
```

Focused E2E:

```bash
go test -tags e2e -v -timeout 10m ./e2e
go test -count=1 -tags e2e -v -timeout 10m ./e2e
```

The PDF dataset E2E requires `pdftotext`, Dgraph from the E2E helper, and a
tool-calling provider. OpenRouter embedding coverage uses:

```bash
OPENROUTER_E2E_EMBEDDING_MODEL=nvidia/llama-nemotron-embed-vl-1b-v2:free
```

## Documentation

- [AGENTS.md](AGENTS.md) - coding-agent instructions, boundaries, commands,
  commit rules.
- [docs/services.md](docs/services.md) - service architecture, service plugins,
  embedding/indexer flows.
- [docs/running-and-debugging.md](docs/running-and-debugging.md) - local
  commands, debugging, E2E notes.
- [CONTRIBUTING.md](CONTRIBUTING.md) - contribution expectations.
- [SECURITY.md](SECURITY.md) - security policy.

## Project Status

Quark is under active development. The core supervisor/runtime/plugin/service
architecture is in place, and the indexer/embedding document flow is being
hardened through real E2E tests. Expect APIs and docs to evolve quickly.

Production-readiness work is tracked through tests, service boundaries, strict
data-flow rules, and manual CLI verification rather than marketing claims.

## Contributing

Issues and PRs are welcome. Please keep changes scoped, add tests that match the
risk, and respect the package ownership boundaries documented in `AGENTS.md`.
Use conventional commit messages such as `feat: add service plugin catalog`.
