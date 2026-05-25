# Release Readiness

Quark releases should be cut only after the local readiness gate passes and
provider-backed E2E has been reviewed.

## Required Local Gate

```bash
make release-check
```

The gate verifies:

- Go 1.26 is active,
- formatting, vet, unit tests, architecture boundaries, and dead-code checks,
- generated protobuf/service API output is current,
- CLI, supervisor, runtime, tool plugins, and services build,
- the configured local E2E scenarios pass against their real service/model boundaries.

## Provider-Backed Gate

Before release candidates, run provider-backed E2E when credentials and quota
are available:

```bash
export OPENROUTER_API_KEY=sk-or-v1-...
export OPENROUTER_E2E_EMBEDDING_MODEL=nvidia/llama-nemotron-embed-vl-1b-v2:free
make test-e2e
```

Skips are acceptable only for explicit provider auth or quota conditions. Any
functional failure in supervisor, runtime, tool, service, catalog, or indexing
flow blocks the release.

## Manual Review

- Check README, ARCHITECTURE, DEVELOPMENT, AGENTS, and service plugin README
  files for stale commands or removed workflows.
- Confirm service plugin manifests list the exported service functions.
- Confirm E2E artifacts redact secrets and include tool/service timelines.
- Confirm no changes under `docs/` are accidentally staged when those files are
  only local task tracking or drafts.
