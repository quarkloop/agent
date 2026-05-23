# Space Service

`services/space` owns Quark space metadata, authoritative Quarkfile bytes,
derived paths, launch environment resolution, and space diagnostics.
Supervisor is the primary caller; runtime receives resolved catalogs and should
not discover local space state directly.

## Service Functions

| Function | RPC method | Request | Response | Purpose |
| --- | --- | --- | --- | --- |
| `space_CreateSpace` | `quark.space.v1.SpaceService/CreateSpace` | `CreateSpaceRequest` | `Space` | Create a space layout and persist the initial Quarkfile. |
| `space_UpdateQuarkfile` | `quark.space.v1.SpaceService/UpdateQuarkfile` | `UpdateQuarkfileRequest` | `Space` | Replace the authoritative Quarkfile for a space. |
| `space_GetSpace` | `quark.space.v1.SpaceService/GetSpace` | `GetSpaceRequest` | `Space` | Return persisted space metadata. |
| `space_ListSpaces` | `quark.space.v1.SpaceService/ListSpaces` | `Empty` | `ListSpacesResponse` | List registered spaces. |
| `space_DeleteSpace` | `quark.space.v1.SpaceService/DeleteSpace` | `DeleteSpaceRequest` | `Empty` | Delete service-owned space data. |
| `space_GetQuarkfile` | `quark.space.v1.SpaceService/GetQuarkfile` | `GetQuarkfileRequest` | `QuarkfileResponse` | Return authoritative Quarkfile bytes. |
| `space_GetAgentEnvironment` | `quark.space.v1.SpaceService/GetAgentEnvironment` | `GetAgentEnvironmentRequest` | `AgentEnvironmentResponse` | Resolve model launch environment entries from injected startup environment. |
| `space_GetSpacePaths` | `quark.space.v1.SpaceService/GetSpacePaths` | `GetSpacePathsRequest` | `SpacePaths` | Return derived storage paths for a space. |
| `space_Doctor` | `quark.space.v1.SpaceService/Doctor` | `DoctorRequest` | `DoctorResponse` | Validate Quarkfile and installed plugin state. |

## Ownership Boundaries

- The service owns persisted space files under the configured space root.
- The CLI remains HTTP-only through supervisor and does not call this service
  directly.
- Runtime receives resolved launch/config data from supervisor and does not read
  Quarkfiles or space directories.
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
- Space remains supervisor/control-plane owned. Core owns operational artifacts,
  audit, approvals, policy, and workspace mutation plans.
