package ingestionsvc

import (
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
)

func Descriptor(address string, skill *servicev1.SkillDescriptor) *servicev1.ServiceDescriptor {
	var skills []*servicev1.SkillDescriptor
	if skill != nil {
		skills = append(skills, skill)
	}
	serviceName := "quark.ingestion.v1.IngestionService"
	return &servicev1.ServiceDescriptor{
		Name:    "ingestion",
		Type:    "ingestion",
		Version: "1.0.0",
		Address: address,
		Rpcs: []*servicev1.RpcDescriptor{
			rpc(serviceName, "StartRun", "quark.ingestion.v1.StartRunRequest", "quark.ingestion.v1.StartRunResponse", "Create a durable ingestion run."),
			rpc(serviceName, "GetRun", "quark.ingestion.v1.GetRunRequest", "quark.ingestion.v1.GetRunResponse", "Return one ingestion run."),
			rpc(serviceName, "ListRuns", "quark.ingestion.v1.ListRunsRequest", "quark.ingestion.v1.ListRunsResponse", "List ingestion runs."),
			rpc(serviceName, "ResumeRun", "quark.ingestion.v1.ResumeRunRequest", "quark.ingestion.v1.ResumeRunResponse", "Mark incomplete sources resumable."),
			rpc(serviceName, "UpdateSourceState", "quark.ingestion.v1.UpdateSourceStateRequest", "quark.ingestion.v1.UpdateSourceStateResponse", "Update one source phase/status."),
			rpc(serviceName, "AppendArtifact", "quark.ingestion.v1.AppendArtifactRequest", "quark.ingestion.v1.AppendArtifactResponse", "Attach one artifact reference."),
			rpc(serviceName, "MarkFailed", "quark.ingestion.v1.MarkFailedRequest", "quark.ingestion.v1.MarkFailedResponse", "Mark a run or source failed."),
			rpc(serviceName, "MarkComplete", "quark.ingestion.v1.MarkCompleteRequest", "quark.ingestion.v1.MarkCompleteResponse", "Mark a run or source complete."),
			rpc(serviceName, "CancelRun", "quark.ingestion.v1.CancelRunRequest", "quark.ingestion.v1.CancelRunResponse", "Cancel an ingestion run."),
			rpc(serviceName, "ListIncompleteSources", "quark.ingestion.v1.ListIncompleteSourcesRequest", "quark.ingestion.v1.ListIncompleteSourcesResponse", "List resumable sources."),
			rpc(serviceName, "ListArtifacts", "quark.ingestion.v1.ListArtifactsRequest", "quark.ingestion.v1.ListArtifactsResponse", "List artifact references."),
		},
		Skills: skills,
	}
}

func rpc(service, method, request, response, description string) *servicev1.RpcDescriptor {
	return &servicev1.RpcDescriptor{
		Service:     service,
		Method:      method,
		Request:     request,
		Response:    response,
		Description: description,
	}
}
