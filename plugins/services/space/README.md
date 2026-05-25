# Space Service Plugin

The space service owns authoritative `space.json` bytes and opaque record
persistence. Supervisor remains the semantic owner of sessions and plugin
selection; runtime consumes resolved catalog data.

## Service Functions

| Function | RPC method | Risk | Approval | Idempotent | Purpose |
| --- | --- | --- | --- | --- | --- |
| `space_CreateSpace` | `quark.space.v1.SpaceService/CreateSpace` | write | yes | no | Create a space and persist its initial `space.json`. |
| `space_UpdateConfig` | `quark.space.v1.SpaceService/UpdateConfig` | write | yes | no | Replace authoritative `space.json`. |
| `space_GetSpace` | `quark.space.v1.SpaceService/GetSpace` | read | no | yes | Return persisted space metadata. |
| `space_ListSpaces` | `quark.space.v1.SpaceService/ListSpaces` | read | no | yes | List registered spaces. |
| `space_DeleteSpace` | `quark.space.v1.SpaceService/DeleteSpace` | admin | yes | no | Delete a space and its service-owned data. |
| `space_GetConfig` | `quark.space.v1.SpaceService/GetConfig` | read | no | yes | Return authoritative configuration bytes. |
| `space_PutRecord` | `quark.space.v1.SpaceService/PutRecord` | write | yes | no | Persist an opaque record in a caller-owned namespace. |
| `space_GetRecord` | `quark.space.v1.SpaceService/GetRecord` | read | no | yes | Read an opaque record. |
| `space_ListRecords` | `quark.space.v1.SpaceService/ListRecords` | read | no | yes | List records in a namespace. |
| `space_DeleteRecord` | `quark.space.v1.SpaceService/DeleteRecord` | write | yes | no | Delete an opaque record. |
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
