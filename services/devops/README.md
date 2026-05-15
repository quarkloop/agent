# DevOps Service

`services/devops` is the planned Quark DevOps service boundary. It owns selected
repository, build, test, container, deploy, and policy functions. It should wrap
complex ecosystems only through narrow service functions that match Quark
workflows.

Release automation remains in `services/build-release`.

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
| `test_DiscoverTests` | `quark.devops.v1.TestService/DiscoverTests` | `DiscoverTestsRequest` | `DiscoverTestsResponse` | Discover project test targets. |
| `test_RunTests` | `quark.devops.v1.TestService/RunTests` | `RunTestsRequest` | `RunTestsResponse` | Run selected tests or produce a dry-run test plan. |
| `test_ExplainFailure` | `quark.devops.v1.TestService/ExplainFailure` | `ExplainFailureRequest` | `ExplainFailureResponse` | Summarize structured test failure evidence. |
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
- Core/runtime owns approval state, policy gating, audit, and artifact
  persistence.
- Build-release service owns release business logic.

## Non-Goals

Do not clone every Git, Docker, Kubernetes, Helm, Terraform, or CI operation.
Add service functions only when they support a stable Quark workflow.
