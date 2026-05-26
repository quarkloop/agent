package workflowsvc

import (
	"encoding/json"
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	WorkflowTypeDocumentIngestion = "document_ingestion_indexing"
	QueryState                    = "state"
	SignalCancel                  = "cancel"
	SignalCheckpointCompleted     = "checkpoint_completed"
	SignalCheckpointFailed        = "checkpoint_failed"

	ActivityRecordWorkflowEvent = "RecordWorkflowEvent"
)

type DocumentIngestionInput struct {
	WorkflowID  string                `json:"workflow_id"`
	SpaceID     string                `json:"space_id"`
	SessionID   string                `json:"session_id,omitempty"`
	AgentID     string                `json:"agent_id,omitempty"`
	Title       string                `json:"title,omitempty"`
	Sources     []WorkflowSourceInput `json:"sources,omitempty"`
	Checkpoints []CheckpointInput     `json:"checkpoints,omitempty"`
	Metadata    map[string]string     `json:"metadata,omitempty"`
}

type WorkflowSourceInput struct {
	URI      string            `json:"uri,omitempty"`
	Path     string            `json:"path,omitempty"`
	Filename string            `json:"filename,omitempty"`
	Digest   string            `json:"digest,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type CheckpointInput struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
}

type WorkflowState struct {
	WorkflowID           string   `json:"workflow_id"`
	Status               string   `json:"status"`
	CurrentCheckpointID  string   `json:"current_checkpoint_id,omitempty"`
	CompletedCheckpoints []string `json:"completed_checkpoints,omitempty"`
	LastError            string   `json:"last_error,omitempty"`
}

type DocumentIngestionResult struct {
	State WorkflowState `json:"state"`
}

type CheckpointSignal struct {
	CheckpointID string `json:"checkpoint_id"`
	Message      string `json:"message,omitempty"`
	PayloadJSON  string `json:"payload_json,omitempty"`
}

type WorkflowEventRecord struct {
	WorkflowID   string `json:"workflow_id"`
	Type         string `json:"type"`
	CheckpointID string `json:"checkpoint_id,omitempty"`
	Message      string `json:"message,omitempty"`
	PayloadJSON  string `json:"payload_json,omitempty"`
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
	cancelCh := workflow.GetSignalChannel(ctx, SignalCancel)
	completedCh := workflow.GetSignalChannel(ctx, SignalCheckpointCompleted)
	failedCh := workflow.GetSignalChannel(ctx, SignalCheckpointFailed)

	if err := recordWorkflowEvent(ctx, WorkflowEventRecord{WorkflowID: input.WorkflowID, Type: "workflow_started", Message: input.Title}); err != nil {
		return DocumentIngestionResult{}, err
	}
	for _, checkpoint := range input.Checkpoints {
		var cancelPayload string
		if cancelCh.ReceiveAsync(&cancelPayload) {
			state.Status = "cancelled"
			state.LastError = cancelPayload
			_ = recordWorkflowEvent(ctx, WorkflowEventRecord{WorkflowID: input.WorkflowID, Type: "workflow_cancelled", Message: cancelPayload})
			return DocumentIngestionResult{State: state}, workflow.ErrCanceled
		}
		state.CurrentCheckpointID = checkpoint.ID
		if err := recordWorkflowEvent(ctx, WorkflowEventRecord{WorkflowID: input.WorkflowID, Type: "checkpoint_waiting", CheckpointID: checkpoint.ID, Message: checkpoint.Description}); err != nil {
			return DocumentIngestionResult{}, err
		}
		for {
			signalName, signal, err := waitForCheckpointSignal(ctx, cancelCh, completedCh, failedCh)
			if err != nil {
				return DocumentIngestionResult{}, err
			}
			if signalName == SignalCancel {
				state.Status = "cancelled"
				state.LastError = signal.Message
				_ = recordWorkflowEvent(ctx, WorkflowEventRecord{WorkflowID: input.WorkflowID, Type: "workflow_cancelled", Message: signal.Message})
				return DocumentIngestionResult{State: state}, workflow.ErrCanceled
			}
			if signal.CheckpointID != checkpoint.ID {
				if err := recordWorkflowEvent(ctx, WorkflowEventRecord{WorkflowID: input.WorkflowID, Type: "checkpoint_signal_rejected", CheckpointID: checkpoint.ID, Message: "signal does not match current checkpoint"}); err != nil {
					return DocumentIngestionResult{}, err
				}
				continue
			}
			if signalName == SignalCheckpointFailed {
				state.LastError = signal.Message
				if err := recordWorkflowEvent(ctx, WorkflowEventRecord{WorkflowID: input.WorkflowID, Type: "checkpoint_failed", CheckpointID: checkpoint.ID, Message: signal.Message, PayloadJSON: signal.PayloadJSON}); err != nil {
					return DocumentIngestionResult{}, err
				}
				if checkpoint.Required {
					state.Status = "failed"
					return DocumentIngestionResult{State: state}, fmt.Errorf("required checkpoint %q failed: %s", checkpoint.ID, signal.Message)
				}
				break
			}
			state.CompletedCheckpoints = append(state.CompletedCheckpoints, checkpoint.ID)
			if err := recordWorkflowEvent(ctx, WorkflowEventRecord{WorkflowID: input.WorkflowID, Type: "checkpoint_completed", CheckpointID: checkpoint.ID, Message: signal.Message, PayloadJSON: signal.PayloadJSON}); err != nil {
				return DocumentIngestionResult{}, err
			}
			break
		}
	}
	state.CurrentCheckpointID = ""
	state.Status = "succeeded"
	if err := recordWorkflowEvent(ctx, WorkflowEventRecord{WorkflowID: input.WorkflowID, Type: "workflow_completed"}); err != nil {
		return DocumentIngestionResult{}, err
	}
	return DocumentIngestionResult{State: state}, nil
}

func recordWorkflowEvent(ctx workflow.Context, event WorkflowEventRecord) error {
	return workflow.ExecuteActivity(ctx, ActivityRecordWorkflowEvent, event).Get(ctx, nil)
}

func waitForCheckpointSignal(ctx workflow.Context, cancelCh, completedCh, failedCh workflow.ReceiveChannel) (string, CheckpointSignal, error) {
	var (
		name string
		raw  string
	)
	selector := workflow.NewSelector(ctx)
	selector.AddReceive(cancelCh, func(channel workflow.ReceiveChannel, _ bool) {
		channel.Receive(ctx, &raw)
		name = SignalCancel
	})
	selector.AddReceive(completedCh, func(channel workflow.ReceiveChannel, _ bool) {
		channel.Receive(ctx, &raw)
		name = SignalCheckpointCompleted
	})
	selector.AddReceive(failedCh, func(channel workflow.ReceiveChannel, _ bool) {
		channel.Receive(ctx, &raw)
		name = SignalCheckpointFailed
	})
	selector.Select(ctx)
	if name == SignalCancel {
		return name, CheckpointSignal{Message: raw}, nil
	}
	var signal CheckpointSignal
	if err := json.Unmarshal([]byte(raw), &signal); err != nil {
		return "", CheckpointSignal{}, fmt.Errorf("%s payload is invalid: %w", name, err)
	}
	if signal.CheckpointID == "" {
		return "", CheckpointSignal{}, fmt.Errorf("%s payload requires checkpoint_id", name)
	}
	return name, signal, nil
}
