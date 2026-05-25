package workflowsvc

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/quarkloop/pkg/serviceapi/servicefunction"
	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
)

func TestDocumentIngestionWorkflowDispatchesRequiredSteps(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(DocumentIngestionWorkflow)
	registerTestActivities(env)
	env.OnActivity(ActivityRecordWorkflowEvent, mock.Anything, mock.Anything).Return(nil).Maybe()
	env.OnActivity(ActivityDispatchServiceFunction, mock.Anything, mock.MatchedBy(func(input ServiceFunctionActivityInput) bool {
		return input.StepID == "extract" && input.Service == "document"
	})).Return(ServiceFunctionActivityResult{PayloadJSON: `{"text":"hello"}`}, nil).Once()
	env.OnActivity(ActivityDispatchServiceFunction, mock.Anything, mock.MatchedBy(func(input ServiceFunctionActivityInput) bool {
		return input.StepID == "index" && input.Service == "indexer"
	})).Return(ServiceFunctionActivityResult{PayloadJSON: `{"indexed":true}`}, nil).Once()

	env.ExecuteWorkflow(DocumentIngestionWorkflow, DocumentIngestionInput{
		WorkflowID: "wf-1",
		SpaceID:    "space-1",
		Title:      "Index documents",
		Steps: []ServiceCallInput{
			{ID: "extract", Service: "document", Function: "extract_text", Required: true},
			{ID: "index", Service: "indexer", Function: "index_record", Required: true},
		},
	})
	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}
	var result DocumentIngestionResult
	if err := env.GetWorkflowResult(&result); err != nil {
		t.Fatalf("workflow result: %v", err)
	}
	if result.State.Status != "succeeded" {
		t.Fatalf("status = %q, want succeeded", result.State.Status)
	}
	if got := strings.Join(result.State.CompletedSteps, ","); got != "extract,index" {
		t.Fatalf("completed steps = %q", got)
	}
	env.AssertExpectations(t)
}

func TestDocumentIngestionWorkflowFailsRequiredStep(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(DocumentIngestionWorkflow)
	registerTestActivities(env)
	env.OnActivity(ActivityRecordWorkflowEvent, mock.Anything, mock.Anything).Return(nil).Maybe()
	env.OnActivity(ActivityDispatchServiceFunction, mock.Anything, mock.Anything).Return(ServiceFunctionActivityResult{}, errors.New("index unavailable")).Times(3)

	env.ExecuteWorkflow(DocumentIngestionWorkflow, DocumentIngestionInput{
		WorkflowID: "wf-1",
		SpaceID:    "space-1",
		Steps:      []ServiceCallInput{{ID: "index", Service: "indexer", Function: "index_record", Required: true}},
	})
	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err == nil {
		t.Fatal("expected workflow error")
	} else if !strings.Contains(err.Error(), "index unavailable") {
		t.Fatalf("workflow error = %v", err)
	}
	env.AssertExpectations(t)
}

func TestActivitiesDispatchServiceFunctionBuildsWorkflowEnvelope(t *testing.T) {
	dispatcher := &recordingDispatcher{}
	activities := NewActivities(dispatcher, NewEventLog())
	result, err := activities.DispatchServiceFunction(context.Background(), ServiceFunctionActivityInput{
		WorkflowID:  "wf-1",
		SpaceID:     "space-1",
		SessionID:   "session-1",
		AgentID:     "agent-1",
		StepID:      "extract",
		Service:     "document",
		Function:    "extract_text",
		PayloadJSON: `{"path":"/tmp/a.pdf"}`,
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if result.PayloadJSON != `{"ok":true}` {
		t.Fatalf("payload = %s", result.PayloadJSON)
	}
	req := dispatcher.request
	if req.Actor != "workflow" || req.WorkflowID != "wf-1" || req.SpaceID != "space-1" {
		t.Fatalf("request correlation = %+v", req)
	}
	if req.Subject != "svc.document.v1.extract_text" {
		t.Fatalf("subject = %q", req.Subject)
	}
}

type recordingDispatcher struct {
	request servicefunction.RequestEnvelope
}

func (d *recordingDispatcher) Dispatch(_ context.Context, req servicefunction.RequestEnvelope) (servicefunction.ResponseEnvelope, error) {
	d.request = req.Clone()
	return servicefunction.OKResponse(req.ServiceCallID, []byte(`{"ok":true}`)), nil
}

type workflowTestEnv interface {
	RegisterActivityWithOptions(any, activity.RegisterOptions)
}

func registerTestActivities(env workflowTestEnv) {
	env.RegisterActivityWithOptions(func(context.Context, WorkflowEventRecord) error {
		return nil
	}, activity.RegisterOptions{Name: ActivityRecordWorkflowEvent})
	env.RegisterActivityWithOptions(func(context.Context, ServiceFunctionActivityInput) (ServiceFunctionActivityResult, error) {
		return ServiceFunctionActivityResult{}, nil
	}, activity.RegisterOptions{Name: ActivityDispatchServiceFunction})
}
