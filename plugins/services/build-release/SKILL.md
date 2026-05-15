# service-build-release

The build-release service runs Quark's Go release pipeline over gRPC. It is
independently deployable and does not depend on the agent plugin runtime.

Use `quark.buildrelease.v1.BuildReleaseService` when release automation should
run as a service instead of as a local tool plugin.

## RPCs

- `Release(ReleaseRequest) -> ReleaseResponse`
  - Generated service function: `build_release_Release`
  - Required: `working_dir`
  - Optional: `config_path`, `version`, `parallelism`, `skip_tests`
  - Runs config loading, version resolution, optional tests, cross-compilation,
    archive generation, checksums, signing, README, and metadata output.
  - Requires approval because it can execute commands and write release
    artifacts.

- `DryRun(DryRunRequest) -> DryRunResponse`
  - Generated service function: `build_release_DryRun`
  - Required: `working_dir`
  - Optional: `config_path`, `version`, `parallelism`
  - Returns the artifact matrix without compiling or writing release files.
  - Does not require approval.

- `Init(InitRequest) -> InitResponse`
  - Generated service function: `build_release_Init`
  - Required: `working_dir`
  - Optional: `overwrite`
  - Creates `build_release.json` when it does not already exist.
  - Requires approval because it writes to the workspace.

## Contract Notes

- Paths are resolved relative to `working_dir` unless already absolute.
- Cancellation is honored for external commands such as `go test`, `go build`,
  and `gpg`.
- The service owns the release pipeline; the legacy plugin is a compatibility
  adapter and should not contain release business logic.
- Keep build/test/package internals inside this service for now. Future DevOps
  service functions can expose narrower build, test, or package operations when
  Quark DevOps workflows need them.
