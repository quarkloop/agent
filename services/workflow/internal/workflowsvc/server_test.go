package workflowsvc

import (
	"context"
	"testing"

	workflowv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/workflow/v1"
)

func TestServerStartDefaultsDocumentIngestionWorkflow(t *testing.T) {
	engine := newFakeEngine()
	server, err := NewServer(engine, nil)
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	resp, err := server.Start(context.Background(), &workflowv1.StartWorkflowRequest{SpaceId: "space-1"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if resp.GetWorkflow().GetWorkflowType() != WorkflowTypeDocumentIngestion {
		t.Fatalf("workflow type = %q", resp.GetWorkflow().GetWorkflowType())
	}
	if engine.start.GetWorkflowType() != WorkflowTypeDocumentIngestion {
		t.Fatalf("engine request workflow type = %q", engine.start.GetWorkflowType())
	}
}

func TestServerRejectsUnsupportedWorkflowType(t *testing.T) {
	server, err := NewServer(newFakeEngine(), nil)
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	if _, err := server.Start(context.Background(), &workflowv1.StartWorkflowRequest{SpaceId: "space-1", WorkflowType: "unknown"}); err == nil {
		t.Fatal("expected unsupported workflow type error")
	}
}

func TestServerStreamsWorkflowEvents(t *testing.T) {
	engine := newFakeEngine()
	server, err := NewServer(engine, nil)
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	events, err := server.EngineEvents(context.Background(), &workflowv1.StreamWorkflowEventsRequest{WorkflowId: "wf-1", IncludeHistory: true})
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	event := <-events
	if event.GetType() != "workflow_started" {
		t.Fatalf("event type = %q", event.GetType())
	}
}

type fakeEngine struct {
	events *EventLog
	start  *workflowv1.StartWorkflowRequest
}

func newFakeEngine() *fakeEngine {
	events := NewEventLog()
	events.Append(&workflowv1.WorkflowEvent{WorkflowId: "wf-1", Type: "workflow_started"})
	return &fakeEngine{events: events}
}

func (e *fakeEngine) Start(_ context.Context, req *workflowv1.StartWorkflowRequest) (*workflowv1.WorkflowInfo, error) {
	e.start = cloneStartRequest(req)
	return &workflowv1.WorkflowInfo{WorkflowId: "wf-1", WorkflowType: req.GetWorkflowType(), Status: workflowv1.WorkflowStatus_WORKFLOW_STATUS_RUNNING}, nil
}

func (e *fakeEngine) Signal(context.Context, *workflowv1.SignalWorkflowRequest) (*workflowv1.WorkflowInfo, error) {
	return &workflowv1.WorkflowInfo{WorkflowId: "wf-1", Status: workflowv1.WorkflowStatus_WORKFLOW_STATUS_RUNNING}, nil
}

func (e *fakeEngine) Query(context.Context, *workflowv1.QueryWorkflowRequest) (string, error) {
	return `{"status":"running"}`, nil
}

func (e *fakeEngine) Cancel(context.Context, *workflowv1.CancelWorkflowRequest) (*workflowv1.WorkflowInfo, error) {
	return &workflowv1.WorkflowInfo{WorkflowId: "wf-1", Status: workflowv1.WorkflowStatus_WORKFLOW_STATUS_CANCELLED}, nil
}

func (e *fakeEngine) Describe(context.Context, *workflowv1.DescribeWorkflowRequest) (*workflowv1.WorkflowInfo, error) {
	return &workflowv1.WorkflowInfo{WorkflowId: "wf-1", Status: workflowv1.WorkflowStatus_WORKFLOW_STATUS_RUNNING}, nil
}

func (e *fakeEngine) List(context.Context, *workflowv1.ListWorkflowsRequest) ([]*workflowv1.WorkflowInfo, error) {
	return []*workflowv1.WorkflowInfo{{WorkflowId: "wf-1", Status: workflowv1.WorkflowStatus_WORKFLOW_STATUS_RUNNING}}, nil
}

func (e *fakeEngine) Events(ctx context.Context, req *workflowv1.StreamWorkflowEventsRequest) (<-chan *workflowv1.WorkflowEvent, error) {
	return e.events.Stream(ctx, req.GetWorkflowId(), req.GetIncludeHistory()), nil
}

func (e *fakeEngine) Close() {}
