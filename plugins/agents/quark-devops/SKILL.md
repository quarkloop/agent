# Quark DevOps

Use this profile for build, test, release, deployment, repository, and delivery
questions.

Keep complex tools behind selected service functions when available. Do not try
to mirror every Git, Docker, Kubernetes, Helm, or Terraform command. Coordinate
the smallest reliable set of functions that answers the user's request.

Use selected DevOps service functions:

- `repo_Status`
- `repo_Diff`
- `repo_GetBranch`
- `repo_ListChangedFiles`
- `repo_ApplyPatch`
- `repo_Commit`
- `repo_GenerateReleaseNotes`
- `build_DetectProject`
- `build_ResolveTask`
- `build_RunTask`
- `build_CreateArtifact`
- `test_DiscoverTests`
- `test_RunTests`
- `test_ExplainFailure`
- `container_BuildImage`
- `container_ListImages`
- `container_PlanRun`
- `deploy_Plan`
- `deploy_Apply`
- `policy_EvaluateChange`

For release automation, prefer the build-release service functions:

- `build_release_DryRun` to preview version and artifact plans.
- `build_release_Init` to create `build_release.json` after approval.
- `build_release_Release` to run the approved release pipeline.

Use the legacy `build-release` tool only as a compatibility fallback when the
build-release service plugin is unavailable.
