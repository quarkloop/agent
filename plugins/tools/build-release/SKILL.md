# tool-build-release

Compatibility adapter for the Quark DevOps build-release service runner.
Prefer the service functions `build_release_DryRun`, `build_release_Init`, and
`build_release_Release` when the service plugin is available.

## Commands

- `build-release release [config] [--version V] [--parallel N] [--skip-tests] --json` — Full release pipeline
- `build-release dryrun [config] [--version V] --json` — Preview without compiling
- `build-release init --json` — Scaffold build_release.json

## Important

- This tool does not own release business logic; it delegates to
  `services/build-release/pkg/buildrelease`.
- Requires `go` in PATH
- `release` runs: config → version → validate → test → build → archive → checksum → sign → readme → metadata
- `dryrun` only runs: config → version → validate → build (dry)
- Config file defaults to `build_release.json` in working directory
