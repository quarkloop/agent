package workflowsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/boundary"
	workflowv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/workflow/v1"
	"github.com/quarkloop/pkg/serviceapi/servicefunction"
)

type ServiceFunctionDispatcher interface {
	Dispatch(context.Context, servicefunction.RequestEnvelope) (servicefunction.ResponseEnvelope, error)
}

type NATSDispatcher struct {
	conn    *natsgo.Conn
	timeout time.Duration
}

func NewNATSDispatcher(conn *natsgo.Conn, timeout time.Duration) *NATSDispatcher {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &NATSDispatcher{conn: conn, timeout: timeout}
}

func (d *NATSDispatcher) Dispatch(ctx context.Context, req servicefunction.RequestEnvelope) (servicefunction.ResponseEnvelope, error) {
	if d == nil || d.conn == nil {
		return servicefunction.ResponseEnvelope{}, fmt.Errorf("nats dispatcher is not configured")
	}
	data, err := json.Marshal(req)
	if err != nil {
		return servicefunction.ResponseEnvelope{}, err
	}
	callCtx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()
	msg, err := d.conn.RequestWithContext(callCtx, req.Subject, data)
	if err != nil {
		return servicefunction.ResponseEnvelope{}, err
	}
	var resp servicefunction.ResponseEnvelope
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		return servicefunction.ResponseEnvelope{}, err
	}
	if err := resp.Validate(); err != nil {
		return servicefunction.ResponseEnvelope{}, err
	}
	if resp.Status == servicefunction.StatusError {
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
	subject := strings.TrimSpace(input.Subject)
	if subject == "" {
		var err error
		subject, err = servicefunction.Subject(input.Service, servicefunction.DefaultVersion, input.Function)
		if err != nil {
			return ServiceFunctionActivityResult{}, err
		}
	}
	req, err := servicefunction.NewRequest(
		fmt.Sprintf("%s.%s", input.WorkflowID, firstNonEmpty(input.StepID, input.Function)),
		input.SpaceID,
		servicefunction.ActorWorkflow,
		servicefunction.Descriptor{Service: input.Service, Function: input.Function, Subject: subject},
		payload,
	)
	if err != nil {
		return ServiceFunctionActivityResult{}, err
	}
	req.WorkflowID = input.WorkflowID
	req.SessionID = input.SessionID
	req.AgentID = input.AgentID
	resp, err := a.dispatcher.Dispatch(ctx, req)
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

func responseError(resp servicefunction.ResponseEnvelope) error {
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
