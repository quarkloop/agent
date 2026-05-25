package workflowsvc

import (
	"github.com/quarkloop/pkg/natskit"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
)

func Descriptor(address string, skill *servicev1.SkillDescriptor) *servicev1.ServiceDescriptor {
	var skills []*servicev1.SkillDescriptor
	if skill != nil {
		skills = append(skills, skill)
	}
	serviceName := "quark.workflow.v1.WorkflowService"
	return &servicev1.ServiceDescriptor{
		Name:    "workflow",
		Type:    "workflow",
		Version: "1.0.0",
		Address: address,
		Rpcs: []*servicev1.RpcDescriptor{
			rpc(serviceName, "Start", "quark.workflow.v1.StartWorkflowRequest", "quark.workflow.v1.StartWorkflowResponse", "Start a durable workflow."),
			rpc(serviceName, "Signal", "quark.workflow.v1.SignalWorkflowRequest", "quark.workflow.v1.SignalWorkflowResponse", "Send a signal to a running workflow."),
			rpc(serviceName, "Query", "quark.workflow.v1.QueryWorkflowRequest", "quark.workflow.v1.QueryWorkflowResponse", "Query workflow state."),
			rpc(serviceName, "Cancel", "quark.workflow.v1.CancelWorkflowRequest", "quark.workflow.v1.CancelWorkflowResponse", "Cancel a workflow."),
			rpc(serviceName, "Describe", "quark.workflow.v1.DescribeWorkflowRequest", "quark.workflow.v1.DescribeWorkflowResponse", "Describe one workflow execution."),
			rpc(serviceName, "List", "quark.workflow.v1.ListWorkflowsRequest", "quark.workflow.v1.ListWorkflowsResponse", "List workflow executions."),
			streamingRPC(serviceName, "StreamEvents", "quark.workflow.v1.StreamWorkflowEventsRequest", "quark.workflow.v1.WorkflowEvent", "Stream workflow progress events."),
		},
		Skills: skills,
	}
}

func rpc(service, method, request, response, description string) *servicev1.RpcDescriptor {
	return natskit.MustServiceRPC("workflow", "workflow_"+method, service, method, request, response, description)
}

func streamingRPC(service, method, request, response, description string) *servicev1.RpcDescriptor {
	return natskit.MustStreamingServiceRPC("workflow", "workflow_"+method, service, method, request, response, description)
}
