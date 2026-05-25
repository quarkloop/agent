# service-space

The space service owns the authoritative `space.json` configuration file,
derived storage paths, environment extraction from that configuration, and
configuration diagnostics. It does not interpret plugin policy or mutate user
working directories.

Use `quark.space.v1.SpaceService` service functions for space lifecycle and
metadata operations. Space business logic lives behind this NATS
service-function contract.

## RPCs

- `CreateSpace(CreateSpaceRequest) -> Space`
  - Generated service function: `space_CreateSpace`
  - Required: `config`
  - Creates the service-owned space layout and writes its initial `space.json`.

- `UpdateConfig(UpdateConfigRequest) -> Space`
  - Generated service function: `space_UpdateConfig`
  - Required: `config`
  - Replaces `space.json`, retaining service-owned creation metadata.

- `GetSpace(GetSpaceRequest) -> Space`
  - Generated service function: `space_GetSpace`
  - Required: `name`
  - Returns metadata for one space.

- `ListSpaces(Empty) -> ListSpacesResponse`
  - Generated service function: `space_ListSpaces`
  - Lists all registered spaces.

- `DeleteSpace(DeleteSpaceRequest) -> Empty`
  - Generated service function: `space_DeleteSpace`
  - Required: `name`
  - Deletes a space and all service-owned data.

- `GetConfig(GetConfigRequest) -> ConfigResponse`
  - Generated service function: `space_GetConfig`
  - Required: `name`
  - Returns the authoritative space configuration and version.

- `GetAgentEnvironment(GetAgentEnvironmentRequest) -> AgentEnvironmentResponse`
  - Generated service function: `space_GetAgentEnvironment`
  - Required: `name`
  - Resolves model/provider environment entries needed to launch a runtime.

- `GetSpacePaths(GetSpacePathsRequest) -> SpacePaths`
  - Generated service function: `space_GetSpacePaths`
  - Required: `name`
  - Returns derived storage paths for service-owned state and `space.json`.

- `Doctor(DoctorRequest) -> DoctorResponse`
  - Generated service function: `space_Doctor`
  - Required: `name`
  - Validates stored `space.json` syntax and space invariants.

## Contract Notes

- Spaces are keyed by `space.json` `name`, not by path.
- Plugin meaning, catalog resolution, and orchestration remain supervisor-owned.
- The CLI should use supervisor-owned NATS contracts for space operations.
- Runtime and supervisor callers should use service functions for space
  metadata and environment operations.
