package workflow

import (
	"encoding/json"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/quarkloop/pkg/plugin"
)

const maxCanonicalEmbeddingTextRunes = 2000

func (t *Tracker) FinalGuard(content string) (string, bool) {
	if t == nil {
		return "", false
	}
	if containsInternalToolMarkup(content) {
		for _, state := range t.states {
			t.emit(Event{Type: "blocked_final", WorkflowID: state.ID, SessionID: state.SessionID, Kind: state.Kind, Reason: "internal_tool_markup_in_final_response"})
		}
		details := map[string]any{
			"requirement": "return only a user-facing answer; do not render function calls or internal tool directives as text",
		}
		if !t.hasMissingSteps() {
			details["workflow_status"] = "complete"
			details["response_mode"] = "user_answer_only"
			details["functions_available"] = false
		}
		return t.blockedStatus("internal_tool_markup_in_final_response", details), true
	}
	if !t.hasMissingSteps() {
		if t.completedKnowledgeIndexResponseInvalid(content) {
			for _, state := range t.states {
				if state.Kind == KindKnowledgeIndex {
					t.emit(Event{Type: "blocked_final", WorkflowID: state.ID, SessionID: state.SessionID, Kind: state.Kind, Reason: "completed_index_status_mismatch"})
				}
			}
			return t.blockedStatus("completed_index_status_mismatch", map[string]any{
				"workflow_status": "complete",
				"response_mode":   "user_answer_only",
				"requirement":     "state that indexing completed successfully; do not describe completed extraction, embedding, indexing, or run completion as pending work",
			}), true
		}
		return "", false
	}
	for _, state := range t.states {
		t.emit(Event{Type: "blocked_final", WorkflowID: state.ID, SessionID: state.SessionID, Kind: state.Kind, Reason: "finalization_incomplete"})
	}
	return t.blockedStatus("finalization_incomplete", nil), true
}

func (t *Tracker) completedKnowledgeIndexResponseInvalid(content string) bool {
	hasCompletedIndex := false
	for _, state := range t.states {
		if state.Kind == KindKnowledgeIndex {
			hasCompletedIndex = true
			break
		}
	}
	if !hasCompletedIndex {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(content))
	acknowledgesCompletion := false
	for _, marker := range []string{"indexed", "indexing complete", "indexing is complete", "indexing has completed", "indexing completed"} {
		if strings.Contains(lower, marker) {
			acknowledgesCompletion = true
			break
		}
	}
	for _, contradiction := range []string{
		"ready to proceed",
		"continue the indexing run",
		"still need to",
		"not indexed",
		"indexing is incomplete",
		"indexing incomplete",
		"unable to complete",
	} {
		if strings.Contains(lower, contradiction) {
			return true
		}
	}
	return !acknowledgesCompletion
}

