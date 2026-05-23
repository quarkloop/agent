package workflowsvc

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	WorkflowTypeDocumentIngestion = "document_ingestion_indexing"
	QueryState                    = "state"
	SignalCancel                  = "cancel"

	ActivityDispatchServiceFunction = "DispatchServiceFunction"
	ActivityRecordWorkflowEvent     = "RecordWorkflowEvent"
)

type DocumentIngestionInput struct {
	WorkflowID string                `json:"workflow_id"`
	SpaceID    string                `json:"space_id"`
	SessionID  string                `json:"session_id,omitempty"`
	AgentID    string                `json:"agent_id,omitempty"`
	Title      string                `json:"title,omitempty"`
	Sources    []WorkflowSourceInput `json:"sources,omitempty"`
	Steps      []ServiceCallInput    `json:"steps,omitempty"`
	Metadata   map[string]string     `json:"metadata,omitempty"`
}

type WorkflowSourceInput struct {
	URI      string            `json:"uri,omitempty"`
	Path     string            `json:"path,omitempty"`
	Filename string            `json:"filename,omitempty"`
	Digest   string            `json:"digest,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type ServiceCallInput struct {
	ID          string `json:"id"`
	Service     string `json:"service"`
	Function    string `json:"function"`
	Subject     string `json:"subject"`
	PayloadJSON string `json:"payload_json"`
	Required    bool   `json:"required"`
}

type WorkflowState struct {
	WorkflowID     string   `json:"workflow_id"`
	Status         string   `json:"status"`
	CurrentStepID  string   `json:"current_step_id,omitempty"`
	CompletedSteps []string `json:"completed_steps,omitempty"`
	LastError      string   `json:"last_error,omitempty"`
}

type DocumentIngestionResult struct {
	State WorkflowState `json:"state"`
}

type ServiceFunctionActivityInput struct {
	WorkflowID  string `json:"workflow_id"`
	SpaceID     string `json:"space_id"`
	SessionID   string `json:"session_id,omitempty"`
	AgentID     string `json:"agent_id,omitempty"`
	StepID      string `json:"step_id"`
	Service     string `json:"service"`
	Function    string `json:"function"`
	Subject     string `json:"subject"`
	PayloadJSON string `json:"payload_json"`
}

type ServiceFunctionActivityResult struct {
	PayloadJSON string `json:"payload_json"`
}

type WorkflowEventRecord struct {
	WorkflowID  string `json:"workflow_id"`
	Type        string `json:"type"`
	StepID      string `json:"step_id,omitempty"`
	Message     string `json:"message,omitempty"`
	PayloadJSON string `json:"payload_json,omitempty"`
}

func DocumentIngestionWorkflow(ctx workflow.Context, input DocumentIngestionInput) (DocumentIngestionResult, error) {
	state := WorkflowState{WorkflowID: input.WorkflowID, Status: "running"}
	if err := workflow.SetQueryHandler(ctx, QueryState, func() (WorkflowState, error) {
		return state, nil
	}); err != nil {
		return DocumentIngestionResult{}, err
	}

	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    3,
		},
	})
	signalCh := workflow.GetSignalChannel(ctx, SignalCancel)

	if err := recordWorkflowEvent(ctx, WorkflowEventRecord{WorkflowID: input.WorkflowID, Type: "workflow_started", Message: input.Title}); err != nil {
		return DocumentIngestionResult{}, err
	}
	for _, step := range input.Steps {
		var cancelPayload string
		if signalCh.ReceiveAsync(&cancelPayload) {
			state.Status = "cancelled"
			state.LastError = cancelPayload
			_ = recordWorkflowEvent(ctx, WorkflowEventRecord{WorkflowID: input.WorkflowID, Type: "workflow_cancelled", Message: cancelPayload})
			return DocumentIngestionResult{State: state}, workflow.ErrCanceled
		}
		state.CurrentStepID = step.ID
		if err := recordWorkflowEvent(ctx, WorkflowEventRecord{WorkflowID: input.WorkflowID, Type: "step_started", StepID: step.ID}); err != nil {
			return DocumentIngestionResult{}, err
		}

		var result ServiceFunctionActivityResult
		err := workflow.ExecuteActivity(ctx, ActivityDispatchServiceFunction, ServiceFunctionActivityInput{
			WorkflowID:  input.WorkflowID,
			SpaceID:     input.SpaceID,
			SessionID:   input.SessionID,
			AgentID:     input.AgentID,
			StepID:      step.ID,
			Service:     step.Service,
			Function:    step.Function,
			Subject:     step.Subject,
			PayloadJSON: step.PayloadJSON,
		}).Get(ctx, &result)
		if err != nil {
			state.LastError = err.Error()
			if recordErr := recordWorkflowEvent(ctx, WorkflowEventRecord{WorkflowID: input.WorkflowID, Type: "step_failed", StepID: step.ID, Message: err.Error()}); recordErr != nil {
				return DocumentIngestionResult{}, recordErr
			}
			if step.Required {
				state.Status = "failed"
				return DocumentIngestionResult{State: state}, err
			}
			continue
		}
		state.CompletedSteps = append(state.CompletedSteps, step.ID)
		if err := recordWorkflowEvent(ctx, WorkflowEventRecord{WorkflowID: input.WorkflowID, Type: "step_completed", StepID: step.ID, PayloadJSON: result.PayloadJSON}); err != nil {
			return DocumentIngestionResult{}, err
		}
	}
	state.CurrentStepID = ""
	state.Status = "succeeded"
	if err := recordWorkflowEvent(ctx, WorkflowEventRecord{WorkflowID: input.WorkflowID, Type: "workflow_completed"}); err != nil {
		return DocumentIngestionResult{}, err
	}
	return DocumentIngestionResult{State: state}, nil
}

func recordWorkflowEvent(ctx workflow.Context, event WorkflowEventRecord) error {
	return workflow.ExecuteActivity(ctx, ActivityRecordWorkflowEvent, event).Get(ctx, nil)
}
