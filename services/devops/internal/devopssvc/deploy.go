package devopssvc

import (
	"context"
	"strings"

	devopsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/devops/v1"
	"github.com/quarkloop/pkg/serviceapi/serviceerrors"
)

func (s *Server) Plan(_ context.Context, req *devopsv1.PlanRequest) (*devopsv1.PlanResponse, error) {
	target := firstNonBlank(req.GetTarget(), req.GetEnvironment(), req.GetPath())
	return &devopsv1.PlanResponse{
		Plan:    mutationPlan("deploy.plan", target, "deployment planning only", true, "deploy.plan"),
		Changes: []string{"deployment apply is not executed during planning"},
	}, nil
}

func (s *Server) Apply(_ context.Context, req *devopsv1.ApplyRequest) (*devopsv1.ApplyResponse, error) {
	if strings.TrimSpace(req.GetApprovalId()) == "" {
		return nil, serviceerrors.PermissionDenied("approval_id is required to apply a deployment plan")
	}
	return &devopsv1.ApplyResponse{Result: &devopsv1.TaskResult{Status: taskStatusPlanned, Summary: "deployment apply adapter is not configured"}}, nil
}
