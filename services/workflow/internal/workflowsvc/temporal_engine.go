package workflowsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	workflowv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/workflow/v1"
	enumspb "go.temporal.io/api/enums/v1"
	workflowpb "go.temporal.io/api/workflow/v1"
	workflowservicev1 "go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	temporalworkflow "go.temporal.io/sdk/workflow"
)

const defaultTaskQueue = "quark-workflow"

type TemporalEngine struct {
	client    client.Client
	taskQueue string
	events    *EventLog
}

func NewTemporalEngine(c client.Client, taskQueue string, events *EventLog) (*TemporalEngine, error) {
	if c == nil {
		return nil, fmt.Errorf("temporal client is required")
	}
	if strings.TrimSpace(taskQueue) == "" {
		taskQueue = defaultTaskQueue
	}
	if events == nil {
		events = NewEventLog()
	}
	return &TemporalEngine{client: c, taskQueue: taskQueue, events: events}, nil
}

func RegisterTemporalWorker(w worker.Worker, events *EventLog) {
	activities := NewActivities(events)
	w.RegisterWorkflowWithOptions(DocumentIngestionWorkflow, temporalworkflow.RegisterOptions{Name: WorkflowTypeDocumentIngestion})
	w.RegisterActivityWithOptions(activities.RecordWorkflowEvent, activity.RegisterOptions{Name: ActivityRecordWorkflowEvent})
}

func (e *TemporalEngine) Start(ctx context.Context, req *workflowv1.StartWorkflowRequest) (*workflowv1.WorkflowInfo, error) {
	id := strings.TrimSpace(req.GetWorkflowId())
	if id == "" {
		id = generatedWorkflowID(req.GetSpaceId())
	}
	input := documentInputFromRequest(id, req)
	run, err := e.client.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        id,
		TaskQueue: e.taskQueue,
	}, WorkflowTypeDocumentIngestion, input)
	if err != nil {
		return nil, err
	}
	info := &workflowv1.WorkflowInfo{
		WorkflowId:   run.GetID(),
		RunId:        run.GetRunID(),
		WorkflowType: req.GetWorkflowType(),
		Status:       workflowv1.WorkflowStatus_WORKFLOW_STATUS_RUNNING,
		StartedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:    time.Now().UTC().Format(time.RFC3339Nano),
		Metadata:     cloneStringMap(req.GetMetadata()),
	}
	if info.WorkflowType == "" {
		info.WorkflowType = WorkflowTypeDocumentIngestion
	}
	e.events.Append(&workflowv1.WorkflowEvent{WorkflowId: info.WorkflowId, RunId: info.RunId, Type: "workflow_started", Message: input.Title})
	return info, nil
}

func (e *TemporalEngine) Signal(ctx context.Context, req *workflowv1.SignalWorkflowRequest) (*workflowv1.WorkflowInfo, error) {
	if err := e.client.SignalWorkflow(ctx, req.GetWorkflowId(), req.GetRunId(), req.GetSignal(), req.GetPayloadJson()); err != nil {
		return nil, err
	}
	e.events.Append(&workflowv1.WorkflowEvent{WorkflowId: req.GetWorkflowId(), RunId: req.GetRunId(), Type: "workflow_signaled", Message: req.GetSignal(), PayloadJson: req.GetPayloadJson()})
	return e.Describe(ctx, &workflowv1.DescribeWorkflowRequest{WorkflowId: req.GetWorkflowId(), RunId: req.GetRunId()})
}

