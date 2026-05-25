package workspace

import "testing"

func TestPlanSidecarsDisabledDoesNothing(t *testing.T) {
	t.Parallel()

	plan, err := PlanSidecars([]SourceFile{{Path: "/tmp/source.pdf"}}, SidecarOptions{})
	if err != nil {
		t.Fatalf("disabled plan returned error: %v", err)
	}
	if plan.Approved || len(plan.Changes) != 0 {
		t.Fatalf("disabled plan = %+v, want empty", plan)
	}
}

func TestPlanSidecarsRequiresApproval(t *testing.T) {
	t.Parallel()

	_, err := PlanSidecars([]SourceFile{{Path: "/tmp/source.pdf"}}, SidecarOptions{
		Enabled:        true,
		CreateSidecars: true,
	})
	if err == nil {
		t.Fatal("expected approval error")
	}
}

func TestPlanSidecarsUsesDeterministicNames(t *testing.T) {
	t.Parallel()

	plan, err := PlanSidecars([]SourceFile{{
		Path:         "/tmp/My Source File.pdf",
		SourceHash:   "abc123",
		DetectedType: "Research Paper",
		Title:        "Attention Is All You Need",
		IndexIDs:     []string{"chunk-1"},
	}}, SidecarOptions{
		Enabled:          true,
		Approved:         true,
		CreateSidecars:   true,
		RestructureFiles: true,
	})
	if err != nil {
		t.Fatalf("plan sidecars: %v", err)
	}
	if len(plan.Changes) != 1 {
		t.Fatalf("changes = %d, want 1", len(plan.Changes))
	}
	change := plan.Changes[0]
	if change.SidecarPath != "/tmp/My Source File.quark.json" {
		t.Fatalf("sidecar path = %q", change.SidecarPath)
	}
	if change.RenamePath != "/tmp/research-paper-attention-is-all-you-need.pdf" {
		t.Fatalf("rename path = %q", change.RenamePath)
	}
	if change.Metadata.SourceHash != "abc123" || len(change.Metadata.IndexIDs) != 1 {
		t.Fatalf("metadata = %+v", change.Metadata)
	}
}
