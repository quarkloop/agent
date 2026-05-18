package toolpolicy

import (
	"testing"

	"github.com/quarkloop/pkg/boundary"
)

func TestValidateAllowsReadOnlyFSToolCalls(t *testing.T) {
	t.Parallel()

	err := Validate(Invocation{
		Name:      "fs",
		Arguments: `{"command":"list","path":"/tmp"}`,
	})
	if err != nil {
		t.Fatalf("read-only fs call was denied: %v", err)
	}
}

func TestValidateDeniesAutonomousFSMutationsEvenWithApprovedArgument(t *testing.T) {
	t.Parallel()

	err := Validate(Invocation{
		Name:      "fs",
		Arguments: `{"command":"rm","path":"/tmp/quark-space","approved":true}`,
	})
	if !boundary.IsCategory(err, boundary.ApprovalRequired) {
		t.Fatalf("expected approval-required boundary error, got %v", err)
	}
}

func TestValidateAllowsRuntimeApprovedFSMutations(t *testing.T) {
	t.Parallel()

	err := Validate(Invocation{
		Name:            "fs",
		Arguments:       `{"command":"write","path":"/tmp/out.txt","content":"ok","approved":true}`,
		RuntimeApproved: true,
	})
	if err != nil {
		t.Fatalf("runtime-approved fs mutation was denied: %v", err)
	}
}

func TestValidateIgnoresMalformedArgumentsForPluginValidation(t *testing.T) {
	t.Parallel()

	err := Validate(Invocation{
		Name:      "fs",
		Arguments: `{not-json`,
	})
	if err != nil {
		t.Fatalf("malformed arguments should be left to plugin validation: %v", err)
	}
}
