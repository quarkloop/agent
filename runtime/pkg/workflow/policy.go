package workflow

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/quarkloop/pkg/plugin"
)

func (t *Tracker) FinalGuard(string) (string, bool) {
	if t == nil {
		return "", false
	}
	missing := t.missing()
	if len(missing) == 0 {
		return "", false
	}
	for _, state := range t.states {
		t.emit(Event{Type: "blocked_final", WorkflowID: state.ID, SessionID: state.SessionID, Kind: state.Kind})
	}
	if instruction := t.ContinuationPrompt(); instruction != "" {
		return "Runtime workflow validation blocked finalization.\n\n" + instruction, true
	}
	return "Runtime workflow validation blocked finalization. " + strings.Join(missing, " ") + " Continue using the existing service results and complete the missing service-backed workflow steps before producing the final answer. When multiple sources or remaining workflow steps are known, issue the remaining service calls as an ordered multi-call batch in one assistant turn.", true
}

func (t *Tracker) AcceptFinalBeforeToolCalls(content string, toolCalls []plugin.ToolCall) bool {
	return t.acceptFinalWithCompletedStepCalls(content, toolCalls)
}

func (t *Tracker) AcceptFinalAfterToolCalls(content string, toolCalls []plugin.ToolCall) bool {
	return t.acceptFinalWithCompletedStepCalls(content, toolCalls)
}

func (t *Tracker) GuardToolCalls(content string, toolCalls []plugin.ToolCall) (string, bool) {
	if t == nil || len(toolCalls) == 0 {
		return "", false
	}
	if len(t.missing()) == 0 {
		return t.guardCallsAfterWorkflowCompletion(content, toolCalls)
	}
	if instruction, retry := t.guardInvalidKnowledgeCalls(toolCalls); retry {
		return instruction, retry
	}
	if instruction, retry := t.guardOutOfOrderCalls(toolCalls); retry {
		return instruction, retry
	}
	for _, call := range toolCalls {
		if t.missingStepTool(call.Function.Name) {
			return "", false
		}
	}
	if strings.TrimSpace(content) == "" && !t.shouldBlockNonAdvancingCalls() {
		return "", false
	}
	for _, state := range t.states {
		t.emit(Event{Type: "blocked_tool_calls", WorkflowID: state.ID, SessionID: state.SessionID, Kind: state.Kind})
	}
	return "Runtime workflow validation blocked the proposed tool calls because the assistant was already drafting a final answer while required service-backed workflow steps are still incomplete. Continue from the existing service results and call one of the missing workflow service functions next: " + strings.Join(t.missingToolNames(), ", ") + ".", true
}

func (t *Tracker) completedStepTool(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, state := range t.states {
		for _, step := range state.Steps {
			if stepComplete(step) && stepSatisfiedBy(step, name) {
				return true
			}
		}
	}
	return false
}

func (t *Tracker) acceptFinalWithCompletedStepCalls(content string, toolCalls []plugin.ToolCall) bool {
	if t == nil || strings.TrimSpace(content) == "" || len(toolCalls) == 0 || len(t.missing()) > 0 {
		return false
	}
	for _, call := range toolCalls {
		if !t.completedStepTool(call.Function.Name) {
			return false
		}
	}
	return true
}

func (t *Tracker) missingStepTool(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, state := range t.states {
		for _, step := range state.Steps {
			if !stepComplete(step) && stepSatisfiedBy(step, name) {
				return true
			}
		}
	}
	return false
}

func (t *Tracker) missing() []string {
	missing := make([]string, 0)
	for _, state := range t.states {
		for _, step := range state.Steps {
			if stepComplete(step) {
				continue
			}
			count := ""
			if requiredCount(step) > 1 {
				count = fmt.Sprintf(" (%d of %d complete)", step.CompletedCount, requiredCount(step))
			}
			missing = append(missing, fmt.Sprintf("%s workflow still needs %s%s using one of: %s.", state.Kind, step.Label, count, strings.Join(step.AnyOf, ", ")))
		}
	}
	sort.Strings(missing)
	return missing
}

