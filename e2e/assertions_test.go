//go:build e2e

package e2e

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/quarkloop/e2e/utils"
)

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsText(value, want string) bool {
	return strings.Contains(strings.ToLower(canonicalText(value)), strings.ToLower(canonicalText(want)))
}

func containsAnyText(value string, wants ...string) bool {
	for _, want := range wants {
		if containsText(value, want) {
			return true
		}
	}
	return false
}

func canonicalText(value string) string {
	replacer := strings.NewReplacer(
		"\u00ad", "",
		"\u2010", "-",
		"\u2011", "-",
		"\u2012", "-",
		"\u2013", "-",
		"\u2014", "-",
		"\u2212", "-",
		"\u00a0", " ",
		"\u2007", " ",
		"\u2009", " ",
		"\u202f", " ",
		"\u20ac", "EUR",
	)
	value = replacer.Replace(value)
	for strings.Contains(value, "EUR ") {
		value = strings.ReplaceAll(value, "EUR ", "EUR")
	}
	return value
}

func assertToolStarted(t *testing.T, trace utils.MessageTrace, name string) {
	t.Helper()
	if !contains(trace.ToolStarts, name) {
		t.Fatalf("agent did not start %s tool; starts=%v", name, trace.ToolStarts)
	}
}

func assertToolResultContains(t *testing.T, trace utils.MessageTrace, tool string, wants ...string) {
	t.Helper()
	for _, event := range trace.ToolResultEvents {
		if event.Name != tool {
			continue
		}
		missing := false
		for _, want := range wants {
			if !containsText(event.Result, want) {
				missing = true
				break
			}
		}
		if !missing {
			return
		}
	}
	t.Fatalf("%s tool results missing %v: %+v", tool, wants, trace.ToolResultEvents)
}

func assertEmbeddingToolResult(t *testing.T, trace utils.MessageTrace, provider, model string, dimensions int) {
	t.Helper()
	for _, event := range trace.ToolResultEvents {
		if event.Name != "embedding_Embed" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(event.Result), &payload); err != nil {
			continue
		}
		gotProvider := strings.TrimSpace(fmt.Sprint(payload["provider"]))
		gotModel := strings.TrimSpace(fmt.Sprint(payload["model"]))
		gotDimensions := strings.TrimSpace(fmt.Sprint(payload["dimensions"]))
		if gotProvider != provider {
			continue
		}
		if gotDimensions != fmt.Sprint(dimensions) {
			continue
		}
		if embeddingModelMatches(provider, gotModel, model) {
			return
		}
	}
	t.Fatalf("embedding_Embed tool results missing provider=%q model=%q dimensions=%d: %+v", provider, model, dimensions, trace.ToolResultEvents)
}

func embeddingModelMatches(provider, got, want string) bool {
	if strings.EqualFold(strings.TrimSpace(got), strings.TrimSpace(want)) {
		return true
	}
	return normalizeEmbeddingModel(provider, got) == normalizeEmbeddingModel(provider, want)
}

func normalizeEmbeddingModel(provider, model string) string {
	normalized := strings.ToLower(strings.TrimSpace(model))
	normalized = strings.TrimPrefix(normalized, "private/")
	if strings.EqualFold(provider, "openrouter") {
		normalized = strings.TrimPrefix(normalized, "openrouter/")
	}
	normalized = strings.TrimSuffix(normalized, ":free")
	return normalized
}

func assertNoToolErrors(t *testing.T, trace utils.MessageTrace, tools ...string) {
	t.Helper()
	wanted := make(map[string]bool, len(tools))
	for _, tool := range tools {
		wanted[tool] = true
	}
	for _, event := range trace.ToolResultEvents {
		if !wanted[event.Name] {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(event.Result), "error:") {
			t.Fatalf("%s returned an error: %s", event.Name, event.Result)
		}
	}
}

func assertToolSuccessCount(t *testing.T, trace utils.MessageTrace, tool string, want int) {
	t.Helper()
	count := 0
	for _, event := range trace.ToolResultEvents {
		if event.Name != tool {
			continue
		}
		compact := strings.ReplaceAll(event.Result, " ", "")
		if strings.Contains(compact, `"success":true`) {
			count++
		}
	}
	if count < want {
		t.Fatalf("%s successful results = %d, want at least %d: %+v", tool, count, want, trace.ToolResultEvents)
	}
}

func assertEmbeddingSuccessCount(t *testing.T, trace utils.MessageTrace, want int) {
	t.Helper()
	count := 0
	for _, event := range trace.ToolResultEvents {
		if event.Name != "embedding_Embed" {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(event.Result), "error:") {
			continue
		}
		if containsText(event.Result, "embeddingRef") {
			count++
		}
	}
	if count < want {
		t.Fatalf("embedding_Embed successful results = %d, want at least %d: %+v", count, want, trace.ToolResultEvents)
	}
}

