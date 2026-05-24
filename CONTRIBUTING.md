# Contributing to Quark

Thank you for your interest in contributing. This guide covers everything you need to get started.

## Prerequisites

- **Go 1.26+** — Quark uses Go workspace mode; all 14 modules resolve locally
- An LLM provider API key if you want to run E2E tests (optional for unit tests)

## Getting started

```bash
git clone https://github.com/quarkloop/quark
cd quark
make build        # builds all 7 binaries into ./bin/
export PATH="$PWD/bin:$PATH"
```

## Running tests

```bash
make test         # unit tests across all 14 modules (no API key needed)
make vet          # go vet across all modules
make fmt          # gofmt all modules in-place
```

### E2E tests

E2E tests start real agent, bash, and fs processes and drive them through the shared HTTP client. They require an API key:

```bash
cp .env.example .env
# edit .env — set OPENROUTER_API_KEY or ZHIPU_API_KEY

make test-e2e
```

E2E tests run with a 10-minute timeout. See `agent/e2e/` for details.

## Module structure

Quark is a Go workspace (`go.work`) containing 14 independent modules. The dependency graph is strict — no circular imports:

```
pkg/plugin → supervisor/pkg/pluginmanager, plugins/tools/*
pkg/space → supervisor/pkg/space, cli/pkg/commands/*
supervisor → cli (via supervisor/pkg/client)
runtime → cli (via runtime/pkg/client)
```

When adding code, place it in the lowest-level module that needs it. Shared types go in `core` or `agent-api`. Agent logic goes in the appropriate `agent/pkg/<package>`. New tools follow the pattern in `tools/bash` or `tools/read`.

See `AGENTS.md` for a full breakdown of each module's role and the agent package structure.

## Code style

- Standard Go formatting — run `make fmt` before committing
- `make vet` must pass with no warnings
- Exported types and functions require doc comments
- Errors are wrapped with `fmt.Errorf("component: %w", err)` — never discarded silently
- Panics are reserved for constructors that validate compile-time invariants (the `Must*` pattern)

## Submitting changes

1. Fork the repository and create a branch from `main`
2. Make your changes — keep commits focused and atomic
3. Run `make vet && make test`
4. Open a pull request against `main` with a clear description of what and why
5. Link any related issues

### Commit message format

```
type(scope): short summary

Optional longer description explaining why the change was made.
```

Types: `feat`, `fix`, `refactor`, `docs`, `test`, `chore`

Examples:
- `feat(agent): add cron session type`
- `fix(web-search): propagate io.ReadAll errors`
- `docs(readme): add web UI setup instructions`

## Reporting bugs

Please use GitHub Issues with the bug report template. Include your Quark version (`quark version`), OS, Go version, and steps to reproduce.

## Feature requests

Open a GitHub Issue with the feature request template. Describe the problem you're solving and your proposed solution.

## Code of Conduct

Please note that this project is released with a [Contributor Code of Conduct](CODE_OF_CONDUCT.md). By participating in this project you agree to abide by its terms.

## License

By contributing you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).
