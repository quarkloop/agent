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
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	AnyOf       []string `json:"any_of"`
	CompletedBy string   `json:"completed_by,omitempty"`
	CompletedAt string   `json:"completed_at,omitempty"`
}

type State struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Kind      Kind      `json:"kind"`
	Status    string    `json:"status"`
	Prompt    string    `json:"prompt,omitempty"`
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
			if step.ID != stepID || step.CompletedBy != "" {
				continue
			}
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
			t.RecordToolResult(name)
		}
		return result, err
	}
}

func (t *Tracker) RecordToolResult(name string) {
	if t == nil || t.store == nil {
		return
	}
	for i := range t.states {
		state := t.states[i]
		for _, step := range state.Steps {
			if step.CompletedBy != "" || !stepSatisfiedBy(step, name) {
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
	return "Runtime workflow validation blocked finalization. " + strings.Join(missing, " ") + " Continue using the existing service results and complete the missing service-backed workflow steps before producing the final answer.", true
}

func (t *Tracker) missing() []string {
	missing := make([]string, 0)
	for _, state := range t.states {
		for _, step := range state.Steps {
			if step.CompletedBy != "" {
				continue
			}
			missing = append(missing, fmt.Sprintf("%s workflow still needs %s using one of: %s.", state.Kind, step.Label, strings.Join(step.AnyOf, ", ")))
		}
	}
	sort.Strings(missing)
	return missing
}

func (t *Tracker) emit(event Event) {
	if t != nil && t.observer != nil {
		t.observer(event)
	}
}

func Detect(prompt string, tools []plugin.ToolSchema) []Intent {
	available := availableTools(tools)
	text := normalize(prompt)
	intents := make([]Intent, 0, 2)
	if looksLikeKnowledgeIndex(text) {
		if steps := requiredSteps(available,
			step("ingest-start", "ingestion run creation", "ingestion_StartRun"),
			step("extract", "document extraction", "document_ExtractText", "document_ParseBytes", "document_GetPages"),
			step("embed", "embedding generation", "embedding_Embed", "model_Embed"),
			step("index", "canonical indexing", "indexer_UpsertChunk", "indexer_IndexDocument"),
			step("ingest-complete", "ingestion run completion", "ingestion_MarkComplete"),
		); len(steps) > 0 {
			intents = append(intents, Intent{Kind: KindKnowledgeIndex, Steps: steps})
		}
	}
	if looksLikeKnowledgeQuery(text) {
		if steps := requiredSteps(available,
			step("embed-query", "query embedding", "embedding_Embed", "model_Embed"),
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

func devopsSteps(text string, available map[string]struct{}) []Step {
	specs := make([]Step, 0)
	if containsAny(text, " repo ", " repository ", " git ", " status ", " changed ", " diff ") {
		specs = append(specs, step("repo-status", "repository inspection", "repo_Status", "repo_ListChangedFiles", "repo_Diff"))
	}
	if containsAny(text, " test ", " tests ", " failing ", " failure ") {
		specs = append(specs, step("tests", "test execution or discovery", "test_RunTests", "test_DiscoverTests"))
	}
	if containsAny(text, " build ", " compile ", " package ") {
		specs = append(specs, step("build", "build execution", "build_RunTask", "build_CreateArtifact"))
	}
	if containsAny(text, " release ", " publish ", " tag ") {
		specs = append(specs, step("release", "release planning", "build_release_DryRun", "repo_GenerateReleaseNotes"))
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
	return Step{ID: id, Label: label, AnyOf: append([]string(nil), anyOf...)}
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
		if step.CompletedBy == "" {
			return false
		}
	}
	return true
}

var nonWord = regexp.MustCompile(`[^a-z0-9_/-]+`)

func normalize(input string) string {
	lower := strings.ToLower(input)
	return " " + strings.TrimSpace(nonWord.ReplaceAllString(lower, " ")) + " "
}

func looksLikeKnowledgeIndex(text string) bool {
	return containsAny(text, " index ", " ingest ", " add ", " catalog ") &&
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
		out[i].AnyOf = append([]string(nil), step.AnyOf...)
	}
	return out
}

func cloneState(in State) State {
	in.Steps = cloneSteps(in.Steps)
	return in
}
