package workflow

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/quarkloop/pkg/plugin"
)

func (t *Tracker) FinalGuard(string) (string, bool) {
	if t == nil {
		return "", false
	}
	if !t.hasMissingSteps() {
		return "", false
	}
	for _, state := range t.states {
		t.emit(Event{Type: "blocked_final", WorkflowID: state.ID, SessionID: state.SessionID, Kind: state.Kind})
	}
	return t.blockedStatus("finalization_incomplete", nil), true
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
	if !t.hasMissingSteps() {
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
	return t.blockedStatus("non_advancing_tool_calls", map[string]any{"allowed_functions": t.missingToolNames()}), true
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
	if t == nil || strings.TrimSpace(content) == "" || len(toolCalls) == 0 || t.hasMissingSteps() {
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

func (t *Tracker) hasMissingSteps() bool {
	for _, state := range t.states {
		for _, step := range state.Steps {
			if !stepComplete(step) {
				return true
			}
		}
	}
	return false
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
				return t.blockedStatus("tool_call_after_completion", map[string]any{"function": call.Function.Name}), true
			}
		}
		return "", false
	}
	for _, call := range toolCalls {
		if t.workflowRelatedTool(call.Function.Name) {
			return t.blockedStatus("tool_call_after_completion", map[string]any{"function": call.Function.Name}), true
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
				return t.blockedStatus("missing_run_items", map[string]any{"function": call.Function.Name}), true
			}
		case "index":
			for _, call := range toolCalls {
				if call.Function.Name != "indexer_UpsertChunk" {
					continue
				}
				if missing := canonicalUpsertChunkMissingFields(call.Function.Arguments); len(missing) > 0 {
					t.emit(Event{Type: "blocked_tool_calls", WorkflowID: state.ID, SessionID: state.SessionID, Kind: state.Kind})
					return t.blockedStatus("incomplete_canonical_record", map[string]any{"function": call.Function.Name, "missing_fields": missing}), true
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
		if instruction, blocked := t.orderedBatchGuardInstruction(state, toolCalls); blocked {
			t.emit(Event{Type: "blocked_tool_calls", WorkflowID: state.ID, SessionID: state.SessionID, Kind: state.Kind})
			return instruction, true
		}
	}
	return "", false
}

func (t *Tracker) orderedBatchGuardInstruction(state State, toolCalls []plugin.ToolCall) (string, bool) {
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
			return t.blockedStatus("out_of_order_tool_call", map[string]any{
				"function": name, "current_step": current.Label, "future_step": step.Label,
				"allowed_functions": append([]string(nil), current.AnyOf...),
			}), true
		}
		if simulated.Kind == KindKnowledgeIndex && (current.ID != "ingest-start" || knowledgeIndexServiceFunction(name)) {
			return t.blockedStatus("tool_call_not_in_current_step", map[string]any{
				"function": name, "current_step": current.Label,
				"allowed_functions": append([]string(nil), current.AnyOf...),
			}), true
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

func (t *Tracker) blockedStatus(reason string, details map[string]any) string {
	fact := map[string]any{
		"type":   "runtime.workflow.validation",
		"status": "blocked",
		"reason": reason,
	}
	if status := t.ContinuationStatus(); status != "" {
		var workflow any
		if json.Unmarshal([]byte(status), &workflow) == nil {
			fact["workflow"] = workflow
		}
	}
	for key, value := range details {
		fact[key] = value
	}
	data, err := json.Marshal(fact)
	if err != nil {
		return `{"type":"runtime.workflow.validation","status":"blocked"}`
	}
	return string(data)
}
