# Contributing to Quark

Thank you for your interest in contributing. This guide covers everything you need to get started.

## Prerequisites

- **Go 1.26+** - Quark uses Go workspace mode
- **Rust and Cargo** - required for the Harness service
- **Docker Compose** - required for E2E service stacks
- An LLM provider API key if you want to run E2E tests (optional for unit tests)

## Getting started

```bash
git clone https://github.com/quarkloop/agent
cd quark
make build        # builds Go binaries and the Rust Harness service
export PATH="$PWD/bin:$PATH"
```

## Running tests

```bash
make test         # unit tests across all 14 modules (no API key needed)
make vet          # go vet across all modules
make fmt          # gofmt all modules in-place
```

### E2E tests

Provider-independent E2E contract scenarios use Docker Compose and do not
make model-provider requests:

```bash
make test-e2e-local
```

Provider-backed E2E starts real NATS-native services and runtime workers
through Docker Compose, sends user-style prompts, and observes Gateway usage.
It requires an OpenRouter key and an approved E2E model:

```bash
cp .env.example .env
# edit .env - set OPENROUTER_API_KEY and OPENROUTER_E2E_MODEL

make test-e2e
```

See `e2e/` for scenarios and `DEVELOPMENT.md` for verification prerequisites.

## Module structure

Quark is a Go workspace with an additional Rust Harness service. Major
ownership boundaries are strict:

```
pkg/natskit -> shared Go NATS transport and subject contracts
services/* -> durable domain behavior exposed as NATS service functions
supervisor -> embedded NATS, catalogs, spaces, sessions, and discovery control
runtime -> agent loops and service-function tool execution
cli -> NATS client for supervisor/runtime operations
```

When adding code, keep domain behavior in its owning service and use shared
packages only for cross-boundary contracts or transport mechanics.

See `AGENTS.md` and `ARCHITECTURE.md` for current ownership rules.

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