func (t *Tracker) AcceptFinalBeforeToolCalls(content string, toolCalls []plugin.ToolCall) bool {
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
	if instruction, retry := t.guardInvalidKnowledgeQueryCalls(toolCalls); retry {
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
	// Content emitted beside evidence-preparation tool calls is intermediate
	// model narration, not a final response. Keep completion-text enforcement
	// for administrative support calls at terminal stages.
	if t.callsAdvanceOrSupportCurrentStep(toolCalls) &&
		(strings.TrimSpace(content) == "" || t.callsAreKnowledgeEvidencePreparation(toolCalls)) {
		return "", false
	}
	if strings.TrimSpace(content) == "" && !t.shouldBlockNonAdvancingCalls() {
		return "", false
	}
	return t.blockAllToolCalls("non_advancing_tool_calls", map[string]any{"allowed_functions": t.missingToolNames()})
}

func (t *Tracker) callsAdvanceOrSupportCurrentStep(toolCalls []plugin.ToolCall) bool {
	if t == nil || len(toolCalls) == 0 {
		return false
	}
	for _, call := range toolCalls {
		if t.missingStepTool(call.Function.Name) {
			continue
		}
		supported := false
		for _, state := range t.states {
			current, ok := currentMissingStep(state)
			if ok && workflowSupportToolAllowed(state.Kind, current.ID, call.Function.Name) {
				supported = true
				break
			}
		}
		if !supported {
			return false
		}
	}
	return true
}

func (t *Tracker) callsAreKnowledgeEvidencePreparation(toolCalls []plugin.ToolCall) bool {
	if t == nil || len(toolCalls) == 0 {
		return false
	}
	for _, call := range toolCalls {
		matched := false
		for _, state := range t.states {
			current, ok := currentMissingStep(state)
			if ok && state.Kind == KindKnowledgeIndex && knowledgeEvidencePreparationToolAllowed(current.ID, call.Function.Name) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
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
				return t.blockAllToolCalls("tool_call_after_completion", map[string]any{"function": call.Function.Name})
			}
		}
		return "", false
	}
	for _, call := range toolCalls {
		if t.workflowRelatedTool(call.Function.Name) {
			return t.blockAllToolCalls("tool_call_after_completion", map[string]any{"function": call.Function.Name})
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
				return t.blockToolCalls(state, "missing_run_items", map[string]any{"function": call.Function.Name})
			}
		case "embed":
			for _, call := range toolCalls {
				if call.Function.Name != "gateway_Embed" {
					continue
				}
				if !embeddingInputsHaveSourceIdentity(call.Function.Arguments) {
					return t.blockToolCalls(state, "missing_embedding_source_identity", map[string]any{
						"function":       call.Function.Name,
						"required_field": "inputs[].metadata.sourceUri",
					})
				}
				if !embeddingInputsAreBoundedChunks(call.Function.Arguments) {
					return t.blockToolCalls(state, "unbounded_embedding_content", map[string]any{
						"function": call.Function.Name,
						"requirement": "embed one bounded canonical text passage or one pageRef per indexed chunk; " +
							"do not embed a whole-document contentRef or combine an entire document from pageRefs",
						"batching": "for multiple sources use one input per source, with exactly one bounded passage or one pageRef in each input",
					})
				}
				if !pdfEmbeddingInputsUsePageReferences(call.Function.Arguments) {
					return t.blockToolCalls(state, "pdf_embedding_requires_page_reference", map[string]any{
						"function":    call.Function.Name,
						"requirement": "for PDF sources, embed one extracted pageRef and persist that same pageRef as textContentRef; do not copy PDF text into embedding input",
					})
				}
			}
		case "index":
			for _, call := range toolCalls {
				if call.Function.Name != "indexer_UpsertChunk" {
					continue
				}
				if missing := canonicalUpsertChunkMissingFields(call.Function.Arguments); len(missing) > 0 {
					return t.blockToolCalls(state, "incomplete_canonical_record", map[string]any{"function": call.Function.Name, "missing_fields": missing})
				}
			}
		}
		if instruction, blocked := t.guardRepeatedOrUnprovenSourceWork(state, current.ID, toolCalls); blocked {
			return instruction, true
		}
	}
	return "", false
}

func (t *Tracker) guardInvalidKnowledgeQueryCalls(toolCalls []plugin.ToolCall) (string, bool) {
	for _, state := range t.states {
		if state.Kind != KindKnowledgeQuery {
			continue
		}
		current, ok := currentMissingStep(state)
		if !ok || current.ID != "embed-query" {
			continue
		}
		for _, call := range toolCalls {
			if call.Function.Name != "gateway_Embed" || queryEmbeddingUsesSingleInlineText(call.Function.Arguments) {
				continue
			}
			return t.blockToolCalls(state, "query_embedding_requires_inline_text", map[string]any{
				"function": call.Function.Name,
				"requirement": "embed exactly one non-empty inline text retrieval query that represents the current request; " +
					"do not use runtime content, page, image, or shorthand references, or split one question into multiple embeddings",
			})
		}
	}
	return "", false
}

