package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	corev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/core/v1"
	"github.com/quarkloop/pkg/serviceapi/serviceerrors"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type Server struct {
	root  string
	store *fileStore
}

func New(root string) (*Server, error) {
	store, err := newFileStore(root)
	if err != nil {
		return nil, err
	}
	return &Server{root: root, store: store}, nil
}

func (s *Server) CheckHealth(context.Context, *corev1.CheckHealthRequest) (*corev1.CheckHealthResponse, error) {
	return &corev1.CheckHealthResponse{
		Ok: true,
		Diagnostics: []*corev1.Diagnostic{{
			Severity: "info",
			Message:  "core service is serving",
		}},
	}, nil
}

func (s *Server) CheckReadiness(context.Context, *corev1.CheckReadinessRequest) (*corev1.CheckReadinessResponse, error) {
	return &corev1.CheckReadinessResponse{
		Ready: true,
		Diagnostics: []*corev1.Diagnostic{{
			Severity: "info",
			Message:  "core state root is ready: " + s.root,
		}},
	}, nil
}

func (s *Server) RecordAuditEvent(_ context.Context, req *corev1.RecordAuditEventRequest) (*corev1.RecordAuditEventResponse, error) {
	event := cloneAuditEvent(req.GetEvent())
	if event.GetRunId() == "" {
		return nil, serviceError(fmt.Errorf("run_id is required"))
	}
	if event.GetAction() == "" {
		return nil, serviceError(fmt.Errorf("action is required"))
	}
	ensureID(&event.Id, "audit")
	ensureTimestamp(&event.CreatedAt)
	redactAuditEvent(event)
	stored, err := s.store.recordAudit(event)
	if err != nil {
		return nil, serviceError(err)
	}
	return &corev1.RecordAuditEventResponse{Event: stored}, nil
}

func (s *Server) ListAuditEvents(_ context.Context, req *corev1.ListAuditEventsRequest) (*corev1.ListAuditEventsResponse, error) {
	events, err := s.store.listAudit(req.GetRunId(), int(req.GetLimit()))
	if err != nil {
		return nil, serviceError(err)
	}
	return &corev1.ListAuditEventsResponse{Events: events}, nil
}

func (s *Server) PutArtifact(_ context.Context, req *corev1.PutArtifactRequest) (*corev1.PutArtifactResponse, error) {
	artifact := cloneArtifact(req.GetArtifact())
	if artifact.GetKind() == "" {
		return nil, serviceError(fmt.Errorf("kind is required"))
	}
	if artifact.GetUri() == "" {
		return nil, serviceError(fmt.Errorf("uri is required"))
	}
	ensureID(&artifact.Id, "artifact")
	ensureTimestamp(&artifact.CreatedAt)
	redactArtifact(artifact)
	stored, err := s.store.putArtifact(artifact)
	if err != nil {
		return nil, serviceError(err)
	}
	return &corev1.PutArtifactResponse{Artifact: stored}, nil
}

func (s *Server) GetArtifact(_ context.Context, req *corev1.GetArtifactRequest) (*corev1.GetArtifactResponse, error) {
	artifact, err := s.store.getArtifact(req.GetId())
	if err != nil {
		return nil, serviceError(err)
	}
	return &corev1.GetArtifactResponse{Artifact: artifact}, nil
}

func (s *Server) RequestApproval(_ context.Context, req *corev1.RequestApprovalRequest) (*corev1.RequestApprovalResponse, error) {
	approval := cloneApproval(req.GetApproval())
	if approval.GetAction() == "" {
		return nil, serviceError(fmt.Errorf("action is required"))
	}
	if approval.GetSubject() == "" {
		return nil, serviceError(fmt.Errorf("subject is required"))
	}
	ensureID(&approval.Id, "approval")
	ensureTimestamp(&approval.CreatedAt)
	if approval.GetStatus() == "" {
		approval.Status = "pending"
	}
	if approval.GetStatus() != "pending" {
		return nil, serviceError(fmt.Errorf("new approval status must be pending"))
	}
	approval.Decision = nil
	stored, err := s.store.putApproval(approval)
	if err != nil {
		return nil, serviceError(err)
	}
	return &corev1.RequestApprovalResponse{Approval: stored}, nil
}

