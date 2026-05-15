# Build Release Service Plugin

The build-release service belongs to Quark DevOps. It runs release automation
behind typed service functions. The legacy build-release tool is only a
compatibility adapter and should not own release business logic.

## Service Functions

| Function | RPC method | Risk | Approval | Idempotent | Purpose |
| --- | --- | --- | --- | --- | --- |
| `build_release_Release` | `quark.buildrelease.v1.BuildReleaseService/Release` | admin | yes | no | Run the configured build and release pipeline, including command execution and artifact writes. |
| `build_release_DryRun` | `quark.buildrelease.v1.BuildReleaseService/DryRun` | read | no | yes | Preview planned release artifacts without compiling. |
| `build_release_Init` | `quark.buildrelease.v1.BuildReleaseService/Init` | write | yes | no | Create a default `build_release.json` in a working directory. |

Approval is required for functions that write files or execute release builds.
