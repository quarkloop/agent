# Space Service

`services/space` owns authoritative `space.json` bytes and opaque per-space
record persistence. Supervisor owns the meaning of session/plugin selection
state and calls this service for storage; runtime receives resolved catalogs.

## Service Functions

| Function | RPC method | Request | Response | Purpose |
| --- | --- | --- | --- | --- |
| `space_CreateSpace` | `quark.space.v1.SpaceService/CreateSpace` | `CreateSpaceRequest` | `Space` | Create a layout and persist the initial `space.json`. |
| `space_UpdateConfig` | `quark.space.v1.SpaceService/UpdateConfig` | `UpdateConfigRequest` | `Space` | Replace authoritative `space.json`. |
| `space_GetSpace` | `quark.space.v1.SpaceService/GetSpace` | `GetSpaceRequest` | `Space` | Return persisted space metadata. |
| `space_ListSpaces` | `quark.space.v1.SpaceService/ListSpaces` | `Empty` | `ListSpacesResponse` | List registered spaces. |
| `space_DeleteSpace` | `quark.space.v1.SpaceService/DeleteSpace` | `DeleteSpaceRequest` | `Empty` | Delete service-owned space data. |
| `space_GetConfig` | `quark.space.v1.SpaceService/GetConfig` | `GetConfigRequest` | `ConfigResponse` | Return authoritative configuration bytes. |
| `space_PutRecord` | `quark.space.v1.SpaceService/PutRecord` | `PutRecordRequest` | `Record` | Persist an opaque record in a caller-owned namespace. |
| `space_GetRecord` | `quark.space.v1.SpaceService/GetRecord` | `GetRecordRequest` | `Record` | Read one opaque record. |
| `space_ListRecords` | `quark.space.v1.SpaceService/ListRecords` | `ListRecordsRequest` | `ListRecordsResponse` | List opaque records within a namespace. |
| `space_DeleteRecord` | `quark.space.v1.SpaceService/DeleteRecord` | `DeleteRecordRequest` | `Empty` | Delete one opaque record. |
| `space_Doctor` | `quark.space.v1.SpaceService/Doctor` | `DoctorRequest` | `DoctorResponse` | Validate stored space configuration. |

## Ownership Boundaries

- The service owns persisted space files under the configured space root and
  never writes hidden product state into a user's working directory.
- The CLI sends high-level NATS control operations to supervisor; supervisor
  invokes this service for persisted space configuration operations.
- Runtime receives resolved launch/config data from supervisor and does not read
  space configuration files or space directories.
- Record bytes are uninterpreted by this service; semantic validation remains
  in the owning supervisor/runtime domain.

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

- Supervisor owns space semantics and catalog policy; this service owns only
  low-level configuration and record persistence.
