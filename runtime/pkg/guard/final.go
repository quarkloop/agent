package guard

import (
	"fmt"
	"strings"

	"github.com/quarkloop/runtime/pkg/llm"
)

// CombineFinalGuards returns the first retry instruction emitted by the given
// guards. Nil guards are ignored.
func CombineFinalGuards(guards ...llm.FinalGuard) llm.FinalGuard {
	active := make([]llm.FinalGuard, 0, len(guards))
	for _, guard := range guards {
		if guard != nil {
			active = append(active, guard)
		}
	}
	if len(active) == 0 {
		return nil
	}
	return func(content string) (string, bool) {
		for _, guard := range active {
			if instruction, retry := guard(content); retry {
				return instruction, true
			}
		}
		return "", false
	}
}

// PendingEmbeddingRefs blocks finalization until the runtime service path has
// consumed all pending embedding references.
func PendingEmbeddingRefs(refs func() []string, maxAttempts int) llm.FinalGuard {
	if refs == nil {
		return nil
	}
	if maxAttempts <= 0 {
		maxAttempts = 8
	}
	attempts := 0
	return func(content string) (string, bool) {
		pending := refs()
		if len(pending) == 0 {
			return "", false
		}
		attempts++
		if attempts > maxAttempts {
			return "", false
		}
		return fmt.Sprintf(
			"Runtime validation blocked finalization. The following embeddingRef values are pending and must be consumed before a final answer: %s. Continue using the existing tool context. If this is an indexing task, use the canonical indexer write path for each pending document embedding. If this is a retrieval task, use the canonical indexer query path with the pending query embedding. Do not produce a final answer until no pending embeddingRef remains.",
			strings.Join(pending, ", "),
		), true
	}
}

// UnconsumedPendingRefsError reports pending references that still exist after
// the model/tool loop has returned a final response.
func UnconsumedPendingRefsError(refs []string) error {
	if len(refs) == 0 {
		return nil
	}
	return fmt.Errorf("runtime validation failed: pending embeddingRef values were not consumed: %s", strings.Join(refs, ", "))
}
