package server

import (
	"github.com/quarkloop/pkg/natskit"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
)

func Descriptor(address string, skill *servicev1.SkillDescriptor) *servicev1.ServiceDescriptor {
	var skills []*servicev1.SkillDescriptor
	if skill != nil {
		skills = append(skills, skill)
	}
	serviceName := "quark.core.v1.CoreService"
	return &servicev1.ServiceDescriptor{
		Name:    "core",
		Type:    "core",
		Version: "1.0.0",
		Address: address,
		Rpcs: []*servicev1.RpcDescriptor{
			rpc(serviceName, "CheckHealth", "quark.core.v1.CheckHealthRequest", "quark.core.v1.CheckHealthResponse", "Return Core service health diagnostics."),
			rpc(serviceName, "CheckReadiness", "quark.core.v1.CheckReadinessRequest", "quark.core.v1.CheckReadinessResponse", "Return Core service readiness diagnostics."),
			rpc(serviceName, "RecordAuditEvent", "quark.core.v1.RecordAuditEventRequest", "quark.core.v1.RecordAuditEventResponse", "Record a redacted audit event."),
			rpc(serviceName, "ListAuditEvents", "quark.core.v1.ListAuditEventsRequest", "quark.core.v1.ListAuditEventsResponse", "List redacted audit events."),
			rpc(serviceName, "PutArtifact", "quark.core.v1.PutArtifactRequest", "quark.core.v1.PutArtifactResponse", "Persist a redacted artifact reference."),
			rpc(serviceName, "GetArtifact", "quark.core.v1.GetArtifactRequest", "quark.core.v1.GetArtifactResponse", "Return a redacted artifact reference."),
			rpc(serviceName, "RequestApproval", "quark.core.v1.RequestApprovalRequest", "quark.core.v1.RequestApprovalResponse", "Create an approval request."),
			rpc(serviceName, "RecordApprovalDecision", "quark.core.v1.RecordApprovalDecisionRequest", "quark.core.v1.RecordApprovalDecisionResponse", "Record an approval decision."),
			rpc(serviceName, "GetConfig", "quark.core.v1.GetConfigRequest", "quark.core.v1.GetConfigResponse", "Read a scoped configuration value."),
			rpc(serviceName, "SetConfig", "quark.core.v1.SetConfigRequest", "quark.core.v1.SetConfigResponse", "Write a scoped configuration value."),
			rpc(serviceName, "GetSecretRef", "quark.core.v1.GetSecretRefRequest", "quark.core.v1.GetSecretRefResponse", "Return a secret reference without revealing the value."),
			rpc(serviceName, "PublishEvent", "quark.core.v1.PublishEventRequest", "quark.core.v1.PublishEventResponse", "Publish a redacted ordered event."),
			rpc(serviceName, "ListEvents", "quark.core.v1.ListEventsRequest", "quark.core.v1.ListEventsResponse", "List ordered redacted events."),
			rpc(serviceName, "EvaluatePolicy", "quark.core.v1.EvaluatePolicyRequest", "quark.core.v1.EvaluatePolicyResponse", "Evaluate policy for an action."),
			rpc(serviceName, "CreateWorkspaceMutationPlan", "quark.core.v1.CreateWorkspaceMutationPlanRequest", "quark.core.v1.CreateWorkspaceMutationPlanResponse", "Create an approval-gated workspace mutation plan."),
			rpc(serviceName, "ApproveWorkspaceMutationPlan", "quark.core.v1.ApproveWorkspaceMutationPlanRequest", "quark.core.v1.ApproveWorkspaceMutationPlanResponse", "Bind approval to a workspace mutation plan."),
			rpc(serviceName, "ScheduleRun", "quark.core.v1.ScheduleRunRequest", "quark.core.v1.ScheduleRunResponse", "Create or update a scheduled run."),
			rpc(serviceName, "ListSchedules", "quark.core.v1.ListSchedulesRequest", "quark.core.v1.ListSchedulesResponse", "List scheduled runs."),
			rpc(serviceName, "RecordEvaluation", "quark.core.v1.RecordEvaluationRequest", "quark.core.v1.RecordEvaluationResponse", "Record an evaluation result."),
			rpc(serviceName, "GetEvaluation", "quark.core.v1.GetEvaluationRequest", "quark.core.v1.GetEvaluationResponse", "Return an evaluation result."),
		},
		Skills: skills,
	}
}

func rpc(service, method, request, response, description string) *servicev1.RpcDescriptor {
	return natskit.MustServiceRPC("core", "core_"+method, service, method, request, response, description)
}