func assertAgentStructuredPDFIndexPayloads(t *testing.T, trace utils.MessageTrace, documents []indexedPDFDocument) {
	t.Helper()
	payloads := indexerUpsertChunkPayloads(t, trace)
	if len(payloads) < len(documents) {
		t.Fatalf("indexer_UpsertChunk payload count = %d, want at least %d", len(payloads), len(documents))
	}

	seen := make(map[string]bool, len(documents))
	for i, payload := range payloads {
		if !hasNonEmptyString(payload, "chunkId") {
			t.Fatalf("UpsertChunk payload %d missing chunkId: %+v", i, payload)
		}
		if !hasNonEmptyString(payload, "textContentRef") && !hasNonEmptyString(payload, "textContent") {
			t.Fatalf("UpsertChunk payload %d missing textContentRef or textContent: %+v", i, payload)
		}
		if !hasNonEmptyString(payload, "embeddingRef") {
			t.Fatalf("UpsertChunk payload %d missing embeddingRef from embedding_Embed: %+v", i, payload)
		}
		if !hasNonEmptyObject(payload, "document") {
			t.Fatalf("UpsertChunk payload %d missing first-class document metadata: %+v", i, payload)
		}
		if !hasNonEmptyObject(payload, "sourceMetadata") {
			t.Fatalf("UpsertChunk payload %d missing sourceMetadata: %+v", i, payload)
		}
		if !hasNonEmptyArray(payload, "facts") {
			t.Fatalf("UpsertChunk payload %d missing agent-produced facts: %+v", i, payload)
		}
		if !hasNonEmptyArray(payload, "entities") {
			t.Fatalf("UpsertChunk payload %d missing agent-produced entities: %+v", i, payload)
		}
		if !hasArray(payload, "relations") {
			t.Fatalf("UpsertChunk payload %d missing relations field: %+v", i, payload)
		}
		if !hasCitationEvidence(payload) {
			t.Fatalf("UpsertChunk payload %d missing source citations: %+v", i, payload)
		}
		if !hasNonEmptyObject(payload, "provenance") {
			t.Fatalf("UpsertChunk payload %d missing provenance: %+v", i, payload)
		}
		for _, document := range documents {
			if payloadMentionsFilename(payload, document.Filename) {
				seen[document.Filename] = true
			}
		}
	}
	for _, document := range documents {
		if !seen[document.Filename] {
			t.Fatalf("no structured UpsertChunk payload referenced %s: %+v", document.Filename, payloads)
		}
	}
}

func indexerUpsertChunkPayloads(t *testing.T, trace utils.MessageTrace) []map[string]any {
	t.Helper()
	payloads := make([]map[string]any, 0)
	for _, event := range trace.ToolStartEvents {
		if event.Name != "indexer_UpsertChunk" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(event.Arguments), &payload); err != nil {
			t.Fatalf("decode indexer_UpsertChunk arguments: %v\n%s", err, event.Arguments)
		}
		payloads = append(payloads, payload)
	}
	return payloads
}

func payloadMentionsFilename(payload map[string]any, filename string) bool {
	if containsText(fmt.Sprint(payload["sourceMetadata"]), filename) {
		return true
	}
	if containsText(fmt.Sprint(payload["document"]), filename) {
		return true
	}
	if containsText(fmt.Sprint(payload["provenance"]), filename) {
		return true
	}
	return false
}

func hasNonEmptyString(payload map[string]any, key string) bool {
	value, ok := payload[key].(string)
	return ok && strings.TrimSpace(value) != ""
}

func hasNonEmptyObject(payload map[string]any, key string) bool {
	value, ok := payload[key].(map[string]any)
	return ok && len(value) > 0
}

func hasNonEmptyArray(payload map[string]any, key string) bool {
	value, ok := payload[key].([]any)
	return ok && len(value) > 0
}

func hasArray(payload map[string]any, key string) bool {
	_, ok := payload[key].([]any)
	return ok
}

func hasCitationEvidence(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		if citations, ok := typed["citations"].([]any); ok && len(citations) > 0 {
			return true
		}
		for _, child := range typed {
			if hasCitationEvidence(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if hasCitationEvidence(child) {
				return true
			}
		}
	}
	return false
}

func assertAnswerContains(t *testing.T, answer string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !containsText(answer, want) {
			t.Fatalf("answer missing %q:\n%s", want, answer)
		}
	}
}

func assertAnswerContainsAny(t *testing.T, answer string, wants ...string) {
	t.Helper()
	if !containsAnyText(answer, wants...) {
		t.Fatalf("answer missing one of %v:\n%s", wants, answer)
	}
}
