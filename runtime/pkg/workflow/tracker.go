package workflow

import (
	"context"
	"strings"

	"github.com/quarkloop/pkg/plugin"
)

// Tracker correlates completed tool results to transient workflow progress.
// Tool execution remains owned by the runtime tool surface.
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
	if name == "runstate_StartRun" {
		t.applyObservedRunStateRun(observedItemCount(result), observedRunStateRunID(result))
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

func (t *Tracker) emit(event Event) {
	if t != nil && t.observer != nil {
		t.observer(event)
	}
}

func (t *Tracker) applyObservedRunStateRun(count int, runID string) {
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
