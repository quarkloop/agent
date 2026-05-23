package workflownats

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	workflowv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/workflow/v1"
	"github.com/quarkloop/pkg/serviceapi/servicefunction"
	"github.com/quarkloop/services/workflow/internal/workflowsvc"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestWorkflowStartEndpointRoundTrip(t *testing.T) {
	ns := startTestNATS(t)
	defer ns.Shutdown()

	engine := newFakeEngine()
	workflowServer, err := workflowsvc.NewServer(engine, nil)
	if err != nil {
		t.Fatalf("workflow server: %v", err)
	}
	endpoints := New(Config{URL: ns.ClientURL(), Timeout: 2 * time.Second}, workflowServer)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := endpoints.Start(ctx); err != nil {
		t.Fatalf("start endpoints: %v", err)
	}
	defer endpoints.Close()

	nc, err := natsgo.Connect(ns.ClientURL())
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer nc.Close()

	payload, err := protojson.Marshal(&workflowv1.StartWorkflowRequest{SpaceId: "space-1"})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	subject, err := servicefunction.Subject("workflow", "v1", "start")
	if err != nil {
		t.Fatalf("subject: %v", err)
	}
	req, err := servicefunction.NewRequest("call-1", "space-1", servicefunction.ActorRuntime, servicefunction.Descriptor{
		Service:  "workflow",
		Function: "start",
		Subject:  subject,
	}, payload)
	if err != nil {
		t.Fatalf("request envelope: %v", err)
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	msg, err := nc.Request(subject, data, 2*time.Second)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	var resp servicefunction.ResponseEnvelope
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != servicefunction.StatusOK {
		t.Fatalf("response = %+v", resp)
	}
	if engine.start.GetSpaceId() != "space-1" {
		t.Fatalf("start request = %+v", engine.start)
	}
}

func startTestNATS(t *testing.T) *natsserver.Server {
	t.Helper()
	ns, err := natsserver.NewServer(&natsserver.Options{Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true})
	if err != nil {
		t.Fatalf("new nats server: %v", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(5 * time.Second) {
		ns.Shutdown()
		t.Fatal("nats server did not become ready")
	}
	return ns
}

type fakeEngine struct {
	events *workflowsvc.EventLog
	start  *workflowv1.StartWorkflowRequest
}

func newFakeEngine() *fakeEngine {
	return &fakeEngine{events: workflowsvc.NewEventLog()}
}

func (e *fakeEngine) Start(_ context.Context, req *workflowv1.StartWorkflowRequest) (*workflowv1.WorkflowInfo, error) {
	e.start = req
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
