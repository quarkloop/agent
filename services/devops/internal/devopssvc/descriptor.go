package devopssvc

import (
	devopsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/devops/v1"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
)

func Descriptor(address string, skill *servicev1.SkillDescriptor) *servicev1.ServiceDescriptor {
	return &servicev1.ServiceDescriptor{
		Name:    "devops",
		Type:    "devops",
		Version: "1.0.0",
		Address: address,
		Rpcs: []*servicev1.RpcDescriptor{
			rpc("repo_Status", devopsv1.RepoService_ServiceDesc.ServiceName, "Status", "quark.devops.v1.StatusRequest", "quark.devops.v1.StatusResponse", "Return repository branch, cleanliness, and changed-file status."),
			rpc("repo_Diff", devopsv1.RepoService_ServiceDesc.ServiceName, "Diff", "quark.devops.v1.DiffRequest", "quark.devops.v1.DiffResponse", "Return a bounded diff for selected repository files."),
			rpc("repo_GetBranch", devopsv1.RepoService_ServiceDesc.ServiceName, "GetBranch", "quark.devops.v1.GetBranchRequest", "quark.devops.v1.GetBranchResponse", "Return current and upstream branch information."),
			rpc("repo_ListChangedFiles", devopsv1.RepoService_ServiceDesc.ServiceName, "ListChangedFiles", "quark.devops.v1.ListChangedFilesRequest", "quark.devops.v1.ListChangedFilesResponse", "List changed repository files with staged and untracked state."),
			rpc("repo_ApplyPatch", devopsv1.RepoService_ServiceDesc.ServiceName, "ApplyPatch", "quark.devops.v1.ApplyPatchRequest", "quark.devops.v1.ApplyPatchResponse", "Prepare or validate an approval-gated repository patch application."),
			rpc("repo_Commit", devopsv1.RepoService_ServiceDesc.ServiceName, "Commit", "quark.devops.v1.CommitRequest", "quark.devops.v1.CommitResponse", "Prepare an approval-gated scoped commit plan."),
			rpc("repo_GenerateReleaseNotes", devopsv1.RepoService_ServiceDesc.ServiceName, "GenerateReleaseNotes", "quark.devops.v1.GenerateReleaseNotesRequest", "quark.devops.v1.GenerateReleaseNotesResponse", "Generate release-note markdown from repository history."),
			rpc("build_DetectProject", devopsv1.BuildService_ServiceDesc.ServiceName, "DetectProject", "quark.devops.v1.DetectProjectRequest", "quark.devops.v1.DetectProjectResponse", "Detect project kind, root, build files, and known tasks."),
			rpc("build_ResolveTask", devopsv1.BuildService_ServiceDesc.ServiceName, "ResolveTask", "quark.devops.v1.ResolveTaskRequest", "quark.devops.v1.ResolveTaskResponse", "Resolve one named build task into an executable plan."),
			rpc("build_RunTask", devopsv1.BuildService_ServiceDesc.ServiceName, "RunTask", "quark.devops.v1.RunTaskRequest", "quark.devops.v1.RunTaskResponse", "Run or plan one approved build task."),
			rpc("build_CreateArtifact", devopsv1.BuildService_ServiceDesc.ServiceName, "CreateArtifact", "quark.devops.v1.CreateArtifactRequest", "quark.devops.v1.CreateArtifactResponse", "Create or plan build artifacts for an approved task."),
			rpc("test_DiscoverTests", devopsv1.TestService_ServiceDesc.ServiceName, "DiscoverTests", "quark.devops.v1.DiscoverTestsRequest", "quark.devops.v1.DiscoverTestsResponse", "Discover test targets for a project."),
			rpc("test_RunTests", devopsv1.TestService_ServiceDesc.ServiceName, "RunTests", "quark.devops.v1.RunTestsRequest", "quark.devops.v1.RunTestsResponse", "Run selected test targets or produce a dry-run test plan."),
			rpc("test_ExplainFailure", devopsv1.TestService_ServiceDesc.ServiceName, "ExplainFailure", "quark.devops.v1.ExplainFailureRequest", "quark.devops.v1.ExplainFailureResponse", "Summarize structured test failure evidence."),
			rpc("container_BuildImage", devopsv1.ContainerService_ServiceDesc.ServiceName, "BuildImage", "quark.devops.v1.BuildImageRequest", "quark.devops.v1.BuildImageResponse", "Build or plan a container image from an approved Dockerfile."),
			rpc("container_ListImages", devopsv1.ContainerService_ServiceDesc.ServiceName, "ListImages", "quark.devops.v1.ListImagesRequest", "quark.devops.v1.ListImagesResponse", "List local container images matching an optional filter."),
			rpc("container_PlanRun", devopsv1.ContainerService_ServiceDesc.ServiceName, "PlanRun", "quark.devops.v1.PlanRunRequest", "quark.devops.v1.PlanRunResponse", "Prepare an approval-gated container run plan."),
			rpc("deploy_Plan", devopsv1.DeployService_ServiceDesc.ServiceName, "Plan", "quark.devops.v1.PlanRequest", "quark.devops.v1.PlanResponse", "Prepare a deployment change plan for review."),
			rpc("deploy_Apply", devopsv1.DeployService_ServiceDesc.ServiceName, "Apply", "quark.devops.v1.ApplyRequest", "quark.devops.v1.ApplyResponse", "Apply an approved deployment plan."),
			rpc("policy_EvaluateChange", devopsv1.PolicyService_ServiceDesc.ServiceName, "EvaluateChange", "quark.devops.v1.EvaluateChangeRequest", "quark.devops.v1.EvaluateChangeResponse", "Evaluate whether a proposed DevOps change is allowed and which approvals are required."),
		},
		Skills: []*servicev1.SkillDescriptor{skill},
	}
}

func rpc(functionName, service, method, request, response, description string) *servicev1.RpcDescriptor {
	return &servicev1.RpcDescriptor{
		Service:      service,
		Method:       method,
		Request:      request,
		Response:     response,
		Description:  description,
		FunctionName: functionName,
	}
}
