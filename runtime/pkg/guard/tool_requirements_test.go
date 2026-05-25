package guard

import (
	"context"
	"strings"
	"testing"
)

func TestToolRequirementTrackerBlocksUntilDeclaredSuccessfulResults(t *testing.T) {
	tracker := NewToolRequirementTracker("Do not send a final answer until there are 2 successful indexer_UpsertChunk results.")
	if tracker == nil {
		t.Fatal("tracker was not created")
	}

	if instruction, retry := tracker.FinalGuard(""); !retry || !strings.Contains(instruction, "0 successful") {
		t.Fatalf("initial guard = %q retry=%t", instruction, retry)
	}

	wrapped := tracker.WrapToolHandler(func(context.Context, string, string) (string, error) {
		return `{"success": true}`, nil
	})
	if _, err := wrapped(context.Background(), "indexer_UpsertChunk", "{}"); err != nil {
		t.Fatalf("wrapped tool: %v", err)
	}
	if instruction, retry := tracker.FinalGuard(""); !retry || !strings.Contains(instruction, "1 successful") {
		t.Fatalf("guard after one success = %q retry=%t", instruction, retry)
	}

	if _, err := wrapped(context.Background(), "indexer_UpsertChunk", "{}"); err != nil {
		t.Fatalf("wrapped tool: %v", err)
	}
	if instruction, retry := tracker.FinalGuard(""); retry || instruction != "" {
		t.Fatalf("guard after completion = %q retry=%t", instruction, retry)
	}
}

func TestToolRequirementTrackerIgnoresFailedResults(t *testing.T) {
	tracker := NewToolRequirementTracker("until there are 1 successful indexer_UpsertChunk result")
	if tracker == nil {
		t.Fatal("tracker was not created")
	}

	failures := []struct {
		result string
		err    error
	}{
		{result: `{"success": false}`},
		{result: "error: write failed"},
		{err: context.Canceled},
	}
	for _, failure := range failures {
		wrapped := tracker.WrapToolHandler(func(context.Context, string, string) (string, error) {
			return failure.result, failure.err
		})
		_, _ = wrapped(context.Background(), "indexer_UpsertChunk", "{}")
	}

	if instruction, retry := tracker.FinalGuard(""); !retry || !strings.Contains(instruction, "0 successful") {
		t.Fatalf("guard after failures = %q retry=%t", instruction, retry)
	}
}

func TestCombineFinalGuardsUsesFirstRetryInstruction(t *testing.T) {
	guard := CombineFinalGuards(
		func(string) (string, bool) { return "first", true },
		func(string) (string, bool) { return "second", true },
	)

	instruction, retry := guard("")
	if !retry || instruction != "first" {
		t.Fatalf("combined guard = %q retry=%t", instruction, retry)
	}
}

func TestPendingEmbeddingRefsRetriesUntilConsumed(t *testing.T) {
	pending := []string{"ref-1"}
	guard := PendingEmbeddingRefs(func() []string {
		out := make([]string, len(pending))
		copy(out, pending)
		return out
	}, 2)

	if instruction, retry := guard(""); !retry || !strings.Contains(instruction, "ref-1") {
		t.Fatalf("pending guard = %q retry=%t", instruction, retry)
	}
	pending = nil
	if instruction, retry := guard(""); retry || instruction != "" {
		t.Fatalf("consumed guard = %q retry=%t", instruction, retry)
	}
}

func TestPendingEmbeddingRefsStopsAfterMaxAttempts(t *testing.T) {
	guard := PendingEmbeddingRefs(func() []string { return []string{"ref-1"} }, 1)
	if _, retry := guard(""); !retry {
		t.Fatal("expected first retry")
	}
	if instruction, retry := guard(""); retry || instruction != "" {
		t.Fatalf("guard after max attempts = %q retry=%t", instruction, retry)
	}
}
