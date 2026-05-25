package devopssvc

import (
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
)

func Descriptor(address string, skill *servicev1.SkillDescriptor) *servicev1.ServiceDescriptor {
	return &servicev1.ServiceDescriptor{
		Name:    "devops",
		Type:    "devops",
		Version: "1.0.0",
		Address: address,
		Rpcs: []*servicev1.RpcDescriptor{
			rpc("repo_Status", "repo_status", "quark.devops.v1.RepoService", "Status", "quark.devops.v1.StatusRequest", "quark.devops.v1.StatusResponse", "Return repository branch, cleanliness, and changed-file status."),
			rpc("repo_Diff", "repo_diff", "quark.devops.v1.RepoService", "Diff", "quark.devops.v1.DiffRequest", "quark.devops.v1.DiffResponse", "Return a bounded diff for selected repository files."),
			rpc("repo_GetBranch", "repo_get_branch", "quark.devops.v1.RepoService", "GetBranch", "quark.devops.v1.GetBranchRequest", "quark.devops.v1.GetBranchResponse", "Return current and upstream branch information."),
			rpc("repo_ListChangedFiles", "repo_list_changed_files", "quark.devops.v1.RepoService", "ListChangedFiles", "quark.devops.v1.ListChangedFilesRequest", "quark.devops.v1.ListChangedFilesResponse", "List changed repository files with staged and untracked state."),
			rpc("repo_ApplyPatch", "repo_apply_patch", "quark.devops.v1.RepoService", "ApplyPatch", "quark.devops.v1.ApplyPatchRequest", "quark.devops.v1.ApplyPatchResponse", "Prepare or validate an approval-gated repository patch application."),
			rpc("repo_Commit", "repo_commit", "quark.devops.v1.RepoService", "Commit", "quark.devops.v1.CommitRequest", "quark.devops.v1.CommitResponse", "Prepare an approval-gated scoped commit plan."),
			rpc("repo_GenerateReleaseNotes", "repo_generate_release_notes", "quark.devops.v1.RepoService", "GenerateReleaseNotes", "quark.devops.v1.GenerateReleaseNotesRequest", "quark.devops.v1.GenerateReleaseNotesResponse", "Generate release-note markdown from repository history."),
			rpc("build_DetectProject", "build_detect_project", "quark.devops.v1.BuildService", "DetectProject", "quark.devops.v1.DetectProjectRequest", "quark.devops.v1.DetectProjectResponse", "Detect project kind, root, build files, and known tasks."),
			rpc("build_ResolveTask", "build_resolve_task", "quark.devops.v1.BuildService", "ResolveTask", "quark.devops.v1.ResolveTaskRequest", "quark.devops.v1.ResolveTaskResponse", "Resolve one named build task into an executable plan."),
			rpc("build_RunTask", "build_run_task", "quark.devops.v1.BuildService", "RunTask", "quark.devops.v1.RunTaskRequest", "quark.devops.v1.RunTaskResponse", "Run or plan one approved build task."),
			rpc("build_CreateArtifact", "build_create_artifact", "quark.devops.v1.BuildService", "CreateArtifact", "quark.devops.v1.CreateArtifactRequest", "quark.devops.v1.CreateArtifactResponse", "Create or plan build artifacts for an approved task."),
			rpc("build_InitReleaseConfig", "build_init_release_config", "quark.devops.v1.BuildService", "InitReleaseConfig", "quark.devops.v1.InitReleaseConfigRequest", "quark.devops.v1.InitReleaseConfigResponse", "Create a default release configuration in an approved workspace."),
			rpc("build_DryRunRelease", "build_dry_run_release", "quark.devops.v1.BuildService", "DryRunRelease", "quark.devops.v1.DryRunReleaseRequest", "quark.devops.v1.DryRunReleaseResponse", "Preview release version and artifact matrix without compiling or publishing."),
			rpc("build_RunRelease", "build_run_release", "quark.devops.v1.BuildService", "RunRelease", "quark.devops.v1.RunReleaseRequest", "quark.devops.v1.RunReleaseResponse", "Run an approved release pipeline and return generated artifacts."),
			rpc("test_DiscoverTests", "test_discover_tests", "quark.devops.v1.TestService", "DiscoverTests", "quark.devops.v1.DiscoverTestsRequest", "quark.devops.v1.DiscoverTestsResponse", "Discover test targets for a project."),
			rpc("test_RunTests", "test_run_tests", "quark.devops.v1.TestService", "RunTests", "quark.devops.v1.RunTestsRequest", "quark.devops.v1.RunTestsResponse", "Run selected test targets or produce a dry-run test plan."),
			rpc("test_ExplainFailure", "test_explain_failure", "quark.devops.v1.TestService", "ExplainFailure", "quark.devops.v1.ExplainFailureRequest", "quark.devops.v1.ExplainFailureResponse", "Summarize structured test failure evidence."),
			rpc("container_BuildImage", "container_build_image", "quark.devops.v1.ContainerService", "BuildImage", "quark.devops.v1.BuildImageRequest", "quark.devops.v1.BuildImageResponse", "Build or plan a container image from an approved Dockerfile."),
			rpc("container_ListImages", "container_list_images", "quark.devops.v1.ContainerService", "ListImages", "quark.devops.v1.ListImagesRequest", "quark.devops.v1.ListImagesResponse", "List local container images matching an optional filter."),
			rpc("container_PlanRun", "container_plan_run", "quark.devops.v1.ContainerService", "PlanRun", "quark.devops.v1.PlanRunRequest", "quark.devops.v1.PlanRunResponse", "Prepare an approval-gated container run plan."),
			rpc("deploy_Plan", "deploy_plan", "quark.devops.v1.DeployService", "Plan", "quark.devops.v1.PlanRequest", "quark.devops.v1.PlanResponse", "Prepare a deployment change plan for review."),
			rpc("deploy_Apply", "deploy_apply", "quark.devops.v1.DeployService", "Apply", "quark.devops.v1.ApplyRequest", "quark.devops.v1.ApplyResponse", "Apply an approved deployment plan."),
			rpc("policy_EvaluateChange", "policy_evaluate_change", "quark.devops.v1.PolicyService", "EvaluateChange", "quark.devops.v1.EvaluateChangeRequest", "quark.devops.v1.EvaluateChangeResponse", "Evaluate whether a proposed DevOps change is allowed and which approvals are required."),
		},
		Skills: []*servicev1.SkillDescriptor{skill},
	}
}

func rpc(functionName, function, service, method, request, response, description string) *servicev1.RpcDescriptor {
	return &servicev1.RpcDescriptor{
		Service:      service,
		Method:       method,
		Request:      request,
		Response:     response,
		Description:  description,
		FunctionName: functionName,
		Owner:        "devops",
		Subject:      "svc.devops.v1." + function,
	}
}
