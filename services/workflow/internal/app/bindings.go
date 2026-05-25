package app

import (
	"context"
	"fmt"

	"github.com/quarkloop/pkg/natskit"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	workflowv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/workflow/v1"
	"github.com/quarkloop/services/workflow/internal/workflowsvc"
	"google.golang.org/protobuf/proto"
)

func workflowBinding(address string, skill *servicev1.SkillDescriptor, server *workflowsvc.Server) natskit.Binding {
	return natskit.Binding{
		Descriptor: workflowsvc.Descriptor(address, skill),
		Services: []natskit.RPCService{{
			Service:        "quark.workflow.v1.WorkflowService",
			Implementation: server,
		}},
		Streams: []natskit.RPCStream{{
			Service: "quark.workflow.v1.WorkflowService",
			Method:  "StreamEvents",
			Handle:  workflowStreamEvents(server),
		}},
	}
}

func workflowStreamEvents(server *workflowsvc.Server) natskit.RPCStreamHandler {
	return func(ctx context.Context, input proto.Message, publish func(proto.Message) error) (proto.Message, error) {
		request, ok := input.(*workflowv1.StreamWorkflowEventsRequest)
		if !ok {
			return nil, fmt.Errorf("workflow StreamEvents request type %T is invalid", input)
		}
		events, err := server.EngineEvents(ctx, request)
		if err != nil {
			return nil, err
		}
		for event := range events {
			if err := publish(event); err != nil {
				return nil, err
			}
		}
		return &workflowv1.WorkflowEvent{}, nil
	}
}