func queryEmbeddingUsesSingleInlineText(arguments string) bool {
	var raw map[string]json.RawMessage
	if json.Unmarshal([]byte(strings.TrimSpace(arguments)), &raw) != nil {
		return false
	}
	for _, key := range []string{"inputRef", "contentRef", "pageRef", "pageRefs", "imageRef"} {
		value, exists := raw[key]
		if exists && string(value) != "null" {
			return false
		}
	}
	var inputs []struct {
		Content []struct {
			Kind string `json:"kind"`
			Text string `json:"text"`
			Ref  string `json:"ref"`
		} `json:"content"`
	}
	if json.Unmarshal(raw["inputs"], &inputs) != nil || len(inputs) != 1 || len(inputs[0].Content) != 1 {
		return false
	}
	content := inputs[0].Content[0]
	return strings.TrimSpace(content.Kind) == "CONTENT_KIND_TEXT" &&
		strings.TrimSpace(content.Text) != "" &&
		strings.TrimSpace(content.Ref) == ""
}

func containsInternalToolMarkup(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "<longcat_tool_call>") ||
		strings.Contains(lower, "<tool_call>") ||
		strings.Contains(lower, "</tool_call>")
}

func embeddingInputsHaveSourceIdentity(arguments string) bool {
	var payload struct {
		Inputs []struct {
			Metadata map[string]string `json:"metadata"`
		} `json:"inputs"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(arguments)), &payload) != nil || len(payload.Inputs) == 0 {
		return false
	}
	for _, input := range payload.Inputs {
		if strings.TrimSpace(firstNonEmptyMetadataValue(input.Metadata, "sourceUri", "source_uri")) == "" {
			return false
		}
	}
	return true
}

func embeddingInputsAreBoundedChunks(arguments string) bool {
	var payload struct {
		Inputs []struct {
			Content []struct {
				Kind string `json:"kind"`
				Ref  string `json:"ref"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"inputs"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(arguments)), &payload) != nil || len(payload.Inputs) == 0 {
		return false
	}
	for _, input := range payload.Inputs {
		if len(input.Content) == 0 {
			return false
		}
		textRunes := 0
		pageRefs := 0
		for _, content := range input.Content {
			switch strings.TrimSpace(content.Kind) {
			case "CONTENT_KIND_TEXT":
				if strings.TrimSpace(content.Text) == "" {
					return false
				}
				textRunes += len([]rune(content.Text))
			case "CONTENT_KIND_PAGE_REF":
				if strings.TrimSpace(content.Ref) == "" {
					return false
				}
				pageRefs++
			default:
				return false
			}
		}
		if textRunes > maxCanonicalEmbeddingTextRunes || pageRefs > 1 || (pageRefs > 0 && textRunes > 0) {
			return false
		}
	}
	return true
}

