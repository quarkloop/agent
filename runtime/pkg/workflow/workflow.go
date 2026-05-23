// Package workflow tracks runtime-owned multi-step service workflows.
package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/quarkloop/pkg/plugin"
)

type Kind string

const (
	KindKnowledgeIndex  Kind = "knowledge.index"
	KindKnowledgeQuery  Kind = "knowledge.query"
	KindDevOps          Kind = "devops"
	KindSystemInspect   Kind = "system.inspect"
	KindSystemMutation  Kind = "system.mutation"
	KindApprovalRequest Kind = "approval.request"
)

type Step struct {
	ID             string   `json:"id"`
	Label          string   `json:"label"`
	AnyOf          []string `json:"any_of"`
	RequiredCount  int      `json:"required_count,omitempty"`
	CompletedCount int      `json:"completed_count,omitempty"`
	CompletedBy    string   `json:"completed_by,omitempty"`
	CompletedAt    string   `json:"completed_at,omitempty"`
}

type State struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Kind      Kind      `json:"kind"`
	Status    string    `json:"status"`
	Prompt    string    `json:"prompt,omitempty"`
	RunID     string    `json:"run_id,omitempty"`
	Steps     []Step    `json:"steps"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Event struct {
	Type       string `json:"type"`
	WorkflowID string `json:"workflow_id"`
	SessionID  string `json:"session_id"`
	Kind       Kind   `json:"kind"`
	StepID     string `json:"step_id,omitempty"`
	Tool       string `json:"tool,omitempty"`
}

type Store struct {
	mu     sync.RWMutex
	next   uint64
	states map[string][]State
}

func NewStore() *Store {
	return &Store{states: make(map[string][]State)}
}

func (s *Store) Begin(sessionID, prompt string, intents []Intent) []State {
	if s == nil || len(intents) == 0 {
		return nil
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]State, 0, len(intents))
	for _, intent := range intents {
		s.next++
		state := State{
			ID:        fmt.Sprintf("workflow-%d", s.next),
			SessionID: sessionID,
			Kind:      intent.Kind,
			Status:    "active",
			Prompt:    prompt,
			Steps:     cloneSteps(intent.Steps),
			CreatedAt: now,
			UpdatedAt: now,
		}
		s.states[sessionID] = append(s.states[sessionID], state)
		out = append(out, cloneState(state))
	}
	return out
}

func (s *Store) CompleteStep(sessionID, workflowID, stepID, tool string) (State, bool) {
	if s == nil {
		return State{}, false
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.states[sessionID] {
		state := &s.states[sessionID][i]
		if state.ID != workflowID {
			continue
		}
		for j := range state.Steps {
			step := &state.Steps[j]
			if step.ID != stepID || stepComplete(*step) {
				continue
			}
			step.CompletedCount++
			step.CompletedBy = tool
			step.CompletedAt = now.Format(time.RFC3339)
			state.UpdatedAt = now
			if stateComplete(*state) {
				state.Status = "complete"
			}
			return cloneState(*state), true
		}
	}
	return State{}, false
}

func (s *Store) ReplaceState(state State) bool {
	if s == nil || state.SessionID == "" || state.ID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.states[state.SessionID] {
		if s.states[state.SessionID][i].ID != state.ID {
			continue
		}
		state.UpdatedAt = time.Now().UTC()
		if stateComplete(state) {
			state.Status = "complete"
		} else if state.Status == "" || state.Status == "complete" {
			state.Status = "active"
		}
		s.states[state.SessionID][i] = cloneState(state)
		return true
	}
	return false
}

func (s *Store) List(sessionID string) []State {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	states := s.states[sessionID]
	out := make([]State, 0, len(states))
	for _, state := range states {
		out = append(out, cloneState(state))
	}
	return out
}

type Intent struct {
	Kind  Kind
	Steps []Step
}

type Tracker struct {
	store    *Store
	states   []State
	observer func(Event)
}

func NewTracker(sessionID, prompt string, tools []plugin.ToolSchema, store *Store, observer func(Event)) *Tracker {
	intents := Detect(prompt, tools)
	if len(intents) == 0 {
		return nil
	}
	if store == nil {
		store = NewStore()
	}
	states := store.Begin(sessionID, prompt, intents)
	tracker := &Tracker{store: store, states: states, observer: observer}
	for _, state := range states {
		tracker.emit(Event{Type: "detected", WorkflowID: state.ID, SessionID: state.SessionID, Kind: state.Kind})
	}
	return tracker
}

func (t *Tracker) WrapToolHandler(next plugin.ToolHandler) plugin.ToolHandler {
	if t == nil {
		return next
	}
	return func(ctx context.Context, name, arguments string) (string, error) {
		result, err := next(ctx, name, arguments)
		if toolResultSucceeded(result, err) {
			t.RecordToolResult(name, result)
		}
		return result, err
	}
}

func (t *Tracker) RecordToolResult(name, result string) {
	if t == nil || t.store == nil {
		return
	}
	if name == "ingestion_StartRun" {
		t.applyObservedIngestionRun(observedSourceCount(result), observedIngestionRunID(result))
	}
	for i := range t.states {
		state := t.states[i]
		step, ok := currentMissingStep(state)
		if !ok || !stepSatisfiedBy(step, name) {
			continue
		}
		updated, ok := t.store.CompleteStep(state.SessionID, state.ID, step.ID, name)
		if !ok {
			continue
		}
		t.states[i] = updated
		t.emit(Event{Type: "step_completed", WorkflowID: state.ID, SessionID: state.SessionID, Kind: state.Kind, StepID: step.ID, Tool: name})
	}
}

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

func (t *Tracker) PromptBlock() string {
	if t == nil || len(t.states) == 0 {
		return ""
	}
	sections := make([]string, 0, len(t.states)+1)
	sections = append(sections, "Active Runtime Workflow\nThe runtime detected one or more service-backed workflows for this user request. Complete the listed service-backed steps before finalizing. Use natural reasoning for the user-facing answer, but use the available service functions for durable work.")
	for _, state := range t.states {
		sections = append(sections, workflowPromptBlock(state))
	}
	return strings.Join(sections, "\n\n")
}

// ContinuationPrompt returns a focused system instruction for the next model
// turn after a service call. It describes only the current required step so the
// model does not drift into final prose or future workflow operations.
func (t *Tracker) ContinuationPrompt() string {
	if t == nil || len(t.states) == 0 {
		return ""
	}
	if len(t.missing()) == 0 {
		return "Runtime workflow completion: the required service-backed workflow steps are complete. Produce the final user-facing answer now from the existing service evidence. Do not call more tools unless a required service call failed."
	}
	parts := make([]string, 0, len(t.states))
	for _, state := range t.states {
		if stateComplete(state) {
			continue
		}
		step, ok := currentMissingStep(state)
		if !ok {
			continue
		}
		parts = append(parts, continuationPromptForStep(state, step))
	}
	if len(parts) == 0 {
		return ""
	}
	return "Runtime workflow continuation: respond with tool calls only until the required workflow steps are complete; do not draft a user-facing final answer yet.\n\n" + strings.Join(parts, "\n\n")
}

func (t *Tracker) completedStepTool(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, state := range t.states {
		for _, step := range state.Steps {
			if !stepComplete(step) || !stepSatisfiedBy(step, name) {
				continue
			}
			return true
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

func (t *Tracker) emit(event Event) {
	if t != nil && t.observer != nil {
		t.observer(event)
	}
}

func (t *Tracker) applyObservedIngestionRun(count int, runID string) {
	if t == nil || (count <= 1 && strings.TrimSpace(runID) == "") {
		return
	}
	runID = strings.TrimSpace(runID)
	for i := range t.states {
		if t.states[i].Kind != KindKnowledgeIndex {
			continue
		}
		changed := false
		if t.states[i].RunID == "" && runID != "" {
			t.states[i].RunID = runID
			changed = true
		}
		if count > 1 {
			for j := range t.states[i].Steps {
				step := &t.states[i].Steps[j]
				switch step.ID {
				case "extract", "embed", "index":
					if requiredCount(*step) < count {
						step.RequiredCount = count
						changed = true
					}
				}
			}
		}
		if changed && t.store != nil {
			t.store.ReplaceState(t.states[i])
		}
	}
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
				if call.Function.Name != "ingestion_StartRun" || startRunHasSources(call.Function.Arguments) {
					continue
				}
				t.emit(Event{Type: "blocked_tool_calls", WorkflowID: state.ID, SessionID: state.SessionID, Kind: state.Kind})
				return "Runtime workflow validation blocked ingestion_StartRun because it did not include any source files. List or inspect the directory if needed, then start the ingestion run with every discovered source before continuing.", true
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
		if strings.HasPrefix(tool, "ingestion_") && tool != "ingestion_StartRun" && tool != "ingestion_MarkComplete" {
			return true
		}
	}
	return false
}

func knowledgeIndexServiceFunction(name string) bool {
	for _, prefix := range []string{"citation_", "core_", "document_", "embedding_", "indexer_", "ingestion_", "gateway_"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func devopsServiceFunction(name string) bool {
	for _, prefix := range []string{"repo_", "build_", "test_", "policy_", "build_release_"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func startRunHasSources(arguments string) bool {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.TrimSpace(arguments)), &payload); err != nil {
		return true
	}
	raw, ok := payload["sources"]
	if !ok {
		return false
	}
	var sources []json.RawMessage
	if err := json.Unmarshal(raw, &sources); err != nil {
		return false
	}
	return len(sources) > 0
}

func Detect(prompt string, tools []plugin.ToolSchema) []Intent {
	available := availableTools(tools)
	text := normalize(prompt)
	intents := make([]Intent, 0, 2)
	if looksLikeKnowledgeIndex(text) {
		sourceCount := expectedKnowledgeSourceCount(text)
		if steps := requiredSteps(available,
			step("ingest-start", "ingestion run creation", "ingestion_StartRun"),
			stepCount("extract", "document content extraction", sourceCount, "document_ExtractText", "document_GetPages"),
			stepCount("embed", "embedding generation", sourceCount, "embedding_Embed", "gateway_Embed"),
			stepCount("index", "canonical indexing", sourceCount, "indexer_UpsertChunk"),
			step("ingest-complete", "ingestion run completion", "ingestion_MarkComplete"),
		); len(steps) > 0 {
			intents = append(intents, Intent{Kind: KindKnowledgeIndex, Steps: steps})
		}
	}
	if looksLikeKnowledgeQuery(text) {
		if steps := requiredSteps(available,
			step("embed-query", "query embedding", "embedding_Embed", "gateway_Embed"),
			step("retrieve", "context retrieval", "indexer_QueryContext", "indexer_GetContext"),
			step("ground", "grounding or citation verification", "citation_VerifyGrounding", "citation_RenderReferences"),
		); len(steps) > 0 {
			intents = append(intents, Intent{Kind: KindKnowledgeQuery, Steps: steps})
		}
	}
	if steps := devopsSteps(text, available); len(steps) > 0 {
		intents = append(intents, Intent{Kind: KindDevOps, Steps: steps})
	}
	if steps := systemSteps(text, available); len(steps) > 0 {
		kind := KindSystemInspect
		if containsAny(text, " kill ", " restart ", " stop ", " terminate ") {
			kind = KindSystemMutation
		}
		intents = append(intents, Intent{Kind: kind, Steps: steps})
	}
	return intents
}

func availableTools(tools []plugin.ToolSchema) map[string]struct{} {
	available := make(map[string]struct{}, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) != "" {
			available[tool.Name] = struct{}{}
		}
	}
	return available
}

func requiredSteps(available map[string]struct{}, specs ...Step) []Step {
	steps := make([]Step, 0, len(specs))
	for _, spec := range specs {
		filtered := make([]string, 0, len(spec.AnyOf))
		for _, name := range spec.AnyOf {
			if _, ok := available[name]; ok {
				filtered = append(filtered, name)
			}
		}
		if len(filtered) == 0 {
			continue
		}
		spec.AnyOf = filtered
		steps = append(steps, spec)
	}
	return steps
}

func workflowPromptBlock(state State) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s workflow requirements:\n", state.Kind)
	for i, step := range state.Steps {
		fmt.Fprintf(&b, "%d. %s using one of: %s.\n", i+1, step.Label, strings.Join(step.AnyOf, ", "))
	}
	switch state.Kind {
	case KindKnowledgeIndex:
		b.WriteString("For document indexing, start the ingestion run with the discovered sources before per-source work, extract every source through document service functions, produce embeddings for canonical chunks, persist each canonical chunk with indexer_UpsertChunk using embeddingRef values returned by embedding_Embed, then close the durable run with ingestion_MarkComplete. Every indexer_UpsertChunk call for source documents must be a complete canonical knowledge record with document, sourceMetadata, provenance, non-empty facts, non-empty entities, relations, citations, and either textContentRef or textContent. For batch work, issue all known calls for the same step in one ordered tool-call batch when possible. Never provide manual embedding arrays to indexer functions. Do not use document-only or legacy indexer calls as a substitute for canonical chunk indexing. ingestion_UpdateSourceState may record per-source progress, but it is not the terminal batch completion step. Do not final-answer that indexing is done until ingestion_MarkComplete succeeds after the source content is indexed. After ingestion_MarkComplete succeeds, answer immediately and concisely; do not call more tools unless the terminal call failed.")
	case KindKnowledgeQuery:
		b.WriteString("For knowledge questions, retrieve from the index with a queryVectorRef returned by embedding_Embed before answering and use citation or grounding functions when they are available for the workflow. Never provide manual query vectors to indexer functions. Do not reread original files unless retrieval is insufficient and the user asks for repair or reindexing. After citation or grounding succeeds, answer immediately and concisely from the retrieved evidence; do not call more tools unless retrieval or grounding failed.")
	case KindDevOps:
		b.WriteString("For DevOps work, inspect repository state before running build, test, release, or policy actions. Report only evidence from service results and clearly distinguish dry-run planning from mutations.")
	case KindSystemInspect, KindSystemMutation:
		b.WriteString("For system work, gather read-only inspection evidence before proposing actions. Mutation functions require the configured approval and policy path.")
	}
	return strings.TrimSpace(b.String())
}

func continuationPromptForStep(state State, step Step) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s current required step: %s", state.Kind, step.Label)
	if progress := stepProgress(step); progress != "" {
		fmt.Fprintf(&b, " (%s)", progress)
	}
	fmt.Fprintf(&b, ". Use one of: %s.", strings.Join(step.AnyOf, ", "))
	if detail := continuationDetail(state, step.ID); detail != "" {
		b.WriteString(" ")
		b.WriteString(detail)
	}
	return b.String()
}

func stepProgress(step Step) string {
	required := requiredCount(step)
	if required <= 1 {
		return "not complete"
	}
	remaining := required - step.CompletedCount
	if remaining < 0 {
		remaining = 0
	}
	return fmt.Sprintf("%d of %d complete; %d remaining", step.CompletedCount, required, remaining)
}

func continuationDetail(state State, stepID string) string {
	switch state.Kind {
	case KindKnowledgeIndex:
		return knowledgeIndexContinuationDetail(stepID, state.RunID)
	case KindKnowledgeQuery:
		return knowledgeQueryContinuationDetail(stepID)
	case KindDevOps:
		return "Continue from the existing service evidence and use the next DevOps service function before summarizing."
	case KindSystemInspect, KindSystemMutation:
		return "Use read-only inspection evidence first; mutation functions must follow the configured approval and policy path."
	default:
		return ""
	}
}

func knowledgeIndexContinuationDetail(stepID, runID string) string {
	switch stepID {
	case "ingest-start":
		return "If the source list is not known, inspect the user-provided location with io_List or io_Stat, then call ingestion_StartRun once with every discovered source."
	case "extract":
		return "Extract every remaining source with text/page extraction so source text is available for semantic structuring, embedding, and indexing. Metadata-only parsing does not satisfy this step. Do not use io_Read or io_ExtractPdf as a substitute for extraction and do not embed or index before extraction is complete."
	case "embed":
		return "Create embeddings for the remaining extracted canonical chunks. Batch all known remaining embedding calls in one assistant turn when possible."
	case "index":
		detail := "Persist each remaining canonical chunk with indexer_UpsertChunk using embeddingRef values returned by embedding_Embed. Each upsert must include document, sourceMetadata, provenance, facts, entities, relations, citations, and either textContentRef or textContent. After the final remaining upsert succeeds, close the durable run with ingestion_MarkComplete before answering."
		if runID != "" {
			detail += fmt.Sprintf(" Use runId %q when the workflow reaches ingestion_MarkComplete.", runID)
		}
		return detail
	case "ingest-complete":
		if runID != "" {
			return fmt.Sprintf("Call ingestion_MarkComplete now with runId %q. After it succeeds, answer briefly with what was indexed.", runID)
		}
		return "Call ingestion_MarkComplete now with the runId returned by ingestion_StartRun. After it succeeds, answer briefly with what was indexed."
	default:
		return ""
	}
}

func knowledgeQueryContinuationDetail(stepID string) string {
	switch stepID {
	case "embed-query":
		return "Embed the user question first. Do not invent query vectors."
	case "retrieve":
		return "Retrieve from the index using the query vector reference returned by the embedding service."
	case "ground":
		return "Verify or render citations from retrieved evidence before answering."
	default:
		return ""
	}
}

func devopsSteps(text string, available map[string]struct{}) []Step {
	specs := make([]Step, 0)
	if containsAny(text, " repo ", " repository ", " git ", " status ", " changed ", " diff ") {
		specs = append(specs, step("repo-status", "repository inspection", "repo_Status", "repo_ListChangedFiles", "repo_Diff"))
	}
	if containsAny(text, " project ", " project kind ", " detect project ", " go project ", " package ") {
		specs = append(specs, step("project-detect", "project detection", "build_DetectProject"))
	}
	if containsAny(text, " test ", " tests ", " failing ", " failure ") {
		specs = append(specs, step("tests", "test execution or discovery", "test_RunTests", "test_DiscoverTests"))
	}
	if containsAny(text, " explain ", " failure ", " failing ") {
		specs = append(specs, step("explain-failure", "failure explanation", "test_ExplainFailure"))
	}
	if containsAny(text, " build ", " compile ", " package ") {
		specs = append(specs, step("build", "build execution", "build_RunTask", "build_CreateArtifact"))
	}
	if containsAny(text, " release ", " publish ", " tag ") {
		specs = append(specs, step("release", "release planning", "build_release_DryRun", "repo_GenerateReleaseNotes"))
	}
	if containsAny(text, " release ", " publish ", " deploy ", " apply ", " patch ", " commit ") {
		specs = append(specs, step("policy", "policy evaluation", "policy_EvaluateChange"))
	}
	return requiredSteps(available, specs...)
}

func systemSteps(text string, available map[string]struct{}) []Step {
	specs := make([]Step, 0)
	if containsAny(text, " snapshot ", " system ", " machine ", " host ") {
		specs = append(specs, step("snapshot", "system snapshot", "system_Snapshot"))
	}
	if containsAny(text, " process ", " processes ", " pid ", " kill ", " terminate ") {
		specs = append(specs, step("processes", "process inspection", "system_ListProcesses", "system_KillProcess"))
	}
	if containsAny(text, " port ", " ports ", " network ", " socket ", " connection ") {
		specs = append(specs, step("network", "network inspection", "system_ListPorts", "system_ListNetworkConnections"))
	}
	if containsAny(text, " disk ", " mount ", " filesystem ", " storage ") {
		specs = append(specs, step("disk", "disk or mount inspection", "system_GetDiskUsage", "system_ListMounts"))
	}
	if containsAny(text, " log ", " logs ", " journal ") {
		specs = append(specs, step("logs", "log inspection", "system_ReadLogs"))
	}
	if containsAny(text, " metric ", " metrics ", " memory ", " load ", " cpu ") {
		specs = append(specs, step("metrics", "system metrics", "system_GetMetrics"))
	}
	if containsAny(text, " service ", " services ", " restart ") {
		specs = append(specs, step("services", "service inspection or restart plan", "system_ListServices", "system_RestartService"))
	}
	if containsAny(text, " package ", " packages ", " installed ") {
		specs = append(specs, step("packages", "package inventory", "system_ListPackages"))
	}
	return requiredSteps(available, specs...)
}

func step(id, label string, anyOf ...string) Step {
	return stepCount(id, label, 1, anyOf...)
}

func stepCount(id, label string, count int, anyOf ...string) Step {
	if count <= 0 {
		count = 1
	}
	return Step{ID: id, Label: label, RequiredCount: count, AnyOf: append([]string(nil), anyOf...)}
}

func stepSatisfiedBy(step Step, tool string) bool {
	for _, name := range step.AnyOf {
		if name == tool {
			return true
		}
	}
	return false
}

func stateComplete(state State) bool {
	for _, step := range state.Steps {
		if !stepComplete(step) {
			return false
		}
	}
	return true
}

func stepComplete(step Step) bool {
	return step.CompletedCount >= requiredCount(step)
}

func requiredCount(step Step) int {
	if step.RequiredCount > 0 {
		return step.RequiredCount
	}
	return 1
}

var nonWord = regexp.MustCompile(`[^a-z0-9_/-]+`)

func normalize(input string) string {
	lower := strings.ToLower(input)
	return " " + strings.TrimSpace(nonWord.ReplaceAllString(lower, " ")) + " "
}

func looksLikeKnowledgeIndex(text string) bool {
	return containsAny(text, " index ", " ingest ", " add ") &&
		containsAny(text, " file ", " files ", " pdf ", " document ", " documents ", " directory ", " folder ", " markdown ")
}

func looksLikeKnowledgeQuery(text string) bool {
	if looksLikeKnowledgeIndex(text) {
		return false
	}
	return containsAny(text, " what ", " who ", " when ", " where ", " why ", " how ", " query ", " search ", " find ", " answer ", " summarize ") &&
		containsAny(text, " document ", " documents ", " index ", " indexed ", " knowledge ", " source ", " sources ", " pdf ")
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func expectedKnowledgeSourceCount(text string) int {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`\ball\s+(\d{1,3})\s+(?:documents?|files?|pdfs?|records?|markdown)\b`),
		regexp.MustCompile(`\b(\d{1,3})\s+(?:documents?|files?|pdfs?|records?|markdown)\b`),
	}
	for _, pattern := range patterns {
		match := pattern.FindStringSubmatch(text)
		if len(match) != 2 {
			continue
		}
		var count int
		if _, err := fmt.Sscanf(match[1], "%d", &count); err == nil && count > 0 {
			return count
		}
	}
	return 1
}

func observedSourceCount(result string) int {
	var payload any
	if err := json.Unmarshal([]byte(strings.TrimSpace(result)), &payload); err != nil {
		return 0
	}
	return maxSourceArrayLen(payload)
}

func observedIngestionRunID(result string) string {
	var payload any
	if err := json.Unmarshal([]byte(strings.TrimSpace(result)), &payload); err != nil {
		return ""
	}
	return findIngestionRunID(payload)
}

func findIngestionRunID(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		if run, ok := typed["run"]; ok {
			if id := runObjectID(run); id != "" {
				return id
			}
		}
		for _, child := range typed {
			if id := findIngestionRunID(child); id != "" {
				return id
			}
		}
	case []any:
		for _, child := range typed {
			if id := findIngestionRunID(child); id != "" {
				return id
			}
		}
	}
	return ""
}

func runObjectID(value any) string {
	run, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	if id, ok := run["id"].(string); ok {
		return strings.TrimSpace(id)
	}
	return ""
}

func maxSourceArrayLen(value any) int {
	switch typed := value.(type) {
	case map[string]any:
		maxCount := 0
		for key, child := range typed {
			count := maxSourceArrayLen(child)
			if strings.EqualFold(key, "sources") {
				if sources, ok := child.([]any); ok && len(sources) > count {
					count = len(sources)
				}
			}
			if count > maxCount {
				maxCount = count
			}
		}
		return maxCount
	case []any:
		maxCount := 0
		for _, child := range typed {
			if count := maxSourceArrayLen(child); count > maxCount {
				maxCount = count
			}
		}
		return maxCount
	default:
		return 0
	}
}

func toolResultSucceeded(result string, err error) bool {
	if err != nil {
		return false
	}
	trimmed := strings.TrimSpace(result)
	if trimmed == "" || strings.HasPrefix(trimmed, "error:") {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
		if isError, ok := payload["is_error"].(bool); ok && isError {
			return false
		}
		if success, ok := payload["success"].(bool); ok {
			return success
		}
	}
	return true
}

func cloneSteps(in []Step) []Step {
	out := make([]Step, len(in))
	for i, step := range in {
		out[i] = step
		if out[i].RequiredCount <= 0 {
			out[i].RequiredCount = 1
		}
		out[i].AnyOf = append([]string(nil), step.AnyOf...)
	}
	return out
}

func cloneState(in State) State {
	in.Steps = cloneSteps(in.Steps)
	return in
}