func (s *Server) RecordApprovalDecision(_ context.Context, req *corev1.RecordApprovalDecisionRequest) (*corev1.RecordApprovalDecisionResponse, error) {
	if req.GetApprovalId() == "" {
		return nil, serviceError(fmt.Errorf("approval_id is required"))
	}
	decision := cloneDecision(req.GetDecision())
	if decision.GetActor() == "" {
		return nil, serviceError(fmt.Errorf("decision actor is required"))
	}
	if decision.GetReason() == "" {
		return nil, serviceError(fmt.Errorf("decision reason is required"))
	}
	approval, err := s.store.getApproval(req.GetApprovalId())
	if err != nil {
		return nil, serviceError(err)
	}
	if approval.GetStatus() != "pending" {
		return nil, serviceerrors.FailedPreconditionf("approval %q is already %s", approval.GetId(), approval.GetStatus())
	}
	ensureTimestamp(&decision.DecidedAt)
	approval.Decision = decision
	if decision.GetApproved() {
		approval.Status = "approved"
	} else {
		approval.Status = "denied"
	}
	stored, err := s.store.putApproval(approval)
	if err != nil {
		return nil, serviceError(err)
	}
	return &corev1.RecordApprovalDecisionResponse{Approval: stored}, nil
}

func (s *Server) GetConfig(_ context.Context, req *corev1.GetConfigRequest) (*corev1.GetConfigResponse, error) {
	value, err := s.store.getConfig(req.GetScope(), req.GetKey())
	if err != nil {
		return nil, serviceError(err)
	}
	return &corev1.GetConfigResponse{Value: value}, nil
}

func (s *Server) SetConfig(_ context.Context, req *corev1.SetConfigRequest) (*corev1.SetConfigResponse, error) {
	value := cloneConfig(req.GetValue())
	if value.GetScope() == "" {
		return nil, serviceError(fmt.Errorf("scope is required"))
	}
	if value.GetKey() == "" {
		return nil, serviceError(fmt.Errorf("key is required"))
	}
	if req.GetReason() == "" {
		return nil, serviceError(fmt.Errorf("reason is required"))
	}
	ensureTimestamp(&value.UpdatedAt)
	redactConfig(value)
	stored, err := s.store.putConfig(value)
	if err != nil {
		return nil, serviceError(err)
	}
	return &corev1.SetConfigResponse{Value: stored}, nil
}

func (s *Server) GetSecretRef(_ context.Context, req *corev1.GetSecretRefRequest) (*corev1.GetSecretRefResponse, error) {
	if req.GetScope() == "" {
		return nil, serviceError(fmt.Errorf("scope is required"))
	}
	if req.GetName() == "" {
		return nil, serviceError(fmt.Errorf("name is required"))
	}
	return &corev1.GetSecretRefResponse{Secret: &corev1.SecretRef{
		Scope:         req.GetScope(),
		Name:          req.GetName(),
		Ref:           "secret://" + req.GetScope() + "/" + req.GetName(),
		ValueRedacted: true,
	}}, nil
}

func (s *Server) PublishEvent(_ context.Context, req *corev1.PublishEventRequest) (*corev1.PublishEventResponse, error) {
	event := cloneEvent(req.GetEvent())
	if event.GetStream() == "" {
		return nil, serviceError(fmt.Errorf("stream is required"))
	}
	if event.GetKind() == "" {
		return nil, serviceError(fmt.Errorf("kind is required"))
	}
	ensureID(&event.Id, "event")
	ensureTimestamp(&event.CreatedAt)
	redactEvent(event)
	stored, err := s.store.publishEvent(event)
	if err != nil {
		return nil, serviceError(err)
	}
	return &corev1.PublishEventResponse{Event: stored}, nil
}

func (s *Server) ListEvents(_ context.Context, req *corev1.ListEventsRequest) (*corev1.ListEventsResponse, error) {
	if req.GetStream() == "" {
		return nil, serviceError(fmt.Errorf("stream is required"))
	}
	events, err := s.store.listEvents(req.GetStream(), req.GetAfterSequence(), int(req.GetLimit()))
	if err != nil {
		return nil, serviceError(err)
	}
	return &corev1.ListEventsResponse{Events: events}, nil
}

