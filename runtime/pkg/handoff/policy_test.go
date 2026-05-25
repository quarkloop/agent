package handoff

import "testing"

func TestPolicyCopiesAndSortsTargets(t *testing.T) {
	targets := []string{" quark-system ", "", "quark-devops"}
	policy := NewPolicy("quark-knowledge", targets)
	targets[0] = "mutated"

	got := policy.Targets()
	if len(got) != 2 || got[0] != "quark-devops" || got[1] != "quark-system" {
		t.Fatalf("targets = %#v", got)
	}
}

func TestPolicyValidatesAllowedTarget(t *testing.T) {
	policy := NewPolicy("quark-knowledge", []string{"quark-devops"})
	if err := policy.ValidateTarget("quark-devops"); err != nil {
		t.Fatalf("validate target: %v", err)
	}
	if err := policy.ValidateTarget("quark-system"); err == nil {
		t.Fatal("expected disallowed target error")
	}
}
