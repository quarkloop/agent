package workflowsvc

import (
	"context"

	workflowv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/workflow/v1"
)

type Activities struct {
	events *EventLog
}

func NewActivities(events *EventLog) *Activities {
	return &Activities{events: events}
}

func (a *Activities) RecordWorkflowEvent(_ context.Context, event WorkflowEventRecord) error {
	if a == nil || a.events == nil {
		return nil
	}
	a.events.Append(&workflowv1.WorkflowEvent{
		WorkflowId:   event.WorkflowID,
		Type:         event.Type,
		CheckpointId: event.CheckpointID,
		Message:      event.Message,
		PayloadJson:  event.PayloadJSON,
	})
	return nil
}
