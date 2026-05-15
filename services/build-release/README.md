# Build Release Service

`services/build-release` owns Quark DevOps release automation behind typed gRPC
service functions. The legacy `plugins/tools/build-release` path should become a
thin compatibility adapter and must not own release business logic.

## Service Functions

| Function | RPC method | Request | Response | Purpose |
| --- | --- | --- | --- | --- |
| `build_release_Release` | `quark.buildrelease.v1.BuildReleaseService/Release` | `ReleaseRequest` | `ReleaseResponse` | Run the configured build/test/package/release pipeline and return artifacts. |
| `build_release_DryRun` | `quark.buildrelease.v1.BuildReleaseService/DryRun` | `DryRunRequest` | `DryRunResponse` | Preview release version and artifact matrix without compiling. |
| `build_release_Init` | `quark.buildrelease.v1.BuildReleaseService/Init` | `InitRequest` | `InitResponse` | Create a default `build_release.json` in a working directory. |

## Ownership Boundaries

- Quark DevOps coordinates release intent and approval.
- The build-release service owns config loading, version resolution, command
  execution, build artifacts, archives, checksums, signing, and metadata output.
- The service does not call other services.
- The gRPC server maps protobuf DTOs into package-level release requests before
  invoking the runner.

## Configuration

- `--addr`: gRPC listen address, default `127.0.0.1:7302`.
- `--skill-dir`: directory containing the service plugin `SKILL.md`.
- Release behavior is configured through `build_release.json` in the requested
  working directory.

## Health And Readiness

- Health protocol: gRPC health v1.
- Health service: `quark.buildrelease.v1.BuildReleaseService`.
- Descriptor registry: `quark.service.v1.ServiceRegistry`.
- Readiness requires the service binary and skill metadata. Per-project release
  config is validated by each function call.

## Audit Notes

- The service currently owns the production release runner.
- Task 13 will migrate the legacy tool plugin into a compatibility adapter,
  finalize function names, and add deeper function-level cancellation and
  artifact failure tests.