func (s *Server) EvaluatePolicy(_ context.Context, req *corev1.EvaluatePolicyRequest) (*corev1.EvaluatePolicyResponse, error) {
	if req.GetAction() == "" {
		return nil, serviceError(fmt.Errorf("action is required"))
	}
	return &corev1.EvaluatePolicyResponse{Decision: evaluatePolicy(req.GetAction(), req.GetSubject(), req.GetAttributes())}, nil
}

func (s *Server) CreateWorkspaceMutationPlan(_ context.Context, req *corev1.CreateWorkspaceMutationPlanRequest) (*corev1.CreateWorkspaceMutationPlanResponse, error) {
	plan := clonePlan(req.GetPlan())
	if plan.GetScope() == "" {
		return nil, serviceError(fmt.Errorf("scope is required"))
	}
	if plan.GetAction() == "" {
		return nil, serviceError(fmt.Errorf("action is required"))
	}
	if len(plan.GetPaths()) == 0 {
		return nil, serviceError(fmt.Errorf("at least one path is required"))
	}
	ensureID(&plan.Id, "workspace-plan")
	if plan.GetApprovalRequired() || workspaceMutationNeedsApproval(plan.GetAction()) {
		plan.ApprovalRequired = true
		plan.Status = "pending_approval"
	} else if plan.GetStatus() == "" {
		plan.Status = "planned"
	}
	if len(plan.GetRisks()) == 0 && plan.GetApprovalRequired() {
		plan.Risks = []string{"workspace.write"}
	}
	stored, err := s.store.putPlan(plan)
	if err != nil {
		return nil, serviceError(err)
	}
	return &corev1.CreateWorkspaceMutationPlanResponse{Plan: stored}, nil
}

func (s *Server) ApproveWorkspaceMutationPlan(_ context.Context, req *corev1.ApproveWorkspaceMutationPlanRequest) (*corev1.ApproveWorkspaceMutationPlanResponse, error) {
	if req.GetPlanId() == "" {
		return nil, serviceError(fmt.Errorf("plan_id is required"))
	}
	if req.GetApprovalId() == "" {
		return nil, serviceError(fmt.Errorf("approval_id is required"))
	}
	plan, err := s.store.getPlan(req.GetPlanId())
	if err != nil {
		return nil, serviceError(err)
	}
	approval, err := s.store.getApproval(req.GetApprovalId())
	if err != nil {
		return nil, serviceError(err)
	}
	if approval.GetStatus() != "approved" || approval.GetDecision() == nil || !approval.GetDecision().GetApproved() {
		return nil, serviceerrors.FailedPreconditionf("approval %q is not approved", approval.GetId())
	}
	plan.ApprovalId = approval.GetId()
	plan.Status = "approved"
	stored, err := s.store.putPlan(plan)
	if err != nil {
		return nil, serviceError(err)
	}
	return &corev1.ApproveWorkspaceMutationPlanResponse{Plan: stored}, nil
}

func (s *Server) ScheduleRun(_ context.Context, req *corev1.ScheduleRunRequest) (*corev1.ScheduleRunResponse, error) {
	schedule := cloneSchedule(req.GetSchedule())
	if schedule.GetScope() == "" {
		return nil, serviceError(fmt.Errorf("scope is required"))
	}
	if schedule.GetCron() == "" {
		return nil, serviceError(fmt.Errorf("cron is required"))
	}
	if schedule.GetAction() == "" {
		return nil, serviceError(fmt.Errorf("action is required"))
	}
	ensureID(&schedule.Id, "schedule")
	stored, err := s.store.putSchedule(schedule)
	if err != nil {
		return nil, serviceError(err)
	}
	return &corev1.ScheduleRunResponse{Schedule: stored}, nil
}

func (s *Server) ListSchedules(_ context.Context, req *corev1.ListSchedulesRequest) (*corev1.ListSchedulesResponse, error) {
	schedules, err := s.store.listSchedules(req.GetScope())
	if err != nil {
		return nil, serviceError(err)
	}
	return &corev1.ListSchedulesResponse{Schedules: schedules}, nil
}

