package runstatesvc

import (
	"github.com/quarkloop/pkg/natskit"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
)

func Descriptor(address string, skill *servicev1.SkillDescriptor) *servicev1.ServiceDescriptor {
	var skills []*servicev1.SkillDescriptor
	if skill != nil {
		skills = append(skills, skill)
	}
	serviceName := "quark.runstate.v1.RunStateService"
	return &servicev1.ServiceDescriptor{
		Name: "runstate", Type: "runstate", Version: "1.0.0", Address: address,
		Rpcs: []*servicev1.RpcDescriptor{
			rpc(serviceName, "StartRun", "quark.runstate.v1.StartRunRequest", "quark.runstate.v1.StartRunResponse", "Create a durable execution run."),
			rpc(serviceName, "GetRun", "quark.runstate.v1.GetRunRequest", "quark.runstate.v1.GetRunResponse", "Return one execution run."),
			rpc(serviceName, "ListRuns", "quark.runstate.v1.ListRunsRequest", "quark.runstate.v1.ListRunsResponse", "List execution runs."),
			rpc(serviceName, "ResumeRun", "quark.runstate.v1.ResumeRunRequest", "quark.runstate.v1.ResumeRunResponse", "Make incomplete items resumable."),
			rpc(serviceName, "UpdateItemState", "quark.runstate.v1.UpdateItemStateRequest", "quark.runstate.v1.UpdateItemStateResponse", "Update an item phase and status."),
			rpc(serviceName, "AppendArtifact", "quark.runstate.v1.AppendArtifactRequest", "quark.runstate.v1.AppendArtifactResponse", "Attach an artifact reference."),
			rpc(serviceName, "AppendReference", "quark.runstate.v1.AppendReferenceRequest", "quark.runstate.v1.AppendReferenceResponse", "Attach an audit lookup reference_id returned by a service function."),
			rpc(serviceName, "MarkFailed", "quark.runstate.v1.MarkFailedRequest", "quark.runstate.v1.MarkFailedResponse", "Mark an execution run or item failed."),
			rpc(serviceName, "MarkComplete", "quark.runstate.v1.MarkCompleteRequest", "quark.runstate.v1.MarkCompleteResponse", "Mark an execution run or item complete."),
			rpc(serviceName, "CancelRun", "quark.runstate.v1.CancelRunRequest", "quark.runstate.v1.CancelRunResponse", "Cancel an execution run."),
			rpc(serviceName, "ListIncompleteItems", "quark.runstate.v1.ListIncompleteItemsRequest", "quark.runstate.v1.ListIncompleteItemsResponse", "List resumable run items."),
			rpc(serviceName, "ListArtifacts", "quark.runstate.v1.ListArtifactsRequest", "quark.runstate.v1.ListArtifactsResponse", "List attached artifact references."),
			rpc(serviceName, "AcquireLease", "quark.runstate.v1.AcquireLeaseRequest", "quark.runstate.v1.AcquireLeaseResponse", "Claim active coordination ownership through NATS KV."),
			rpc(serviceName, "RenewLease", "quark.runstate.v1.RenewLeaseRequest", "quark.runstate.v1.RenewLeaseResponse", "Renew owned active coordination lease."),
			rpc(serviceName, "ReleaseLease", "quark.runstate.v1.ReleaseLeaseRequest", "quark.runstate.v1.ReleaseLeaseResponse", "Release owned active coordination lease."),
		},
		Skills: skills,
	}
}

func rpc(service, method, request, response, description string) *servicev1.RpcDescriptor {
	return natskit.MustServiceRPC("runstate", "runstate_"+method, service, method, request, response, description)
}
