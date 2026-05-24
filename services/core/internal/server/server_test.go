package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/quarkloop/pkg/boundary"
	corev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/core/v1"
)

func TestCoreServicePersistsArtifactReferences(t *testing.T) {
	root := t.TempDir()
	srv, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	health, err := srv.CheckHealth(context.Background(), &corev1.CheckHealthRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !health.GetOk() {
		t.Fatal("expected healthy core service")
	}

	put, err := srv.PutArtifact(context.Background(), &corev1.PutArtifactRequest{Artifact: &corev1.Artifact{
		Kind:   "trace",
		Uri:    "artifact://runs/run-1/trace.json",
		Digest: "sha256:abc",
	}})
	if err != nil {
		t.Fatal(err)
	}

	reopened, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.GetArtifact(context.Background(), &corev1.GetArtifactRequest{Id: put.GetArtifact().GetId()})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetArtifact().GetUri() != "artifact://runs/run-1/trace.json" {
		t.Fatalf("artifact URI mismatch: %q", got.GetArtifact().GetUri())
	}
}

func TestCoreStoreRecreatesStateRootOnSave(t *testing.T) {
	root := t.TempDir()
	srv, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatalf("remove state root: %v", err)
	}

	if _, err := srv.PublishEvent(context.Background(), &corev1.PublishEventRequest{Event: &corev1.Event{
		Id:     "event-1",
		Stream: "session/session-1",
		Kind:   "tool_result",
	}}); err != nil {
		t.Fatalf("publish after root removal: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, storeFileName)); err != nil {
		t.Fatalf("state file was not recreated: %v", err)
	}
}

func TestAuditEventsAreRedactedAndImmutable(t *testing.T) {
	srv, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	input := &corev1.AuditEvent{
		RunId:  "run-1",
		Actor:  "agent",
		Action: "read",
		Target: "OPENROUTER_API_KEY=secret",
	}
	resp, err := srv.RecordAuditEvent(context.Background(), &corev1.RecordAuditEventRequest{Event: input})
	if err != nil {
		t.Fatal(err)
	}
	if input.GetTarget() != "OPENROUTER_API_KEY=secret" {
		t.Fatal("server mutated ingress audit event")
	}
	if !resp.GetEvent().GetRedacted() || resp.GetEvent().GetTarget() != "[redacted]" {
		t.Fatalf("expected redacted audit event, got %#v", resp.GetEvent())
	}

	first, err := srv.ListAuditEvents(context.Background(), &corev1.ListAuditEventsRequest{RunId: "run-1"})
	if err != nil {
		t.Fatal(err)
	}
	first.Events[0].Action = "mutated outside store"
	second, err := srv.ListAuditEvents(context.Background(), &corev1.ListAuditEventsRequest{RunId: "run-1"})
	if err != nil {
		t.Fatal(err)
	}
	if second.GetEvents()[0].GetAction() != "read" {
		t.Fatalf("stored event was mutable through returned DTO: %q", second.GetEvents()[0].GetAction())
	}
}

func TestApprovalAndWorkspacePlanFlow(t *testing.T) {
	srv, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := srv.CreateWorkspaceMutationPlan(context.Background(), &corev1.CreateWorkspaceMutationPlanRequest{Plan: &corev1.WorkspaceMutationPlan{
		Scope:  "space-a",
		Action: "rename",
		Paths:  []string{"docs/old.pdf", "docs/new.pdf"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.GetPlan().GetApprovalRequired() || plan.GetPlan().GetStatus() != "pending_approval" {
		t.Fatalf("expected pending approval plan, got %#v", plan.GetPlan())
	}
	denied, err := srv.RequestApproval(context.Background(), &corev1.RequestApprovalRequest{Approval: &corev1.ApprovalRequest{
		Action:  "rename",
		Subject: plan.GetPlan().GetId(),
		Risks:   []string{"workspace.write"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.RecordApprovalDecision(context.Background(), &corev1.RecordApprovalDecisionRequest{
		ApprovalId: denied.GetApproval().GetId(),
		Decision:   &corev1.ApprovalDecision{Actor: "operator", Approved: false, Reason: "not now"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.ApproveWorkspaceMutationPlan(context.Background(), &corev1.ApproveWorkspaceMutationPlanRequest{
		PlanId:     plan.GetPlan().GetId(),
		ApprovalId: denied.GetApproval().GetId(),
	}); !boundary.IsCategory(err, boundary.Conflict) {
		t.Fatalf("expected denied approval to block plan, got %v", err)
	}

	approved, err := srv.RequestApproval(context.Background(), &corev1.RequestApprovalRequest{Approval: &corev1.ApprovalRequest{
		Action:  "rename",
		Subject: plan.GetPlan().GetId(),
		Risks:   []string{"workspace.write"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.RecordApprovalDecision(context.Background(), &corev1.RecordApprovalDecisionRequest{
		ApprovalId: approved.GetApproval().GetId(),
		Decision:   &corev1.ApprovalDecision{Actor: "operator", Approved: true, Reason: "approved"},
	}); err != nil {
		t.Fatal(err)
	}
	bound, err := srv.ApproveWorkspaceMutationPlan(context.Background(), &corev1.ApproveWorkspaceMutationPlanRequest{
		PlanId:     plan.GetPlan().GetId(),
		ApprovalId: approved.GetApproval().GetId(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if bound.GetPlan().GetStatus() != "approved" || bound.GetPlan().GetApprovalId() != approved.GetApproval().GetId() {
		t.Fatalf("approval not bound to plan: %#v", bound.GetPlan())
	}
}

func TestConfigPolicySecretsAndEvents(t *testing.T) {
	srv, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	policy, err := srv.EvaluatePolicy(context.Background(), &corev1.EvaluatePolicyRequest{
		Action: "raw_secret export",
	})
	if err != nil {
		t.Fatal(err)
	}
	if policy.GetDecision().GetAllowed() || len(policy.GetDecision().GetViolations()) == 0 {
		t.Fatalf("expected policy denial, got %#v", policy.GetDecision())
	}

	cfg, err := srv.SetConfig(context.Background(), &corev1.SetConfigRequest{
		Value:  &corev1.ConfigValue{Scope: "space-a", Key: "openrouter_api_key", ValueJson: `"sk-secret"`},
		Reason: "configure provider",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.GetValue().GetRedacted() || cfg.GetValue().GetValueJson() == `"sk-secret"` {
		t.Fatalf("secret config was not redacted: %#v", cfg.GetValue())
	}
	ref, err := srv.GetSecretRef(context.Background(), &corev1.GetSecretRefRequest{Scope: "space-a", Name: "openrouter_api_key"})
	if err != nil {
		t.Fatal(err)
	}
	if !ref.GetSecret().GetValueRedacted() || ref.GetSecret().GetRef() == "" {
		t.Fatalf("secret ref should expose only a reference: %#v", ref.GetSecret())
	}

	first, err := srv.PublishEvent(context.Background(), &corev1.PublishEventRequest{Event: &corev1.Event{
		Stream:      "run-1",
		Kind:        "tool",
		PayloadJson: `{"ok":true}`,
	}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := srv.PublishEvent(context.Background(), &corev1.PublishEventRequest{Event: &corev1.Event{
		Stream:      "run-1",
		Kind:        "model",
		PayloadJson: `{"token":"secret"}`,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if first.GetEvent().GetSequence() != 1 || second.GetEvent().GetSequence() != 2 {
		t.Fatalf("event sequence mismatch: first=%d second=%d", first.GetEvent().GetSequence(), second.GetEvent().GetSequence())
	}
	list, err := srv.ListEvents(context.Background(), &corev1.ListEventsRequest{Stream: "run-1", AfterSequence: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.GetEvents()) != 1 || list.GetEvents()[0].GetSequence() != 2 || !list.GetEvents()[0].GetRedacted() {
		t.Fatalf("expected redacted second event after sequence 1, got %#v", list.GetEvents())
	}
}
