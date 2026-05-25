package devopssvc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	devopsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/devops/v1"
)

func (s *Server) EvaluateChange(_ context.Context, req *devopsv1.EvaluateChangeRequest) (*devopsv1.EvaluateChangeResponse, error) {
	action := strings.ToLower(strings.TrimSpace(req.GetAction()))
	required := approvalsForAction(action)
	violations := make([]string, 0)
	if _, err := s.workspaces.RelativeFiles(req.GetFiles()); err != nil {
		violations = append(violations, err.Error())
	}
	return &devopsv1.EvaluateChangeResponse{Allowed: len(violations) == 0, Violations: violations, RequiredApprovals: required}, nil
}

func mutationPlan(action, target, reason string, approvalRequired bool, risks ...string) *devopsv1.MutationPlan {
	idSeed := action + "|" + target + "|" + reason + "|" + strings.Join(risks, ",")
	sum := sha256.Sum256([]byte(idSeed))
	return &devopsv1.MutationPlan{
		Id:               hex.EncodeToString(sum[:8]),
		Action:           action,
		Target:           target,
		Reason:           strings.TrimSpace(reason),
		ApprovalRequired: approvalRequired,
		Risks:            append([]string(nil), risks...),
	}
}

func approvalsForAction(action string) []string {
	switch {
	case strings.Contains(action, "commit"):
		return []string{"repo.commit"}
	case strings.Contains(action, "patch") || strings.Contains(action, "write"):
		return []string{"workspace.write"}
	case strings.Contains(action, "test") || strings.Contains(action, "build"):
		return []string{"command.execute"}
	case strings.Contains(action, "container"):
		return []string{"container.build"}
	case strings.Contains(action, "deploy"):
		return []string{"deploy.apply"}
	default:
		return nil
	}
}
