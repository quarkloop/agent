package toolpolicy

import (
	"testing"

	"github.com/quarkloop/pkg/boundary"
)

func TestValidateAllowsReadOnlyIOFunctions(t *testing.T) {
	t.Parallel()

	err := Validate(Invocation{
		Name:      "io_List",
		Arguments: `{"path":"/tmp"}`,
	})
	if err != nil {
		t.Fatalf("read-only io call was denied: %v", err)
	}
}

func TestValidateDeniesAutonomousIOMutations(t *testing.T) {
	t.Parallel()

	err := Validate(Invocation{
		Name:      "io_Remove",
		Arguments: `{"path":"/tmp/quark-space","approved":true}`,
	})
	if !boundary.IsCategory(err, boundary.ApprovalRequired) {
		t.Fatalf("expected approval-required boundary error, got %v", err)
	}
}

func TestValidateAllowsRuntimeApprovedIOMutations(t *testing.T) {
	t.Parallel()

	err := Validate(Invocation{
		Name:            "io_Write",
		Arguments:       `{"path":"/tmp/out.txt","content":"ok","approved":true}`,
		RuntimeApproved: true,
	})
	if err != nil {
		t.Fatalf("runtime-approved io mutation was denied: %v", err)
	}
}

func TestValidateRequiresApprovalForExecute(t *testing.T) {
	t.Parallel()

	err := Validate(Invocation{
		Name:      "io_Execute",
		Arguments: `{"command":"echo ok","approved":true}`,
	})
	if !boundary.IsCategory(err, boundary.ApprovalRequired) {
		t.Fatalf("expected approval-required boundary error, got %v", err)
	}
}

func TestValidateIgnoresUnrelatedTools(t *testing.T) {
	t.Parallel()

	err := Validate(Invocation{
		Name:      "document_ExtractText",
		Arguments: `{}`,
	})
	if err != nil {
		t.Fatalf("unrelated function should pass: %v", err)
	}
}
