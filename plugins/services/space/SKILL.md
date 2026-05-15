# service-space

The space service owns Quark space metadata, the authoritative Quarkfile copy,
derived storage paths, launch environment resolution, and space diagnostics.

Use `quark.space.v1.SpaceService` for space lifecycle and metadata operations.
The supervisor keeps its HTTP API for CLI compatibility, but space business
logic lives behind this gRPC contract.

## RPCs

- `CreateSpace(CreateSpaceRequest) -> Space`
  - Generated service function: `space_CreateSpace`
  - Required: `name`, `quarkfile`, `working_dir`
  - Creates the supervised space layout and writes the initial Quarkfile.

- `UpdateQuarkfile(UpdateQuarkfileRequest) -> Space`
  - Generated service function: `space_UpdateQuarkfile`
  - Required: `name`, `quarkfile`
  - Replaces the latest Quarkfile and updates metadata.

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

- `GetQuarkfile(GetQuarkfileRequest) -> QuarkfileResponse`
  - Generated service function: `space_GetQuarkfile`
  - Required: `name`
  - Returns the authoritative Quarkfile bytes and version.

- `GetAgentEnvironment(GetAgentEnvironmentRequest) -> AgentEnvironmentResponse`
  - Generated service function: `space_GetAgentEnvironment`
  - Required: `name`
  - Resolves model/provider environment entries needed to launch a runtime.

- `GetSpacePaths(GetSpacePathsRequest) -> SpacePaths`
  - Generated service function: `space_GetSpacePaths`
  - Required: `name`
  - Returns derived storage paths for KB, plugins, sessions, and Quarkfile.

- `Doctor(DoctorRequest) -> DoctorResponse`
  - Generated service function: `space_Doctor`
  - Required: `name`
  - Runs Quarkfile validation and installed-plugin checks.

## Contract Notes

- Spaces are keyed by Quarkfile `meta.name`, not by path.
- The CLI should continue to use supervisor HTTP APIs; the supervisor delegates
  space work to this service.
- Runtime and supervisor callers should use gRPC for space metadata and
  environment operations.
