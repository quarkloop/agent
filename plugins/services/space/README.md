# Space Service Plugin

The space service owns authoritative `space.json` bytes, derived paths, and
configuration diagnostics. Supervisor is the primary caller; runtime receives
resolved catalog data instead of discovering space state itself.

## Service Functions

| Function | RPC method | Risk | Approval | Idempotent | Purpose |
| --- | --- | --- | --- | --- | --- |
| `space_CreateSpace` | `quark.space.v1.SpaceService/CreateSpace` | write | yes | no | Create a space and persist its initial `space.json`. |
| `space_UpdateConfig` | `quark.space.v1.SpaceService/UpdateConfig` | write | yes | no | Replace authoritative `space.json`. |
| `space_GetSpace` | `quark.space.v1.SpaceService/GetSpace` | read | no | yes | Return persisted space metadata. |
| `space_ListSpaces` | `quark.space.v1.SpaceService/ListSpaces` | read | no | yes | List registered spaces. |
| `space_DeleteSpace` | `quark.space.v1.SpaceService/DeleteSpace` | admin | yes | no | Delete a space and its service-owned data. |
| `space_GetConfig` | `quark.space.v1.SpaceService/GetConfig` | read | no | yes | Return authoritative configuration bytes. |
| `space_GetAgentEnvironment` | `quark.space.v1.SpaceService/GetAgentEnvironment` | admin | no | yes | Resolve model environment entries for runtime launch. |
| `space_GetSpacePaths` | `quark.space.v1.SpaceService/GetSpacePaths` | read | no | yes | Return derived storage paths for a space. |
| `space_Doctor` | `quark.space.v1.SpaceService/Doctor` | read | no | yes | Validate stored space configuration. |

Space mutation functions should stay behind supervisor-owned approval and
validation paths.

## Health And Readiness

- Health protocol: NATS service-function readiness.
- Health service: `quark.space.v1.SpaceService`.
- Required readiness: yes, before runtime receives the service catalog.
- Minimum descriptor version: `1.0.0`.
- Startup diagnostics cover missing NATS endpoints, failed service-function
  readiness, descriptor version mismatch, and missing RPC descriptors.
