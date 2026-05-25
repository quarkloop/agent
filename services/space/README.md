# Space Service

`services/space` owns authoritative `space.json` bytes, derived paths,
environment extraction from space configuration, and configuration diagnostics.
Supervisor is the primary caller; runtime receives resolved catalogs and should
not discover local space state directly.

## Service Functions

| Function | RPC method | Request | Response | Purpose |
| --- | --- | --- | --- | --- |
| `space_CreateSpace` | `quark.space.v1.SpaceService/CreateSpace` | `CreateSpaceRequest` | `Space` | Create a layout and persist the initial `space.json`. |
| `space_UpdateConfig` | `quark.space.v1.SpaceService/UpdateConfig` | `UpdateConfigRequest` | `Space` | Replace authoritative `space.json`. |
| `space_GetSpace` | `quark.space.v1.SpaceService/GetSpace` | `GetSpaceRequest` | `Space` | Return persisted space metadata. |
| `space_ListSpaces` | `quark.space.v1.SpaceService/ListSpaces` | `Empty` | `ListSpacesResponse` | List registered spaces. |
| `space_DeleteSpace` | `quark.space.v1.SpaceService/DeleteSpace` | `DeleteSpaceRequest` | `Empty` | Delete service-owned space data. |
| `space_GetConfig` | `quark.space.v1.SpaceService/GetConfig` | `GetConfigRequest` | `ConfigResponse` | Return authoritative configuration bytes. |
| `space_GetAgentEnvironment` | `quark.space.v1.SpaceService/GetAgentEnvironment` | `GetAgentEnvironmentRequest` | `AgentEnvironmentResponse` | Resolve model launch environment entries from injected startup environment. |
| `space_GetSpacePaths` | `quark.space.v1.SpaceService/GetSpacePaths` | `GetSpacePathsRequest` | `SpacePaths` | Return derived storage paths for a space. |
| `space_Doctor` | `quark.space.v1.SpaceService/Doctor` | `DoctorRequest` | `DoctorResponse` | Validate stored space configuration. |

## Ownership Boundaries

- The service owns persisted space files under the configured space root and
  never writes hidden product state into a user's working directory.
- The CLI sends high-level NATS control operations to supervisor; supervisor
  invokes this service for persisted space configuration operations.
- Runtime receives resolved launch/config data from supervisor and does not read
  space configuration files or space directories.
- Environment values are captured at service startup and injected into the
  store; domain logic does not read process environment variables.

## Configuration

- `--nats-url`: NATS server URL used for service-function subjects.
- `--root`: space storage root. If unset, startup resolves
  `$QUARK_SPACES_ROOT` or `$HOME/.quarkloop/spaces`.
- `--skill-dir`: directory containing the service plugin `SKILL.md`.

## Health And Readiness

- Health protocol: NATS service-function readiness.
- Health service: `quark.space.v1.SpaceService`.
- Descriptor registry: `quark.service.v1.ServiceRegistry`.
- Readiness requires a writable space root and valid service descriptor.

## Audit Notes

- Environment lookup was moved out of domain flow into an injected startup
  snapshot during this audit.
- Supervisor owns space semantics and catalog policy; this service owns only
  low-level configuration persistence and derived storage paths.
