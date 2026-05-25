package workflow

import "encoding/json"

// ContinuationStatus exposes runtime-owned workflow state without authoring
// behavioral prompt text. Agent plugins own all workflow guidance.
func (t *Tracker) ContinuationStatus() string {
	if t == nil || len(t.states) == 0 {
		return ""
	}
	fact := workflowStatusFact{Type: "runtime.workflow.status", Status: "incomplete"}
	if !t.hasMissingSteps() {
		fact.Status = "complete"
	}
	for _, state := range t.states {
		item := workflowStateFact{Kind: state.Kind, RunID: state.RunID}
		if step, ok := currentMissingStep(state); ok {
			item.CurrentStep = step.Label
			item.CompletedCount = step.CompletedCount
			item.RequiredCount = requiredCount(step)
			item.AllowedFunctions = append([]string(nil), step.AnyOf...)
		}
		fact.Workflows = append(fact.Workflows, item)
	}
	data, err := json.Marshal(fact)
	if err != nil {
		return ""
	}
	return string(data)
}

type workflowStatusFact struct {
	Type      string              `json:"type"`
	Status    string              `json:"status"`
	Workflows []workflowStateFact `json:"workflows"`
}

type workflowStateFact struct {
	Kind             Kind     `json:"kind"`
	RunID            string   `json:"run_id,omitempty"`
	CurrentStep      string   `json:"current_step,omitempty"`
	CompletedCount   int      `json:"completed_count,omitempty"`
	RequiredCount    int      `json:"required_count,omitempty"`
	AllowedFunctions []string `json:"allowed_functions,omitempty"`
}