func pdfEmbeddingInputsUsePageReferences(arguments string) bool {
	var payload struct {
		Inputs []struct {
			Content []struct {
				Kind string `json:"kind"`
				Ref  string `json:"ref"`
			} `json:"content"`
			Metadata map[string]string `json:"metadata"`
		} `json:"inputs"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(arguments)), &payload) != nil || len(payload.Inputs) == 0 {
		return false
	}
	for _, input := range payload.Inputs {
		source := firstNonEmptyMetadataValue(input.Metadata, "sourceUri", "source_uri")
		if !isPDFSourceURI(source) {
			continue
		}
		if len(input.Content) != 1 ||
			strings.TrimSpace(input.Content[0].Kind) != "CONTENT_KIND_PAGE_REF" ||
			strings.TrimSpace(input.Content[0].Ref) == "" {
			return false
		}
	}
	return true
}

func isPDFSourceURI(source string) bool {
	source = strings.TrimSpace(source)
	if source == "" {
		return false
	}
	path := source
	if parsed, err := url.Parse(source); err == nil && parsed.Path != "" {
		path = parsed.Path
	}
	return strings.EqualFold(filepath.Ext(path), ".pdf")
}

func firstNonEmptyMetadataValue(metadata map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(metadata[key]); value != "" {
			return value
		}
	}
	return ""
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
	if canonicalUpsertSourceURI(arguments) == "" {
		missing = append(missing, "sourceUri in document, sourceMetadata, or provenance")
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

func canonicalUpsertSourceURI(arguments string) string {
	var payload any
	if json.Unmarshal([]byte(arguments), &payload) != nil {
		return ""
	}
	return firstStringByKeys(payload, "sourceUri", "source_uri")
}

func (t *Tracker) guardRepeatedOrUnprovenSourceWork(state State, stepID string, toolCalls []plugin.ToolCall) (string, bool) {
	if stepID != "extract" && stepID != "embed" && stepID != "index" {
		return "", false
	}
	seen := make(map[string]struct{})
	for _, call := range toolCalls {
		var identities []string
		switch {
		case stepID == "extract" && (call.Function.Name == "document_ExtractText" || call.Function.Name == "document_GetPages"):
			identities = []string{workflowCompletionIdentity(stepID, call.Function.Name, call.Function.Arguments)}
		case stepID == "embed" && knowledgeEvidencePreparationToolAllowed(stepID, call.Function.Name):
			identity := evidencePreparationSourceIdentity(call.Function.Arguments)
			if identity == "" || !t.hasObserved(state.ID, "extract", identity) {
				return t.blockToolCalls(state, "unextracted_evidence_refinement_source", map[string]any{
					"function":    call.Function.Name,
					"requirement": "refine evidence only for a source already returned by successful document extraction",
				})
			}
			if _, duplicate := seen[identity]; duplicate || t.hasObserved(state.ID, "embed-evidence", identity) {
				return t.blockToolCalls(state, "duplicate_evidence_refinement", map[string]any{
					"function": call.Function.Name, "source_identity": identity,
					"requirement": "use the bounded evidence already returned for this source, or embed it; do not repeat page refinement",
				})
			}
			seen[identity] = struct{}{}
			continue
		case stepID == "embed" && call.Function.Name == "gateway_Embed":
			identities = embeddingInputIdentities(call.Function.Arguments)
		case stepID == "index" && call.Function.Name == "indexer_UpsertChunk":
			identities = []string{workflowCompletionIdentity(stepID, call.Function.Name, call.Function.Arguments)}
		default:
			continue
		}
		for _, identity := range identities {
			if identity == "" {
				continue
			}
			if _, duplicate := seen[identity]; duplicate || t.hasObserved(state.ID, stepID, identity) {
				return t.blockToolCalls(state, "duplicate_source_operation", map[string]any{
					"function": call.Function.Name, "source_identity": identity,
					"requirement": "advance the remaining unprocessed source rather than repeating completed source work",
				})
			}
			seen[identity] = struct{}{}
			switch stepID {
			case "embed":
				sourceURI := strings.TrimPrefix(identity, "source:")
				if identity == sourceURI || !t.hasObserved(state.ID, "extract", sourceURI) {
					return t.blockToolCalls(state, "unextracted_embedding_source", map[string]any{
						"function": call.Function.Name, "source_identity": identity,
						"requirement": "embed only source evidence returned by successful document extraction",
					})
				}
			case "index":
				if !t.hasObserved(state.ID, "embed", identity) {
					return t.blockToolCalls(state, "unembedded_index_source", map[string]any{
						"function": call.Function.Name, "source_identity": identity,
						"requirement": "persist only a source whose matching canonical chunk embedding succeeded",
					})
				}
			}
		}
	}
	return "", false
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
		if reason, details, blocked := t.orderedBatchGuardDecision(state, toolCalls); blocked {
			return t.blockToolCalls(state, reason, details)
		}
	}
	return "", false
}

func (t *Tracker) orderedBatchGuardDecision(state State, toolCalls []plugin.ToolCall) (string, map[string]any, bool) {
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
		if simulated.Kind == KindKnowledgeQuery {
			if step := completedStepSatisfiedBy(simulated, name); step.ID != "" {
				return "duplicate_completed_step_call", map[string]any{
					"function":       name,
					"completed_step": step.Label,
					"requirement":    "the completed operation already satisfies this step; advance to the current step instead of repeating it",
				}, true
			}
		}
		if step := futureMissingStepSatisfiedBy(simulated, name); step.ID != "" {
			return "out_of_order_tool_call", map[string]any{
				"function": name, "current_step": current.Label, "future_step": step.Label,
				"allowed_functions": append([]string(nil), current.AnyOf...),
			}, true
		}
		if simulated.Kind == KindKnowledgeIndex && (current.ID != "ingest-start" || knowledgeIndexServiceFunction(name)) {
			details := map[string]any{
				"function": name, "current_step": current.Label,
				"allowed_functions": append([]string(nil), current.AnyOf...),
			}
			if current.ID == "index" {
				details["requirement"] = "persist each prepared canonical chunk using the bounded text or page reference already embedded for its source; do not reread source content"
			}
			return "tool_call_not_in_current_step", details, true
		}
	}
	return "", nil, false
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

func completedStepSatisfiedBy(state State, tool string) Step {
	for _, step := range state.Steps {
		if stepComplete(step) && stepSatisfiedBy(step, tool) {
			return step
		}
	}
	return Step{}
}

func workflowSupportToolAllowed(kind Kind, currentStepID, tool string) bool {
	switch kind {
	case KindKnowledgeIndex:
		switch tool {
		case "io_List", "io_Stat":
			if currentStepID == "ingest-start" {
				return true
			}
		}
		if knowledgeEvidencePreparationToolAllowed(currentStepID, tool) {
			return true
		}
		if strings.HasPrefix(tool, "runstate_") && tool != "runstate_StartRun" && tool != "runstate_MarkComplete" {
			return true
		}
	}
	return false
}

func knowledgeEvidencePreparationToolAllowed(currentStepID, tool string) bool {
	switch currentStepID {
	case "extract", "embed":
		switch tool {
		case "document_GetPages", "document_ExtractLayout", "document_ExtractTables", "document_ExtractImages", "document_RunOCR":
			return true
		}
	}
	return false
}

func evidencePreparationSourceIdentity(arguments string) string {
	return workflowCompletionIdentity("extract", "document_GetPages", arguments)
}

func knowledgeIndexServiceFunction(name string) bool {
	for _, prefix := range []string{"citation_", "core_", "document_", "indexer_", "runstate_", "gateway_"} {
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

func (t *Tracker) blockToolCalls(state State, reason string, details map[string]any) (string, bool) {
	t.emit(Event{
		Type:       "blocked_tool_calls",
		WorkflowID: state.ID,
		SessionID:  state.SessionID,
		Kind:       state.Kind,
		Reason:     reason,
		Details:    cloneEventDetails(details),
	})
	return t.blockedStatus(reason, details), true
}

func (t *Tracker) blockAllToolCalls(reason string, details map[string]any) (string, bool) {
	for _, state := range t.states {
		t.emit(Event{
			Type:       "blocked_tool_calls",
			WorkflowID: state.ID,
			SessionID:  state.SessionID,
			Kind:       state.Kind,
			Reason:     reason,
			Details:    cloneEventDetails(details),
		})
	}
	return t.blockedStatus(reason, details), true
}

func cloneEventDetails(details map[string]any) map[string]any {
	if len(details) == 0 {
		return nil
	}
	out := make(map[string]any, len(details))
	for key, value := range details {
		switch typed := value.(type) {
		case []string:
			out[key] = append([]string(nil), typed...)
		default:
			out[key] = typed
		}
	}
	return out
}
