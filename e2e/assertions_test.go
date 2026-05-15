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
	)
	return replacer.Replace(value)
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

func assertAgentStructuredPDFIndexPayloads(t *testing.T, trace utils.MessageTrace, documents []indexedPDFDocument) {
	t.Helper()
	payloads := indexerIndexDocumentPayloads(t, trace)
	if len(payloads) < len(documents) {
		t.Fatalf("indexer_IndexDocument payload count = %d, want at least %d", len(payloads), len(documents))
	}

	seen := make(map[string]bool, len(documents))
	for i, payload := range payloads {
		if !hasNonEmptyString(payload, "chunkId") {
			t.Fatalf("IndexDocument payload %d missing chunkId: %+v", i, payload)
		}
		if !hasNonEmptyString(payload, "textContent") {
			t.Fatalf("IndexDocument payload %d missing textContent: %+v", i, payload)
		}
		if !hasNonEmptyString(payload, "embeddingRef") {
			t.Fatalf("IndexDocument payload %d missing embeddingRef from embedding_Embed: %+v", i, payload)
		}
		if !hasNonEmptyObject(payload, "document") {
			t.Fatalf("IndexDocument payload %d missing first-class document metadata: %+v", i, payload)
		}
		if !hasNonEmptyObject(payload, "sourceMetadata") {
			t.Fatalf("IndexDocument payload %d missing sourceMetadata: %+v", i, payload)
		}
		if !hasNonEmptyArray(payload, "facts") {
			t.Fatalf("IndexDocument payload %d missing agent-produced facts: %+v", i, payload)
		}
		if !hasNonEmptyArray(payload, "entities") {
			t.Fatalf("IndexDocument payload %d missing agent-produced entities: %+v", i, payload)
		}
		if !hasNonEmptyArray(payload, "relations") {
			t.Fatalf("IndexDocument payload %d missing agent-produced relations: %+v", i, payload)
		}
		if !hasNonEmptyArray(payload, "citations") {
			t.Fatalf("IndexDocument payload %d missing source citations: %+v", i, payload)
		}
		if !hasNonEmptyObject(payload, "provenance") {
			t.Fatalf("IndexDocument payload %d missing provenance: %+v", i, payload)
		}
		for _, document := range documents {
			if payloadMentionsFilename(payload, document.Filename) {
				seen[document.Filename] = true
			}
		}
	}
	for _, document := range documents {
		if !seen[document.Filename] {
			t.Fatalf("no structured IndexDocument payload referenced %s: %+v", document.Filename, payloads)
		}
	}
}

func indexerIndexDocumentPayloads(t *testing.T, trace utils.MessageTrace) []map[string]any {
	t.Helper()
	payloads := make([]map[string]any, 0)
	for _, event := range trace.ToolStartEvents {
		if event.Name != "indexer_IndexDocument" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(event.Arguments), &payload); err != nil {
			t.Fatalf("decode indexer_IndexDocument arguments: %v\n%s", err, event.Arguments)
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
