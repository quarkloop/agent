package server

import (
	"context"
	"strings"
	"testing"

	"github.com/quarkloop/pkg/boundary"
	"github.com/quarkloop/services/indexer/internal/indexing"
)

func TestDescriptorPublishesOnlyCanonicalIndexerSubjects(t *testing.T) {
	t.Parallel()
	descriptor := Descriptor(nil)
	if len(descriptor.GetRpcs()) == 0 {
		t.Fatal("descriptor has no service functions")
	}
	for _, rpc := range descriptor.GetRpcs() {
		if rpc.GetOwner() != "indexer" || !strings.HasPrefix(rpc.GetSubject(), "svc.indexer.v1.") {
			t.Fatalf("rpc does not expose a canonical indexer route: %+v", rpc)
		}
		switch rpc.GetMethod() {
		case "IndexDocument", "GetContext":
			t.Fatalf("legacy RPC is still published: %+v", rpc)
		}
	}
}

func TestServiceErrorPreservesBoundaryCategories(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want boundary.Category
	}{
		{name: "invalid input", err: &indexing.ValidationError{Field: "chunk_id", Message: "is required"}, want: boundary.InvalidArgument},
		{name: "canceled", err: context.Canceled, want: boundary.Canceled},
		{name: "deadline", err: context.DeadlineExceeded, want: boundary.Deadline},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := serviceError(test.err); !boundary.IsCategory(got, test.want) {
				t.Fatalf("error = %v, want category %q", got, test.want)
			}
		})
	}
}
