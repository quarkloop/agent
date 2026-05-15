# Space Service Plugin

The space service owns space metadata, Quarkfile bytes, derived paths, and
space diagnostics. Supervisor is the primary caller; runtime receives resolved
catalog data instead of discovering space state itself.

## Service Functions

| Function | RPC method | Risk | Approval | Idempotent | Purpose |
| --- | --- | --- | --- | --- | --- |
| `space_CreateSpace` | `quark.space.v1.SpaceService/CreateSpace` | write | yes | no | Create a space and persist its initial Quarkfile. |
| `space_UpdateQuarkfile` | `quark.space.v1.SpaceService/UpdateQuarkfile` | write | yes | no | Replace the latest Quarkfile for a space. |
| `space_GetSpace` | `quark.space.v1.SpaceService/GetSpace` | read | no | yes | Return persisted space metadata. |
| `space_ListSpaces` | `quark.space.v1.SpaceService/ListSpaces` | read | no | yes | List registered spaces. |
| `space_DeleteSpace` | `quark.space.v1.SpaceService/DeleteSpace` | admin | yes | no | Delete a space and its service-owned data. |
| `space_GetQuarkfile` | `quark.space.v1.SpaceService/GetQuarkfile` | read | no | yes | Return authoritative Quarkfile bytes. |
| `space_GetAgentEnvironment` | `quark.space.v1.SpaceService/GetAgentEnvironment` | admin | no | yes | Resolve model environment entries for runtime launch. |
| `space_GetSpacePaths` | `quark.space.v1.SpaceService/GetSpacePaths` | read | no | yes | Return derived storage paths for a space. |
| `space_Doctor` | `quark.space.v1.SpaceService/Doctor` | read | no | yes | Run Quarkfile and installed-plugin diagnostics. |

Space mutation functions should stay behind supervisor-owned approval and
validation paths.
