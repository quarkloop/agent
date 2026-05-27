// Package workflow tracks runtime-owned multi-step service workflows.
package workflow

import (
	"fmt"
	"sync"
	"time"
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
	Type       string         `json:"type"`
	WorkflowID string         `json:"workflow_id"`
	SessionID  string         `json:"session_id"`
	Kind       Kind           `json:"kind"`
	StepID     string         `json:"step_id,omitempty"`
	Tool       string         `json:"tool,omitempty"`
	Reason     string         `json:"reason,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
}

type Intent struct {
	Kind  Kind
	Steps []Step
}

// Store owns transient runtime workflow progress. Durable service workflow
// history remains owned by the workflow service.
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
