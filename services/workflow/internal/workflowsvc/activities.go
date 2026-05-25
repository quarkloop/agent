package workflowsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/quarkloop/pkg/boundary"
	"github.com/quarkloop/pkg/natskit"
	workflowv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/workflow/v1"
)

type ServiceFunctionDispatcher interface {
	Dispatch(context.Context, natskit.Operation, natskit.RequestEnvelope) (natskit.ResponseEnvelope, error)
}

type NATSDispatcher struct {
	client  *natskit.Client
	timeout time.Duration
}

func NewNATSDispatcher(client *natskit.Client, timeout time.Duration) *NATSDispatcher {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &NATSDispatcher{client: client, timeout: timeout}
}

func (d *NATSDispatcher) Dispatch(ctx context.Context, operation natskit.Operation, req natskit.RequestEnvelope) (natskit.ResponseEnvelope, error) {
	if d == nil || d.client == nil {
		return natskit.ResponseEnvelope{}, fmt.Errorf("nats dispatcher is not configured")
	}
	callCtx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	resp, err := d.client.Call(callCtx, operation, req)
	if err != nil {
		return natskit.ResponseEnvelope{}, err
	}
	if resp.Status == natskit.StatusError {
		return resp, responseError(resp)
	}
	return resp, nil
}

type Activities struct {
	dispatcher ServiceFunctionDispatcher
	events     *EventLog
}

func NewActivities(dispatcher ServiceFunctionDispatcher, events *EventLog) *Activities {
	return &Activities{dispatcher: dispatcher, events: events}
}

func (a *Activities) DispatchServiceFunction(ctx context.Context, input ServiceFunctionActivityInput) (ServiceFunctionActivityResult, error) {
	if strings.TrimSpace(input.WorkflowID) == "" {
		return ServiceFunctionActivityResult{}, fmt.Errorf("workflow_id is required")
	}
	if strings.TrimSpace(input.SpaceID) == "" {
		return ServiceFunctionActivityResult{}, fmt.Errorf("space_id is required")
	}
	if strings.TrimSpace(input.Service) == "" || strings.TrimSpace(input.Function) == "" {
		return ServiceFunctionActivityResult{}, fmt.Errorf("service and function are required")
	}
	payload := json.RawMessage(strings.TrimSpace(input.PayloadJSON))
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	if !json.Valid(payload) {
		return ServiceFunctionActivityResult{}, fmt.Errorf("payload_json must be valid JSON")
	}
	operation, err := natskit.ServiceOperation(input.Service, input.Function)
	if err != nil {
		return ServiceFunctionActivityResult{}, err
	}
	req, err := natskit.NewRequest(
		fmt.Sprintf("%s.%s", input.WorkflowID, firstNonEmpty(input.StepID, input.Function)),
		input.SpaceID,
		natskit.ActorWorkflow,
		payload,
	)
	if err != nil {
		return ServiceFunctionActivityResult{}, err
	}
	req.WorkflowID = input.WorkflowID
	req.SessionID = input.SessionID
	req.AgentID = input.AgentID
	resp, err := a.dispatcher.Dispatch(ctx, operation, req)
	if err != nil {
		return ServiceFunctionActivityResult{}, err
	}
	return ServiceFunctionActivityResult{PayloadJSON: string(resp.Payload)}, nil
}

func (a *Activities) RecordWorkflowEvent(_ context.Context, event WorkflowEventRecord) error {
	if a == nil || a.events == nil {
		return nil
	}
	a.events.Append(&workflowv1.WorkflowEvent{
		WorkflowId:  event.WorkflowID,
		Type:        event.Type,
		StepId:      event.StepID,
		Message:     event.Message,
		PayloadJson: event.PayloadJSON,
	})
	return nil
}

func responseError(resp natskit.ResponseEnvelope) error {
	if resp.Error == nil {
		return fmt.Errorf("service function returned error")
	}
	return boundary.New(resp.Error.Boundary, resp.Error.Category, resp.Error.Operation, resp.Error.Message)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
