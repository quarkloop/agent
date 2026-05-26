package workflowsvc

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/testsuite"
)

func TestDocumentIngestionWorkflowWaitsForAgentCheckpointSignals(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(DocumentIngestionWorkflow)
	registerTestActivities(env)
	env.OnActivity(ActivityRecordWorkflowEvent, mock.Anything, mock.Anything).Return(nil).Maybe()
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalCheckpointCompleted, `{"checkpoint_id":"extract","payload_json":"{\"text\":\"hello\"}"}`)
	}, time.Second)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalCheckpointCompleted, `{"checkpoint_id":"index","payload_json":"{\"indexed\":true}"}`)
	}, 2*time.Second)

	env.ExecuteWorkflow(DocumentIngestionWorkflow, DocumentIngestionInput{
		WorkflowID: "wf-1",
		SpaceID:    "space-1",
		Title:      "Index documents",
		Checkpoints: []CheckpointInput{
			{ID: "extract", Description: "Agent has extracted the document", Required: true},
			{ID: "index", Description: "Agent has indexed canonical records", Required: true},
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
	if got := strings.Join(result.State.CompletedCheckpoints, ","); got != "extract,index" {
		t.Fatalf("completed checkpoints = %q", got)
	}
	env.AssertExpectations(t)
}

func TestDocumentIngestionWorkflowFailsRequiredCheckpointSignal(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(DocumentIngestionWorkflow)
	registerTestActivities(env)
	env.OnActivity(ActivityRecordWorkflowEvent, mock.Anything, mock.Anything).Return(nil).Maybe()
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalCheckpointFailed, `{"checkpoint_id":"index","message":"index unavailable"}`)
	}, time.Second)

	env.ExecuteWorkflow(DocumentIngestionWorkflow, DocumentIngestionInput{
		WorkflowID:  "wf-1",
		SpaceID:     "space-1",
		Checkpoints: []CheckpointInput{{ID: "index", Required: true}},
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

func TestDocumentIngestionWorkflowDoesNotAcceptWrongCheckpoint(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(DocumentIngestionWorkflow)
	registerTestActivities(env)
	env.OnActivity(ActivityRecordWorkflowEvent, mock.Anything, mock.Anything).Return(nil).Maybe()
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalCheckpointCompleted, `{"checkpoint_id":"future"}`)
	}, time.Second)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalCheckpointCompleted, `{"checkpoint_id":"extract"}`)
	}, 2*time.Second)

	env.ExecuteWorkflow(DocumentIngestionWorkflow, DocumentIngestionInput{
		WorkflowID:  "wf-1",
		SpaceID:     "space-1",
		Checkpoints: []CheckpointInput{{ID: "extract", Required: true}},
	})
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}
	var result DocumentIngestionResult
	if err := env.GetWorkflowResult(&result); err != nil || len(result.State.CompletedCheckpoints) != 1 {
		t.Fatalf("workflow result = %+v, %v", result, err)
	}
}

type workflowTestEnv interface {
	RegisterActivityWithOptions(any, activity.RegisterOptions)
}

func registerTestActivities(env workflowTestEnv) {
	env.RegisterActivityWithOptions(func(context.Context, WorkflowEventRecord) error {
		return nil
	}, activity.RegisterOptions{Name: ActivityRecordWorkflowEvent})
}
