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
cd services/gateway && go test ./...
cd services/io && go test ./...
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

Provider-independent E2E scenarios validate contracts, service endpoints, and
runtime lease behavior without making model-provider requests:

```bash
make test-e2e-local
```

Provider-backed tests run real model requests through Gateway using an allowed
OpenRouter model:

```bash
export OPENROUTER_API_KEY=sk-or-v1-...
export OPENROUTER_E2E_MODEL=openrouter/owl-alpha
make test-e2e
```

OpenRouter embedding coverage can be enabled with:

```bash
export OPENROUTER_E2E_EMBEDDING_MODEL=nvidia/llama-nemotron-embed-vl-1b-v2:free
```

E2E uses Docker Compose for process lifecycle. Test actions are standardized:

1. build the configured Compose service set,
2. wait for NATS-native service readiness,
3. create space/session state through service contracts,
4. start runtime workers for the test space,
5. send user-style prompts,
6. collect NATS diagnostics, Gateway usage, and service logs.

Provider-backed setup validates credentials and Gateway readiness without
spending a generation request. The scenario's agent interaction is the real
model call and its Gateway usage record is the verification evidence. For
bind-mounted workspace scenarios, Compose maps file-touching services through
`QUARK_WORKSPACE_CONTAINER_USER`; DevOps caches remain ephemeral in its
container.

`make test-e2e` executes each real-provider scenario in its own bounded test
process. This preserves per-scenario timeout and artifact isolation when
provider latency varies, then runs the provider-independent E2E coverage once.
`QUARK_E2E_MAX_PROVIDER_REQUESTS` is passed through to Gateway as an outbound
request ceiling so a scenario exceeding its budget is stopped before an
additional external generation or embedding request is dispatched.
The default provider-backed gate allocates at most 50 requests across the
integrated PDF knowledge, DevOps release, DevOps failure, System inspection,
and IO execution flows to remain executable with OpenRouter free-tier
credentials. These retained flows also verify runtime session admission and
direct index/audit persistence, avoiding duplicate model-consuming scenarios.

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
- Policy denied errors: check the active agent profile and any `space.json`
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
