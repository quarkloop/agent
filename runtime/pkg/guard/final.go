package guard

import (
	"encoding/json"
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
		data, _ := json.Marshal(map[string]any{
			"type":               "runtime.reference.validation",
			"status":             "blocked",
			"reason":             "unconsumed_embedding_references",
			"pending_references": append([]string(nil), pending...),
		})
		return string(data), true
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
