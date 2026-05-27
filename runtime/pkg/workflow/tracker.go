package workflow

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/quarkloop/pkg/plugin"
)

// Tracker correlates completed tool results to transient workflow progress.
// Tool execution remains owned by the runtime tool surface.
type Tracker struct {
	store    *Store
	states   []State
	observer func(Event)
	observed map[string]map[string]struct{}
	issued   map[string]struct{}
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
	tracker := &Tracker{
		store: store, states: states, observer: observer,
		observed: make(map[string]map[string]struct{}),
		issued:   make(map[string]struct{}),
	}
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
			t.RecordToolResult(name, arguments, result)
		}
		return result, err
	}
}

// CallableTools narrows one model turn to each active workflow's current
// operation and explicitly supported preparation operations. Policy remains
// authoritative for validating any returned call.
func (t *Tracker) CallableTools(tools []plugin.ToolSchema) []plugin.ToolSchema {
	if t == nil {
		return tools
	}
	allowed := make(map[string]struct{})
	queryInlineEmbedding := false
	canonicalIndexBatch := false
	for _, state := range t.states {
		current, ok := currentMissingStep(state)
		if !ok {
			continue
		}
		if state.Kind == KindKnowledgeQuery && current.ID == "embed-query" {
			queryInlineEmbedding = true
		}
		if state.Kind == KindKnowledgeIndex && current.ID == "index" {
			canonicalIndexBatch = true
		}
		for _, name := range current.AnyOf {
			allowed[name] = struct{}{}
		}
		for _, tool := range tools {
			if t.supportToolAllowed(state, current, tool.Name) {
				allowed[tool.Name] = struct{}{}
			}
		}
	}
	selected := make([]plugin.ToolSchema, 0, len(allowed))
	for _, tool := range tools {
		if _, ok := allowed[tool.Name]; ok {
			if queryInlineEmbedding && tool.Name == "gateway_Embed" {
				tool.Description = strings.TrimSpace(tool.Description + " For this indexed-knowledge question, submit only the text parameter with one non-empty retrieval query that faithfully represents the current request.")
				tool.Parameters = queryEmbeddingParameters()
			}
			if canonicalIndexBatch && tool.Name == "indexer_UpsertChunk" {
				tool.Description = strings.TrimSpace(tool.Description + " Complete the current indexing step by calling this once per prepared source whose embedding succeeded; independent source records may be returned together in one tool-call batch and remain validated separately.")
			}
			selected = append(selected, tool)
		}
	}
	return selected
}

func queryEmbeddingParameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "One non-empty retrieval query faithfully representing the user's indexed-knowledge question.",
			},
		},
		"required":             []string{"text"},
		"additionalProperties": false,
	}
}

func (t *Tracker) supportToolAllowed(state State, current Step, tool string) bool {
	if !workflowSupportToolAllowed(state.Kind, current.ID, tool) {
		return false
	}
	if state.Kind == KindKnowledgeIndex && current.ID == "embed" &&
		knowledgeEvidencePreparationToolAllowed(current.ID, tool) {
		return t.observedCount(state.ID, "embed-evidence") < requiredCount(current)
	}
	return true
}

func (t *Tracker) observedCount(workflowID, stepID string) int {
	if t == nil {
		return 0
	}
	return len(t.observed[workflowID+"."+stepID])
}

// RequiredToolContinuation supplies terminal durable bookkeeping after the
// semantic indexing work is complete. The LLM loop still executes and traces
// the returned call through its ordinary tool-call path.
func (t *Tracker) RequiredToolContinuation() []plugin.ToolCall {
	if t == nil {
		return nil
	}
	for _, state := range t.states {
		if state.Kind != KindKnowledgeIndex || strings.TrimSpace(state.RunID) == "" {
			continue
		}
		step, ok := currentMissingStep(state)
		if !ok || step.ID != "ingest-complete" {
			continue
		}
		if _, alreadyIssued := t.issued[state.ID]; alreadyIssued {
			return nil
		}
		arguments, err := json.Marshal(map[string]string{"runId": state.RunID})
		if err != nil {
			return nil
		}
		t.issued[state.ID] = struct{}{}
		t.emit(Event{
			Type:       "required_action_issued",
			WorkflowID: state.ID,
			SessionID:  state.SessionID,
			Kind:       state.Kind,
			StepID:     step.ID,
			Tool:       "runstate_MarkComplete",
		})
		return []plugin.ToolCall{{
			ID:   "runtime-workflow-" + state.ID + "-complete",
			Type: "function",
			Function: plugin.ToolCallFunction{
				Name:      "runstate_MarkComplete",
				Arguments: string(arguments),
			},
		}}
	}
	return nil
}

func (t *Tracker) RecordToolResult(name, arguments, result string) {
	if t == nil || t.store == nil {
		return
	}
	if name == "runstate_StartRun" {
		t.applyObservedRunStateRunID(observedRunStateRunID(result))
	}
	for i := range t.states {
		state := t.states[i]
		step, ok := currentMissingStep(state)
		if !ok {
			continue
		}
		if state.Kind == KindKnowledgeIndex && step.ID == "embed" && knowledgeEvidencePreparationToolAllowed(step.ID, name) {
			if identity := evidencePreparationSourceIdentity(arguments); identity != "" {
				t.recordObserved(state.ID, "embed-evidence", identity)
			}
			continue
		}
		if !stepSatisfiedBy(step, name) {
			continue
		}
		for _, identity := range workflowCompletionIdentities(step.ID, name, arguments, result) {
			current, ok := currentMissingStep(t.states[i])
			if !ok || current.ID != step.ID {
				break
			}
			if identity != "" && t.hasObserved(state.ID, step.ID, identity) {
				t.emit(Event{Type: "duplicate_result_ignored", WorkflowID: state.ID, SessionID: state.SessionID, Kind: state.Kind, StepID: step.ID, Tool: name})
				continue
			}
			updated, ok := t.store.CompleteStep(state.SessionID, state.ID, step.ID, name)
			if !ok {
				break
			}
			t.recordObserved(state.ID, step.ID, identity)
			t.states[i] = updated
			t.emit(Event{Type: "step_completed", WorkflowID: state.ID, SessionID: state.SessionID, Kind: state.Kind, StepID: step.ID, Tool: name})
		}
	}
}

func (t *Tracker) hasObserved(workflowID, stepID, identity string) bool {
	if t == nil || identity == "" {
		return false
	}
	_, ok := t.observed[workflowID+"."+stepID][identity]
	return ok
}

func (t *Tracker) recordObserved(workflowID, stepID, identity string) {
	if t == nil || identity == "" {
		return
	}
	key := workflowID + "." + stepID
	if t.observed[key] == nil {
		t.observed[key] = make(map[string]struct{})
	}
	t.observed[key][identity] = struct{}{}
}

func (t *Tracker) emit(event Event) {
	if t != nil && t.observer != nil {
		t.observer(event)
	}
}

func (t *Tracker) applyObservedRunStateRunID(runID string) {
	if t == nil || strings.TrimSpace(runID) == "" {
		return
	}
	runID = strings.TrimSpace(runID)
	for i := range t.states {
		if t.states[i].Kind != KindKnowledgeIndex {
			continue
		}
		if t.states[i].RunID != "" {
			continue
		}
		t.states[i].RunID = runID
		t.store.ReplaceState(t.states[i])
	}
}
