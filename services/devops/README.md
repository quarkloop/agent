# DevOps Service

`services/devops` is the Quark DevOps service boundary. It owns selected
repository, build, test, container, deploy, and policy functions. It wraps
complex ecosystems only through narrow service functions that match Quark
workflows.

Release automation is owned by this service through typed build service
functions. The legacy standalone release service/tool path has been removed.

All functions are registered through `pkg/natskit` on canonical NATS
request/reply subjects. The subject for a function is
`svc.devops.v1.<function_name_in_snake_case>`, for example
`svc.devops.v1.repo_status` and `svc.devops.v1.build_run_release`.

## Service Functions

| Function | RPC method | Request | Response | Purpose |
| --- | --- | --- | --- | --- |
| `repo_Status` | `quark.devops.v1.RepoService/Status` | `StatusRequest` | `StatusResponse` | Return branch, cleanliness, and changed-file status. |
| `repo_Diff` | `quark.devops.v1.RepoService/Diff` | `DiffRequest` | `DiffResponse` | Return a bounded diff for selected files. |
| `repo_GetBranch` | `quark.devops.v1.RepoService/GetBranch` | `GetBranchRequest` | `GetBranchResponse` | Return current and upstream branch information. |
| `repo_ListChangedFiles` | `quark.devops.v1.RepoService/ListChangedFiles` | `ListChangedFilesRequest` | `ListChangedFilesResponse` | List changed files with staged/untracked state. |
| `repo_ApplyPatch` | `quark.devops.v1.RepoService/ApplyPatch` | `ApplyPatchRequest` | `ApplyPatchResponse` | Prepare or validate an approval-gated patch application. |
| `repo_Commit` | `quark.devops.v1.RepoService/Commit` | `CommitRequest` | `CommitResponse` | Prepare an approval-gated scoped commit plan. |
| `repo_GenerateReleaseNotes` | `quark.devops.v1.RepoService/GenerateReleaseNotes` | `GenerateReleaseNotesRequest` | `GenerateReleaseNotesResponse` | Generate release-note markdown from history. |
| `build_DetectProject` | `quark.devops.v1.BuildService/DetectProject` | `DetectProjectRequest` | `DetectProjectResponse` | Detect project kind, root, build files, and known tasks. |
| `build_ResolveTask` | `quark.devops.v1.BuildService/ResolveTask` | `ResolveTaskRequest` | `ResolveTaskResponse` | Resolve one named build task. |
| `build_RunTask` | `quark.devops.v1.BuildService/RunTask` | `RunTaskRequest` | `RunTaskResponse` | Run or plan one approved build task. |
| `build_CreateArtifact` | `quark.devops.v1.BuildService/CreateArtifact` | `CreateArtifactRequest` | `CreateArtifactResponse` | Create or plan artifacts for an approved task. |
| `build_InitReleaseConfig` | `quark.devops.v1.BuildService/InitReleaseConfig` | `InitReleaseConfigRequest` | `InitReleaseConfigResponse` | Create a default release configuration in an approved workspace. |
| `build_DryRunRelease` | `quark.devops.v1.BuildService/DryRunRelease` | `DryRunReleaseRequest` | `DryRunReleaseResponse` | Preview release version and artifact matrix without compiling or publishing. |
| `build_RunRelease` | `quark.devops.v1.BuildService/RunRelease` | `RunReleaseRequest` | `RunReleaseResponse` | Run an approved release pipeline and return generated artifacts. |
| `test_DiscoverTests` | `quark.devops.v1.TestService/DiscoverTests` | `DiscoverTestsRequest` | `DiscoverTestsResponse` | Discover project test targets. |
| `test_RunTests` | `quark.devops.v1.TestService/RunTests` | `RunTestsRequest` | `RunTestsResponse` | Run discovered target IDs, or the default target, and return bounded failure evidence. |
| `test_ExplainFailure` | `quark.devops.v1.TestService/ExplainFailure` | `ExplainFailureRequest` | `ExplainFailureResponse` | Summarize bounded structured test failure evidence. |
| `container_BuildImage` | `quark.devops.v1.ContainerService/BuildImage` | `BuildImageRequest` | `BuildImageResponse` | Build or plan a container image. |
| `container_ListImages` | `quark.devops.v1.ContainerService/ListImages` | `ListImagesRequest` | `ListImagesResponse` | List local container images. |
| `container_PlanRun` | `quark.devops.v1.ContainerService/PlanRun` | `PlanRunRequest` | `PlanRunResponse` | Prepare an approval-gated container run plan. |
| `deploy_Plan` | `quark.devops.v1.DeployService/Plan` | `PlanRequest` | `PlanResponse` | Prepare a deployment change plan. |
| `deploy_Apply` | `quark.devops.v1.DeployService/Apply` | `ApplyRequest` | `ApplyResponse` | Apply an approved deployment plan. |
| `policy_EvaluateChange` | `quark.devops.v1.PolicyService/EvaluateChange` | `EvaluateChangeRequest` | `EvaluateChangeResponse` | Evaluate policy and required approvals for a proposed change. |

## Ownership Boundaries

- Quark DevOps agent coordinates user intent and selects service functions.
- DevOps service wraps selected adapters and returns structured results or
  mutation plans.
- `internal/devopssvc` keeps repository, build, release, test, container,
  deployment, policy, workspace, command, and diagnostic adapters separate.
- Ordinary handlers delegate executable invocation to the command adapter;
  the release pipeline owns only its bounded build, test, compression, and
  signing invocations.
- Core/runtime owns approval state, policy gating, audit, and artifact
  persistence.
- Release automation belongs to DevOps build service functions.

## Configuration

- `--nats-url`: NATS server URL used for service-function subjects.
- `--skill-dir`: directory containing the service plugin `SKILL.md`.

## Deployment Runtime

The Docker Compose DevOps service uses `deploy/docker/Dockerfile.devops`
rather than the minimal generic service image. Its execution image includes
the Go toolchain and Docker client required by the exported typed build, test,
release, and container service functions.

## Health And Readiness

- Health protocol: NATS service-function readiness.
- Primary health service: `quark.devops.v1.RepoService`.
- Descriptor source: service plugin manifest and NATS service metadata.

## Non-Goals

Do not clone every Git, Docker, Kubernetes, Helm, Terraform, or CI operation.
Add service functions only when they support a stable Quark workflow.