func (e *TemporalEngine) Query(ctx context.Context, req *workflowv1.QueryWorkflowRequest) (string, error) {
	value, err := e.client.QueryWorkflow(ctx, req.GetWorkflowId(), req.GetRunId(), req.GetQuery(), req.GetPayloadJson())
	if err != nil {
		return "", err
	}
	if req.GetQuery() == QueryState {
		var state WorkflowState
		if err := value.Get(&state); err != nil {
			return "", err
		}
		data, err := json.Marshal(state)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	var text string
	if err := value.Get(&text); err == nil {
		return text, nil
	}
	var raw map[string]any
	if err := value.Get(&raw); err != nil {
		return "", err
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (e *TemporalEngine) Cancel(ctx context.Context, req *workflowv1.CancelWorkflowRequest) (*workflowv1.WorkflowInfo, error) {
	if req.GetReason() != "" {
		_ = e.client.SignalWorkflow(ctx, req.GetWorkflowId(), req.GetRunId(), SignalCancel, req.GetReason())
	}
	if err := e.client.CancelWorkflow(ctx, req.GetWorkflowId(), req.GetRunId()); err != nil {
		return nil, err
	}
	e.events.Append(&workflowv1.WorkflowEvent{WorkflowId: req.GetWorkflowId(), RunId: req.GetRunId(), Type: "workflow_cancelled", Message: req.GetReason()})
	info, err := e.Describe(ctx, &workflowv1.DescribeWorkflowRequest{WorkflowId: req.GetWorkflowId(), RunId: req.GetRunId()})
	if err != nil {
		return &workflowv1.WorkflowInfo{WorkflowId: req.GetWorkflowId(), RunId: req.GetRunId(), Status: workflowv1.WorkflowStatus_WORKFLOW_STATUS_CANCELLED, LastError: req.GetReason()}, nil
	}
	info.Status = workflowv1.WorkflowStatus_WORKFLOW_STATUS_CANCELLED
	return info, nil
}

func (e *TemporalEngine) Describe(ctx context.Context, req *workflowv1.DescribeWorkflowRequest) (*workflowv1.WorkflowInfo, error) {
	resp, err := e.client.DescribeWorkflowExecution(ctx, req.GetWorkflowId(), req.GetRunId())
	if err != nil {
		return nil, err
	}
	return infoFromTemporal(resp.GetWorkflowExecutionInfo()), nil
}

func (e *TemporalEngine) List(ctx context.Context, req *workflowv1.ListWorkflowsRequest) ([]*workflowv1.WorkflowInfo, error) {
	limit := req.GetLimit()
	if limit <= 0 {
		limit = 50
	}
	query := strings.TrimSpace(req.GetQuery())
	if query == "" && req.GetStatus() != workflowv1.WorkflowStatus_WORKFLOW_STATUS_UNSPECIFIED {
		query = fmt.Sprintf(`ExecutionStatus="%s"`, temporalStatusQueryValue(req.GetStatus()))
	}
	resp, err := e.client.ListWorkflow(ctx, &workflowservicev1.ListWorkflowExecutionsRequest{
		PageSize: int32(limit),
		Query:    query,
	})
	if err != nil {
		return nil, err
	}
	out := make([]*workflowv1.WorkflowInfo, 0, len(resp.GetExecutions()))
	for _, execution := range resp.GetExecutions() {
		info := infoFromTemporal(execution)
		if req.GetSpaceId() != "" && !strings.HasPrefix(info.GetWorkflowId(), stableWorkflowPrefix(req.GetSpaceId())) {
			continue
		}
		out = append(out, info)
	}
	return out, nil
}

func (e *TemporalEngine) Events(ctx context.Context, req *workflowv1.StreamWorkflowEventsRequest) (<-chan *workflowv1.WorkflowEvent, error) {
	return e.events.Stream(ctx, req.GetWorkflowId(), req.GetIncludeHistory()), nil
}

func (e *TemporalEngine) Close() {
	if e != nil && e.client != nil {
		e.client.Close()
	}
}

func documentInputFromRequest(id string, req *workflowv1.StartWorkflowRequest) DocumentIngestionInput {
	ingestion := req.GetDocumentIngestion()
	input := DocumentIngestionInput{
		WorkflowID: id,
		SpaceID:    req.GetSpaceId(),
		SessionID:  req.GetSessionId(),
		AgentID:    req.GetAgentId(),
		Metadata:   cloneStringMap(req.GetMetadata()),
	}
	if ingestion == nil {
		return input
	}
	input.Title = ingestion.GetTitle()
	for _, source := range ingestion.GetSources() {
		input.Sources = append(input.Sources, WorkflowSourceInput{
			URI:      source.GetUri(),
			Path:     source.GetPath(),
			Filename: source.GetFilename(),
			Digest:   source.GetDigest(),
			Metadata: cloneStringMap(source.GetMetadata()),
		})
	}
	for _, checkpoint := range ingestion.GetCheckpoints() {
		input.Checkpoints = append(input.Checkpoints, CheckpointInput{
			ID:          checkpoint.GetId(),
			Description: checkpoint.GetDescription(),
			Required:    checkpoint.GetRequired(),
		})
	}
	return input
}

func infoFromTemporal(info *workflowpb.WorkflowExecutionInfo) *workflowv1.WorkflowInfo {
	if info == nil || info.GetExecution() == nil {
		return &workflowv1.WorkflowInfo{}
	}
	out := &workflowv1.WorkflowInfo{
		WorkflowId: info.GetExecution().GetWorkflowId(),
		RunId:      info.GetExecution().GetRunId(),
		Status:     statusFromTemporal(info.GetStatus()),
	}
	if info.GetType() != nil {
		out.WorkflowType = info.GetType().GetName()
	}
	if info.GetStartTime() != nil {
		out.StartedAt = info.GetStartTime().AsTime().UTC().Format(time.RFC3339Nano)
	}
	if info.GetExecutionTime() != nil {
		out.UpdatedAt = info.GetExecutionTime().AsTime().UTC().Format(time.RFC3339Nano)
	}
	if info.GetCloseTime() != nil {
		out.CompletedAt = info.GetCloseTime().AsTime().UTC().Format(time.RFC3339Nano)
	}
	return out
}

func statusFromTemporal(status enumspb.WorkflowExecutionStatus) workflowv1.WorkflowStatus {
	switch status {
	case enumspb.WORKFLOW_EXECUTION_STATUS_COMPLETED:
		return workflowv1.WorkflowStatus_WORKFLOW_STATUS_SUCCEEDED
	case enumspb.WORKFLOW_EXECUTION_STATUS_FAILED, enumspb.WORKFLOW_EXECUTION_STATUS_TERMINATED, enumspb.WORKFLOW_EXECUTION_STATUS_TIMED_OUT:
		return workflowv1.WorkflowStatus_WORKFLOW_STATUS_FAILED
	case enumspb.WORKFLOW_EXECUTION_STATUS_CANCELED:
		return workflowv1.WorkflowStatus_WORKFLOW_STATUS_CANCELLED
	case enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING:
		return workflowv1.WorkflowStatus_WORKFLOW_STATUS_RUNNING
	default:
		return workflowv1.WorkflowStatus_WORKFLOW_STATUS_UNSPECIFIED
	}
}

func temporalStatusQueryValue(status workflowv1.WorkflowStatus) string {
	switch status {
	case workflowv1.WorkflowStatus_WORKFLOW_STATUS_RUNNING:
		return "Running"
	case workflowv1.WorkflowStatus_WORKFLOW_STATUS_SUCCEEDED:
		return "Completed"
	case workflowv1.WorkflowStatus_WORKFLOW_STATUS_FAILED:
		return "Failed"
	case workflowv1.WorkflowStatus_WORKFLOW_STATUS_CANCELLED:
		return "Canceled"
	default:
		return "Running"
	}
}

func generatedWorkflowID(spaceID string) string {
	return fmt.Sprintf("%sworkflow.%d", stableWorkflowPrefix(spaceID), time.Now().UTC().UnixNano())
}

func stableWorkflowPrefix(spaceID string) string {
	token := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		case r >= '0' && r <= '9':
			return r
		default:
			return '-'
		}
	}, strings.TrimSpace(spaceID))
	token = strings.Trim(token, "-")
	if token == "" {
		token = "space"
	}
	return token + "."
}
