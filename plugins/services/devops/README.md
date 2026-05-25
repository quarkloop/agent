# DevOps Service Plugin

The DevOps service plugin defines selected repository, build, test, container,
deploy, and policy service functions for Quark DevOps. It does not mirror every
Git, Docker, Kubernetes, Helm, or Terraform command.

Release automation is part of this DevOps service. There is no separate
release automation production service or tool plugin path.

Runtime calls these functions through canonical NATS subjects owned by this
plugin: `svc.devops.v1.<function_name_in_snake_case>`, such as
`svc.devops.v1.repo_status` and `svc.devops.v1.test_run_tests`.

## Service Functions

| Function | RPC method | Risk | Approval | Idempotent | Purpose |
| --- | --- | --- | --- | --- | --- |
| `repo_Status` | `quark.devops.v1.RepoService/Status` | read | no | yes | Return branch, cleanliness, and changed-file status. |
| `repo_Diff` | `quark.devops.v1.RepoService/Diff` | read | no | yes | Return a bounded diff for selected files. |
| `repo_GetBranch` | `quark.devops.v1.RepoService/GetBranch` | read | no | yes | Return current and upstream branch information. |
| `repo_ListChangedFiles` | `quark.devops.v1.RepoService/ListChangedFiles` | read | no | yes | List changed files with staged/untracked state. |
| `repo_ApplyPatch` | `quark.devops.v1.RepoService/ApplyPatch` | write | yes | no | Prepare or validate an approval-gated patch application. |
| `repo_Commit` | `quark.devops.v1.RepoService/Commit` | write | yes | no | Prepare an approval-gated scoped commit plan. |
| `repo_GenerateReleaseNotes` | `quark.devops.v1.RepoService/GenerateReleaseNotes` | read | no | yes | Generate release-note markdown from history. |
| `build_DetectProject` | `quark.devops.v1.BuildService/DetectProject` | read | no | yes | Detect project kind, root, build files, and known tasks. |
| `build_ResolveTask` | `quark.devops.v1.BuildService/ResolveTask` | read | no | yes | Resolve one named build task. |
| `build_RunTask` | `quark.devops.v1.BuildService/RunTask` | write | yes | no | Run or plan one approved build task. |
| `build_CreateArtifact` | `quark.devops.v1.BuildService/CreateArtifact` | write | yes | no | Create or plan artifacts for an approved task. |
| `build_InitReleaseConfig` | `quark.devops.v1.BuildService/InitReleaseConfig` | write | yes | no | Create a default release configuration in an approved workspace. |
| `build_DryRunRelease` | `quark.devops.v1.BuildService/DryRunRelease` | read | no | yes | Preview release version and artifact matrix without compiling or publishing. |
| `build_RunRelease` | `quark.devops.v1.BuildService/RunRelease` | admin | yes | no | Run an approved release pipeline and return generated artifacts. |
| `test_DiscoverTests` | `quark.devops.v1.TestService/DiscoverTests` | read | no | yes | Discover project test targets. |
| `test_RunTests` | `quark.devops.v1.TestService/RunTests` | write | yes | no | Run selected tests or produce a dry-run test plan. |
| `test_ExplainFailure` | `quark.devops.v1.TestService/ExplainFailure` | read | no | yes | Summarize structured test failure evidence. |
| `container_BuildImage` | `quark.devops.v1.ContainerService/BuildImage` | write | yes | no | Build or plan a container image. |
| `container_ListImages` | `quark.devops.v1.ContainerService/ListImages` | read | no | yes | List local container images. |
| `container_PlanRun` | `quark.devops.v1.ContainerService/PlanRun` | write | yes | no | Prepare an approval-gated container run plan. |
| `deploy_Plan` | `quark.devops.v1.DeployService/Plan` | write | yes | no | Prepare a deployment change plan. |
| `deploy_Apply` | `quark.devops.v1.DeployService/Apply` | admin | yes | no | Apply an approved deployment plan. |
| `policy_EvaluateChange` | `quark.devops.v1.PolicyService/EvaluateChange` | read | no | yes | Evaluate policy and required approvals for a proposed change. |

## Adapter Backends

Implementations may wrap `git`, language build tools, Docker, Kubernetes,
Helm, Terraform, or CI providers, but only through these selected service
functions. Agents should not receive broad command wrappers as a substitute for
typed DevOps behavior.

## Approval

Workspace writes, commits, command execution, artifact creation, container
build/run, and deployment actions require approval. Read-only status, diff,
discovery, and policy evaluation functions do not.

## Non-Goals

- No full clone of Git, Docker, Kubernetes, Helm, Terraform, or CI APIs.
- No hidden shell command execution outside service function contracts.
- No service-to-service calls.
- No broad shell adapter for release work; use the typed release functions
  listed above.

## Health And Readiness

- Health protocol: NATS service-function readiness.
- Health service: `quark.devops.v1.RepoService`.
- Required readiness: yes, before runtime receives the service catalog.
- Minimum descriptor version: `1.0.0`.
