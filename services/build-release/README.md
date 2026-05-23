# Build Release Service

`services/build-release` owns Quark DevOps release automation behind typed service-function
service functions. The legacy `plugins/tools/build-release` path should become a
thin compatibility adapter and must not own release business logic.

## Service Functions

| Function | RPC method | Request | Response | Purpose |
| --- | --- | --- | --- | --- |
| `build_release_Release` | `quark.buildrelease.v1.BuildReleaseService/Release` | `ReleaseRequest` | `ReleaseResponse` | Run the configured build/test/package/release pipeline and return artifacts. |
| `build_release_DryRun` | `quark.buildrelease.v1.BuildReleaseService/DryRun` | `DryRunRequest` | `DryRunResponse` | Preview release version and artifact matrix without compiling. |
| `build_release_Init` | `quark.buildrelease.v1.BuildReleaseService/Init` | `InitRequest` | `InitResponse` | Create a default `build_release.json` in a working directory. |

These are the canonical launch function names. Separate build, test, and
package functions are intentionally deferred until Quark DevOps workflows need
smaller operations; today they remain internal runner stages.

## Ownership Boundaries

- Quark DevOps coordinates release intent and approval.
- The build-release service owns config loading, version resolution, command
  execution, build artifacts, archives, checksums, signing, and metadata output.
- The service does not call other services.
- The transport adapter maps protobuf DTOs into package-level release requests before
  invoking the runner.

## Configuration

- `--nats-url`: NATS server URL used for service-function subjects.
- `--skill-dir`: directory containing the service plugin `SKILL.md`.
- Release behavior is configured through `build_release.json` in the requested
  working directory.

## Examples

Dry run:

```json
{
  "workingDir": "/workspace/project",
  "configPath": "build_release.json",
  "version": "v1.2.3",
  "parallelism": 4
}
```

Release:

```json
{
  "workingDir": "/workspace/project",
  "configPath": "build_release.json",
  "version": "v1.2.3",
  "parallelism": 4,
  "skipTests": false
}
```

## Health And Readiness

- Health protocol: NATS service-function readiness.
- Health service: `quark.buildrelease.v1.BuildReleaseService`.
- Descriptor registry: `quark.service.v1.ServiceRegistry`.
- Readiness requires the service binary and skill metadata. Per-project release
  config is validated by each function call.

## Audit Notes

- The service currently owns the production release runner.
- The legacy tool plugin is compatibility-only. New DevOps release behavior
  should call `build_release_DryRun`, `build_release_Init`, and
  `build_release_Release` through runtime service-function dispatch.