func (s *Server) RecordEvaluation(_ context.Context, req *corev1.RecordEvaluationRequest) (*corev1.RecordEvaluationResponse, error) {
	evaluation := cloneEvaluation(req.GetEvaluation())
	if evaluation.GetRunId() == "" {
		return nil, serviceError(fmt.Errorf("run_id is required"))
	}
	if evaluation.GetName() == "" {
		return nil, serviceError(fmt.Errorf("name is required"))
	}
	if evaluation.GetStatus() == "" {
		return nil, serviceError(fmt.Errorf("status is required"))
	}
	ensureID(&evaluation.Id, "evaluation")
	ensureTimestamp(&evaluation.CreatedAt)
	stored, err := s.store.putEvaluation(evaluation)
	if err != nil {
		return nil, serviceError(err)
	}
	return &corev1.RecordEvaluationResponse{Evaluation: stored}, nil
}

func (s *Server) GetEvaluation(_ context.Context, req *corev1.GetEvaluationRequest) (*corev1.GetEvaluationResponse, error) {
	evaluation, err := s.store.getEvaluation(req.GetId())
	if err != nil {
		return nil, serviceError(err)
	}
	return &corev1.GetEvaluationResponse{Evaluation: evaluation}, nil
}

func evaluatePolicy(action, subject string, attributes map[string]string) *corev1.PolicyDecision {
	normalized := strings.ToLower(action + " " + subject)
	violations := make([]string, 0)
	required := make([]string, 0)
	if strings.Contains(normalized, "raw_secret") || strings.Contains(normalized, "exfiltrate") || attributes["policy"] == "deny" {
		violations = append(violations, "policy.denied")
	}
	if strings.Contains(normalized, "delete") || strings.Contains(normalized, "rename") ||
		strings.Contains(normalized, "restructure") || strings.Contains(normalized, "sidecar") ||
		strings.Contains(normalized, "write") {
		required = append(required, "workspace.write")
	}
	if strings.Contains(normalized, "config") {
		required = append(required, "config.write")
	}
	if strings.Contains(normalized, "deploy") || strings.Contains(normalized, "release") {
		required = append(required, "release.publish")
	}
	if strings.Contains(normalized, "kill") || strings.Contains(normalized, "restart") {
		required = append(required, "system.mutate")
	}
	if attributes["risk"] == "admin" || attributes["risk"] == "write" {
		required = append(required, "approval.required")
	}
	return &corev1.PolicyDecision{
		Allowed:           len(violations) == 0,
		Violations:        uniqueStrings(violations),
		RequiredApprovals: uniqueStrings(required),
	}
}

func workspaceMutationNeedsApproval(action string) bool {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "rename", "delete", "sidecar_create", "create_sidecar", "restructure", "write":
		return true
	default:
		return strings.Contains(strings.ToLower(action), "write")
	}
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func ensureID(value *string, prefix string) {
	if strings.TrimSpace(*value) != "" {
		return
	}
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		*value = fmt.Sprintf("%s-%d", prefix, timestamppb.Now().AsTime().UnixNano())
		return
	}
	*value = prefix + "-" + hex.EncodeToString(random)
}

func ensureTimestamp(value **timestamppb.Timestamp) {
	if *value == nil {
		*value = timestamppb.Now()
	}
}

func cloneDecision(in *corev1.ApprovalDecision) *corev1.ApprovalDecision {
	if in == nil {
		return &corev1.ApprovalDecision{}
	}
	return &corev1.ApprovalDecision{
		Actor:     in.GetActor(),
		Approved:  in.GetApproved(),
		Reason:    in.GetReason(),
		DecidedAt: in.GetDecidedAt(),
	}
}

func serviceError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, errNotFound) {
		return serviceerrors.NotFound(err.Error())
	}
	if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "must") || strings.Contains(err.Error(), "reason is") {
		return serviceerrors.InvalidArgument(err.Error())
	}
	return serviceerrors.Internal(err.Error())
}
