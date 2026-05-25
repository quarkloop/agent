package app

import (
	"context"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/quarkloop/pkg/natskit"
	workflowv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/workflow/v1"
	"github.com/quarkloop/services/workflow/internal/workflowsvc"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestWorkflowHostDispatchesStartOperation(t *testing.T) {
	broker := startWorkflowNATS(t)
	engine := &fakeWorkflowEngine{events: workflowsvc.NewEventLog()}
	server, err := workflowsvc.NewServer(engine, nil)
	if err != nil {
		t.Fatal(err)
	}
	host, err := natskit.StartRPCService(context.Background(), natskit.Config{URL: broker.ClientURL(), Name: "workflow-host"}, workflowBinding("", nil, server))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(host.Close)
	client, err := natskit.Connect(context.Background(), natskit.Config{URL: broker.ClientURL(), Name: "workflow-client"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	payload, _ := protojson.Marshal(&workflowv1.StartWorkflowRequest{SpaceId: "space-1"})
	req, _ := natskit.NewRequest("call-1", "space-1", natskit.ActorRuntime, payload)
	operation, _ := natskit.ServiceOperation("workflow", "start")
	resp, err := client.Call(context.Background(), operation, req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != natskit.StatusOK || engine.start.GetSpaceId() != "space-1" {
		t.Fatalf("response = %+v request = %+v", resp, engine.start)
	}
}

func startWorkflowNATS(t *testing.T) *natsserver.Server {
	t.Helper()
	server, err := natsserver.NewServer(&natsserver.Options{Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true})
	if err != nil {
		t.Fatal(err)
	}
	go server.Start()
	if !server.ReadyForConnections(time.Second) {
		t.Fatal("nats broker not ready")
	}
	t.Cleanup(server.Shutdown)
	return server
}

type fakeWorkflowEngine struct {
	events *workflowsvc.EventLog
	start  *workflowv1.StartWorkflowRequest
}

func (e *fakeWorkflowEngine) Start(_ context.Context, req *workflowv1.StartWorkflowRequest) (*workflowv1.WorkflowInfo, error) {
	e.start = req
	return &workflowv1.WorkflowInfo{WorkflowId: "wf-1", Status: workflowv1.WorkflowStatus_WORKFLOW_STATUS_RUNNING}, nil
}
func (e *fakeWorkflowEngine) Signal(context.Context, *workflowv1.SignalWorkflowRequest) (*workflowv1.WorkflowInfo, error) {
	return &workflowv1.WorkflowInfo{WorkflowId: "wf-1"}, nil
}
func (e *fakeWorkflowEngine) Query(context.Context, *workflowv1.QueryWorkflowRequest) (string, error) {
	return `{}`, nil
}
func (e *fakeWorkflowEngine) Cancel(context.Context, *workflowv1.CancelWorkflowRequest) (*workflowv1.WorkflowInfo, error) {
	return &workflowv1.WorkflowInfo{WorkflowId: "wf-1"}, nil
}
func (e *fakeWorkflowEngine) Describe(context.Context, *workflowv1.DescribeWorkflowRequest) (*workflowv1.WorkflowInfo, error) {
	return &workflowv1.WorkflowInfo{WorkflowId: "wf-1"}, nil
}
func (e *fakeWorkflowEngine) List(context.Context, *workflowv1.ListWorkflowsRequest) ([]*workflowv1.WorkflowInfo, error) {
	return nil, nil
}
func (e *fakeWorkflowEngine) Events(ctx context.Context, req *workflowv1.StreamWorkflowEventsRequest) (<-chan *workflowv1.WorkflowEvent, error) {
	return e.events.Stream(ctx, req.GetWorkflowId(), req.GetIncludeHistory()), nil
}
func (e *fakeWorkflowEngine) Close() {}
