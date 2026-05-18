# Development

## Requirements

- Go 1.26
- `make`
- Docker for Dgraph-backed E2E tests
- `pdftotext` for PDF E2E tests
- `staticcheck` for `make dead-code-check`

Install staticcheck with the active Go toolchain:

```bash
go install honnef.co/go/tools/cmd/staticcheck@latest
```

## Common Commands

```bash
make build
make build-plugins
make proto
make test
make vet
make fmt
make arch-check
make dead-code-check
make check
```

Focused package commands:

```bash
cd runtime && go test ./pkg/agent ./pkg/llm ./pkg/services ./pkg/workspace
cd supervisor && go test ./pkg/server ./pkg/runtime/...
cd services/indexer && go test ./...
cd services/embedding && go test ./...
cd plugins/tools/fs && go test ./...
cd cli && go test ./...
cd e2e && go test -tags e2e -run '^$' ./...
```

## Service Manager

The CLI talks to supervisor for service state. It does not inspect service
files directly.

```bash
quark services list
quark services status indexer
quark services inspect indexer
quark services logs indexer
quark services doctor
quark services restart indexer
```

Use these commands before debugging runtime prompts. If runtime cannot see a
service function, first verify that supervisor discovered the service plugin,
checked readiness, and included the function in the runtime service catalog.

## E2E

The local deterministic subset does not require provider credentials:

```bash
make test-e2e-local
```

Provider-backed tests require a tool-calling model provider:

```bash
export OPENROUTER_API_KEY=sk-or-v1-...
export OPENROUTER_E2E_MODEL=openai/gpt-4o-mini
make test-e2e
```

OpenRouter embedding coverage can be enabled with:

```bash
export OPENROUTER_E2E_EMBEDDING_MODEL=nvidia/llama-nemotron-embed-vl-1b-v2:free
```

E2E startup order is standardized:

1. build binaries/plugins,
2. start supervisor,
3. create a space through supervisor APIs,
4. install plugins and service plugins into the space layout,
5. start external dependencies and local services,
6. start runtime,
7. create sessions,
8. send user-style prompts.

E2E artifacts are written into the test temp directory and include redacted
tool timelines, service timelines, prompt hashes, model/embedding snapshots,
model-usage timelines, diagnostics, and manual verification files.

## Troubleshooting

- `pdftotext` missing: install Poppler utilities.
- Dgraph startup fails: verify Docker is running and can pull
  `dgraph/standalone:v25.0.0`.
- `dead-code-check` reports Go version mismatches: reinstall staticcheck with
  the current Go toolchain.
- Provider-backed E2E skips on quota/auth: verify `OPENROUTER_API_KEY` and
  provider model access.
- Runtime cannot call a service function: run `quark services doctor` and
  inspect the resolved service catalog before changing prompts.
- Policy denied errors: check the active agent profile and any Quarkfile
  permission narrowing.

## Release Readiness

Before publishing a release or large PR, run:

```bash
make check
make build
make build-plugins
make test-e2e-local
```

The canonical release gate is:

```bash
make release-check
```

Run provider-backed E2E before release candidates when provider credentials and
quota are available. See [RELEASE.md](RELEASE.md).
