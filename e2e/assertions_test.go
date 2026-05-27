//go:build e2e

package e2e

import (
	"strings"
	"testing"

	"github.com/quarkloop/e2e/utils"
)

func TestContainsTextNormalizesCurrencyOrder(t *testing.T) {
	if !containsText("Total paid: 4,872.65 EUR", "EUR 4,872.65") {
		t.Fatal("currency normalization should treat trailing EUR and leading EUR as equivalent")
	}
}

func TestAssertIndexerQueryReturnedStructuredContextAcceptsTracePreview(t *testing.T) {
	trace := utils.MessageTrace{ToolResultEvents: []utils.ToolEvent{{
		Name: "indexer_QueryContext",
		Result: `{"chunks":[{"chunkId":"chunk-1","sourceUri":"docs/example.pdf","text":"` +
			strings.Repeat("x", 2100) + `...(truncated)`,
	}}}
	assertIndexerQueryReturnedStructuredContext(t, trace)
}

func TestAssertIndexerQueryReturnedStructuredContextAcceptsChunkIDPreview(t *testing.T) {
	trace := utils.MessageTrace{ToolResultEvents: []utils.ToolEvent{{
		Name: "indexer_QueryContext",
		Result: `{"chunks":[{"id":"chunk_german_health_insurance","text":"` +
			strings.Repeat("source text before source metadata ", 100) + `...(truncated)`,
	}}}
	assertIndexerQueryReturnedStructuredContext(t, trace)
}

func TestAssertIndexerQueryReturnedStructuredContextAcceptsHyphenatedChunkIDPreview(t *testing.T) {
	trace := utils.MessageTrace{ToolResultEvents: []utils.ToolEvent{{
		Name: "indexer_QueryContext",
		Result: `{"chunks":[{"id":"chunk-german_health_insurance","text":"` +
			strings.Repeat("source text before source metadata ", 100) + `...(truncated)`,
	}}}
	assertIndexerQueryReturnedStructuredContext(t, trace)
}

func TestAssertEmbeddingToolResultAcceptsBatchedRealGatewayResponse(t *testing.T) {
	trace := utils.MessageTrace{ToolResultEvents: []utils.ToolEvent{{
		Name:   "gateway_Embed",
		Result: `{"embeddings":[{"provider":"openrouter","model":"nvidia/llama-nemotron-embed-vl-1b-v2:free","dimensions":2048}]}`,
	}}}
	assertEmbeddingToolResult(t, trace, "openrouter", "nvidia/llama-nemotron-embed-vl-1b-v2:free", 0)
}

func TestAssertToolSuccessCountExactlyAcceptsExpectedResultCount(t *testing.T) {
	trace := utils.MessageTrace{ToolResultEvents: []utils.ToolEvent{{
		Name: "gateway_Embed", Result: `{"success":true}`,
	}}}
	assertToolSuccessCountExactly(t, trace, "gateway_Embed", 1)
}

func TestAssertAnswerExcludesAllowsUserFacingAnswer(t *testing.T) {
	assertAnswerExcludes(t, "The indexed source supports the answer.", "<longcat_tool_call>", "gateway_Embed")
}
