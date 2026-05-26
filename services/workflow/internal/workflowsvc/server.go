package workflowsvc

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	workflowv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/workflow/v1"
	"github.com/quarkloop/pkg/serviceapi/serviceerrors"
	"google.golang.org/protobuf/proto"
)

type Engine interface {
	Start(context.Context, *workflowv1.StartWorkflowRequest) (*workflowv1.WorkflowInfo, error)
	Signal(context.Context, *workflowv1.SignalWorkflowRequest) (*workflowv1.WorkflowInfo, error)
	Query(context.Context, *workflowv1.QueryWorkflowRequest) (string, error)
	Cancel(context.Context, *workflowv1.CancelWorkflowRequest) (*workflowv1.WorkflowInfo, error)
	Describe(context.Context, *workflowv1.DescribeWorkflowRequest) (*workflowv1.WorkflowInfo, error)
	List(context.Context, *workflowv1.ListWorkflowsRequest) ([]*workflowv1.WorkflowInfo, error)
	Events(context.Context, *workflowv1.StreamWorkflowEventsRequest) (<-chan *workflowv1.WorkflowEvent, error)
	Close()
}

type Server struct {
	engine Engine
	logger *slog.Logger
}

func NewServer(engine Engine, logger *slog.Logger) (*Server, error) {
	if engine == nil {
		return nil, fmt.Errorf("workflow engine is required")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{engine: engine, logger: logger}, nil
}

func (s *Server) Start(ctx context.Context, req *workflowv1.StartWorkflowRequest) (*workflowv1.StartWorkflowResponse, error) {
	if req == nil {
		return nil, serviceerrors.InvalidArgument("start workflow request is required")
	}
	if strings.TrimSpace(req.GetSpaceId()) == "" {
		return nil, serviceerrors.InvalidArgument("space_id is required")
	}
	workflowType := strings.TrimSpace(req.GetWorkflowType())
	if workflowType == "" {
		req = cloneStartRequest(req)
		req.WorkflowType = WorkflowTypeDocumentIngestion
		workflowType = req.WorkflowType
	}
	if workflowType != WorkflowTypeDocumentIngestion {
		return nil, serviceerrors.InvalidArgumentf("unsupported workflow_type %q", workflowType)
	}
	info, err := s.engine.Start(ctx, req)
	if err != nil {
		return nil, err
	}
	return &workflowv1.StartWorkflowResponse{Workflow: info}, nil
}

func (s *Server) Signal(ctx context.Context, req *workflowv1.SignalWorkflowRequest) (*workflowv1.SignalWorkflowResponse, error) {
	if req == nil || strings.TrimSpace(req.GetWorkflowId()) == "" || strings.TrimSpace(req.GetSignal()) == "" {
		return nil, serviceerrors.InvalidArgument("workflow_id and signal are required")
	}
	switch req.GetSignal() {
	case SignalCancel, SignalCheckpointCompleted, SignalCheckpointFailed:
	default:
		return nil, serviceerrors.InvalidArgumentf("unsupported workflow signal %q", req.GetSignal())
	}
	info, err := s.engine.Signal(ctx, req)
	if err != nil {
		return nil, err
	}
	return &workflowv1.SignalWorkflowResponse{Workflow: info}, nil
}

func (s *Server) Query(ctx context.Context, req *workflowv1.QueryWorkflowRequest) (*workflowv1.QueryWorkflowResponse, error) {
	if req == nil || strings.TrimSpace(req.GetWorkflowId()) == "" || strings.TrimSpace(req.GetQuery()) == "" {
		return nil, serviceerrors.InvalidArgument("workflow_id and query are required")
	}
	result, err := s.engine.Query(ctx, req)
	if err != nil {
		return nil, err
	}
	return &workflowv1.QueryWorkflowResponse{ResultJson: result}, nil
}

func (s *Server) Cancel(ctx context.Context, req *workflowv1.CancelWorkflowRequest) (*workflowv1.CancelWorkflowResponse, error) {
	if req == nil || strings.TrimSpace(req.GetWorkflowId()) == "" {
		return nil, serviceerrors.InvalidArgument("workflow_id is required")
	}
	info, err := s.engine.Cancel(ctx, req)
	if err != nil {
		return nil, err
	}
	return &workflowv1.CancelWorkflowResponse{Workflow: info}, nil
}

func (s *Server) Describe(ctx context.Context, req *workflowv1.DescribeWorkflowRequest) (*workflowv1.DescribeWorkflowResponse, error) {
	if req == nil || strings.TrimSpace(req.GetWorkflowId()) == "" {
		return nil, serviceerrors.InvalidArgument("workflow_id is required")
	}
	info, err := s.engine.Describe(ctx, req)
	if err != nil {
		return nil, err
	}
	return &workflowv1.DescribeWorkflowResponse{Workflow: info}, nil
}

func (s *Server) List(ctx context.Context, req *workflowv1.ListWorkflowsRequest) (*workflowv1.ListWorkflowsResponse, error) {
	if req == nil {
		req = &workflowv1.ListWorkflowsRequest{}
	}
	workflows, err := s.engine.List(ctx, req)
	if err != nil {
		return nil, err
	}
	return &workflowv1.ListWorkflowsResponse{Workflows: cloneInfos(workflows)}, nil
}

func (s *Server) EngineEvents(ctx context.Context, req *workflowv1.StreamWorkflowEventsRequest) (<-chan *workflowv1.WorkflowEvent, error) {
	if req == nil || strings.TrimSpace(req.GetWorkflowId()) == "" {
		return nil, serviceerrors.InvalidArgument("workflow_id is required")
	}
	return s.engine.Events(ctx, req)
}

func (s *Server) Close() {
	if s != nil && s.engine != nil {
		s.engine.Close()
	}
}

func cloneStartRequest(req *workflowv1.StartWorkflowRequest) *workflowv1.StartWorkflowRequest {
	if req == nil {
		return nil
	}
	out, ok := proto.Clone(req).(*workflowv1.StartWorkflowRequest)
	if !ok {
		return nil
	}
	out.Metadata = cloneStringMap(req.Metadata)
	return out
}

func cloneInfo(info *workflowv1.WorkflowInfo) *workflowv1.WorkflowInfo {
	if info == nil {
		return nil
	}
	out, ok := proto.Clone(info).(*workflowv1.WorkflowInfo)
	if !ok {
		return nil
	}
	out.Metadata = cloneStringMap(info.Metadata)
	return out
}

func cloneInfos(in []*workflowv1.WorkflowInfo) []*workflowv1.WorkflowInfo {
	out := make([]*workflowv1.WorkflowInfo, 0, len(in))
	for _, info := range in {
		out = append(out, cloneInfo(info))
	}
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