func (t *Tracker) missingToolNames() []string {
	seen := make(map[string]struct{})
	names := make([]string, 0)
	for _, state := range t.states {
		for _, step := range state.Steps {
			if stepComplete(step) {
				continue
			}
			for _, name := range step.AnyOf {
				if _, ok := seen[name]; ok {
					continue
				}
				seen[name] = struct{}{}
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	return names
}

func (t *Tracker) guardCallsAfterWorkflowCompletion(content string, toolCalls []plugin.ToolCall) (string, bool) {
	if t == nil || len(toolCalls) == 0 {
		return "", false
	}
	if strings.TrimSpace(content) != "" {
		for _, call := range toolCalls {
			if t.workflowRelatedTool(call.Function.Name) && !t.completedStepTool(call.Function.Name) {
				return "Runtime workflow validation blocked extra service calls because the required workflow is already complete. Answer now from the existing service evidence instead of starting additional workflow service functions.", true
			}
		}
		return "", false
	}
	for _, call := range toolCalls {
		if t.workflowRelatedTool(call.Function.Name) {
			return "Runtime workflow validation blocked extra service calls because the required workflow is already complete. Answer now from the existing service evidence instead of starting additional workflow service functions.", true
		}
	}
	return "", false
}

func (t *Tracker) workflowRelatedTool(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, state := range t.states {
		switch state.Kind {
		case KindKnowledgeIndex, KindKnowledgeQuery:
			if knowledgeIndexServiceFunction(name) {
				return true
			}
		case KindDevOps:
			if devopsServiceFunction(name) {
				return true
			}
		case KindSystemInspect, KindSystemMutation:
			if strings.HasPrefix(name, "system_") {
				return true
			}
		}
	}
	return false
}

func (t *Tracker) shouldBlockNonAdvancingCalls() bool {
	if t == nil {
		return false
	}
	for _, state := range t.states {
		if state.Kind != KindKnowledgeIndex {
			continue
		}
		for _, step := range state.Steps {
			if stepComplete(step) {
				continue
			}
			return step.ID == "index" || step.ID == "ingest-complete"
		}
	}
	return false
}

func (t *Tracker) guardInvalidKnowledgeCalls(toolCalls []plugin.ToolCall) (string, bool) {
	for _, state := range t.states {
		if state.Kind != KindKnowledgeIndex {
			continue
		}
		current, ok := currentMissingStep(state)
		if !ok {
			continue
		}
		switch current.ID {
		case "ingest-start":
			for _, call := range toolCalls {
				if call.Function.Name != "runstate_StartRun" || startRunHasItems(call.Function.Arguments) {
					continue
				}
				t.emit(Event{Type: "blocked_tool_calls", WorkflowID: state.ID, SessionID: state.SessionID, Kind: state.Kind})
				return "Runtime workflow validation blocked runstate_StartRun because it did not include any document items. List or inspect the directory if needed, then start the durable run with every discovered document item before continuing.", true
			}
		case "index":
			for _, call := range toolCalls {
				if call.Function.Name != "indexer_UpsertChunk" {
					continue
				}
				if missing := canonicalUpsertChunkMissingFields(call.Function.Arguments); len(missing) > 0 {
					t.emit(Event{Type: "blocked_tool_calls", WorkflowID: state.ID, SessionID: state.SessionID, Kind: state.Kind})
					return "Runtime workflow validation blocked indexer_UpsertChunk because the canonical knowledge record is incomplete. Retry the upsert with these fields populated from the extracted source evidence: " + strings.Join(missing, ", ") + ".", true
				}
			}
		}
	}
	return "", false
}

func canonicalUpsertChunkMissingFields(arguments string) []string {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(arguments), &payload); err != nil {
		return []string{"valid JSON arguments"}
	}
	missing := make([]string, 0)
	if !rawStringField(payload, "chunkId") {
		missing = append(missing, "chunkId")
	}
	if !rawStringField(payload, "embeddingRef") {
		missing = append(missing, "embeddingRef")
	}
	if !rawStringField(payload, "textContentRef") && !rawStringField(payload, "textContent") {
		missing = append(missing, "textContentRef or textContent")
	}
	if !rawObjectField(payload, "document") {
		missing = append(missing, "document")
	}
	if !rawObjectField(payload, "sourceMetadata") {
		missing = append(missing, "sourceMetadata")
	}
	if !rawObjectField(payload, "provenance") {
		missing = append(missing, "provenance")
	}
	if !rawNonEmptyArrayField(payload, "facts") {
		missing = append(missing, "non-empty facts")
	}
	if !rawNonEmptyArrayField(payload, "entities") {
		missing = append(missing, "non-empty entities")
	}
	if !rawArrayField(payload, "relations") {
		missing = append(missing, "relations array")
	}
	if !rawNonEmptyArrayField(payload, "citations") {
		missing = append(missing, "non-empty citations")
	}
	return missing
}

func rawStringField(payload map[string]json.RawMessage, key string) bool {
	raw, ok := payload[key]
	if !ok {
		return false
	}
	var value string
	return json.Unmarshal(raw, &value) == nil && strings.TrimSpace(value) != ""
}

func rawObjectField(payload map[string]json.RawMessage, key string) bool {
	raw, ok := payload[key]
	if !ok {
		return false
	}
	var value map[string]json.RawMessage
	return json.Unmarshal(raw, &value) == nil && len(value) > 0
}

func rawArrayField(payload map[string]json.RawMessage, key string) bool {
	raw, ok := payload[key]
	if !ok {
		return false
	}
	var value []json.RawMessage
	return json.Unmarshal(raw, &value) == nil
}

func rawNonEmptyArrayField(payload map[string]json.RawMessage, key string) bool {
	raw, ok := payload[key]
	if !ok {
		return false
	}
	var value []json.RawMessage
	return json.Unmarshal(raw, &value) == nil && len(value) > 0
}

func (t *Tracker) guardOutOfOrderCalls(toolCalls []plugin.ToolCall) (string, bool) {
	for _, state := range t.states {
		if stateComplete(state) {
			continue
		}
		if instruction, blocked := orderedBatchGuardInstruction(state, toolCalls); blocked {
			t.emit(Event{Type: "blocked_tool_calls", WorkflowID: state.ID, SessionID: state.SessionID, Kind: state.Kind})
			return instruction, true
		}
	}
	return "", false
}

func orderedBatchGuardInstruction(state State, toolCalls []plugin.ToolCall) (string, bool) {
	simulated := cloneState(state)
	for _, call := range toolCalls {
		name := strings.TrimSpace(call.Function.Name)
		if name == "" {
			continue
		}
		current, ok := currentMissingStep(simulated)
		if !ok {
			continue
		}
		if stepSatisfiedBy(current, name) {
			for i := range simulated.Steps {
				if simulated.Steps[i].ID == current.ID {
					simulated.Steps[i].CompletedCount++
					break
				}
			}
			continue
		}
		if workflowSupportToolAllowed(simulated.Kind, current.ID, name) {
			continue
		}
		if step := futureMissingStepSatisfiedBy(simulated, name); step.ID != "" {
			return fmt.Sprintf("Runtime workflow validation blocked %s because the %s workflow must finish %s before %s. Continue with an ordered service-call batch beginning with one of the current required service functions: %s.", name, simulated.Kind, current.Label, step.Label, strings.Join(current.AnyOf, ", ")), true
		}
		if simulated.Kind == KindKnowledgeIndex && (current.ID != "ingest-start" || knowledgeIndexServiceFunction(name)) {
			return fmt.Sprintf("Runtime workflow validation blocked %s because it is not part of the current canonical Knowledge indexing step. Finish %s using one of: %s.", name, current.Label, strings.Join(current.AnyOf, ", ")), true
		}
	}
	return "", false
}

func currentMissingStep(state State) (Step, bool) {
	for _, step := range state.Steps {
		if !stepComplete(step) {
			return step, true
		}
	}
	return Step{}, false
}

func futureMissingStepSatisfiedBy(state State, tool string) Step {
	seenCurrent := false
	for _, step := range state.Steps {
		if stepComplete(step) {
			continue
		}
		if !seenCurrent {
			seenCurrent = true
			continue
		}
		if stepSatisfiedBy(step, tool) {
			return step
		}
	}
	return Step{}
}

func workflowSupportToolAllowed(kind Kind, currentStepID, tool string) bool {
	switch kind {
	case KindKnowledgeIndex:
		switch tool {
		case "io_Read", "io_List", "io_Stat":
			if currentStepID == "ingest-start" {
				return true
			}
		}
		if strings.HasPrefix(tool, "runstate_") && tool != "runstate_StartRun" && tool != "runstate_MarkComplete" {
			return true
		}
	}
	return false
}

func knowledgeIndexServiceFunction(name string) bool {
	for _, prefix := range []string{"citation_", "core_", "document_", "embedding_", "indexer_", "runstate_", "gateway_"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func devopsServiceFunction(name string) bool {
	for _, prefix := range []string{"repo_", "build_", "test_", "policy_"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func startRunHasItems(arguments string) bool {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(arguments)), &payload); err != nil {
		return true
	}
	raw, ok := payload["items"]
	if !ok {
		return false
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return false
	}
	return len(items) > 0
}
