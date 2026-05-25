// Package plan manages the agent's autonomous work plan and execution.
package plan

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/quarkloop/pkg/plugin"
)

// Inferrer is a function that calls the LLM. Injected by the agent to avoid
// plan importing the llm package directly.
// Signature matches llm.Client.Infer for direct use without adapters.
type Inferrer func(ctx context.Context, messages []plugin.Message, tools []plugin.ToolSchema, onTool plugin.ToolHandler, onMessage func(msgType string, data any)) (string, error)

// Step is a single unit of work in a plan.
type Step struct {
	description string
	status      string
	result      string
}

// Description returns the step description.
func (s *Step) Description() string {
	return s.description
}

// Status returns the step status.
func (s *Step) Status() string {
	return s.status
}

// Result returns the step result.
func (s *Step) Result() string {
	return s.result
}

// WorkContext holds accumulated history from autonomous work execution.
type WorkContext struct {
	mu      sync.RWMutex
	History []plugin.Message
}

// AddEntry appends an entry to the work history.
func (wc *WorkContext) AddEntry(role, content string) {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	wc.History = append(wc.History, plugin.Message{Role: role, Content: content})
}

// GetHistory returns a copy of the work history.
func (wc *WorkContext) GetHistory() []plugin.Message {
	wc.mu.RLock()
	defer wc.mu.RUnlock()
	out := make([]plugin.Message, len(wc.History))
	copy(out, wc.History)
	return out
}

// Plan holds the agent's autonomous work state and execution logic.
type Plan struct {
	mu       sync.RWMutex
	steps    []Step
	status   string // idle, active, paused, completed
	workCtx  WorkContext
	nextStep chan struct{}
}

// New creates a new idle Plan.
func New() *Plan {
	return &Plan{
		status:   "idle",
		nextStep: make(chan struct{}, 1),
	}
}

// NextStep returns a channel that signals when a work step is ready.
func (p *Plan) NextStep() <-chan struct{} {
	return p.nextStep
}

// SetSteps replaces the current steps and activates the plan.
func (p *Plan) SetSteps(steps []Step) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.steps = steps
	p.status = "active"
	p.workCtx = WorkContext{} // reset work context for new plan
	if len(steps) > 0 {
		select {
		case p.nextStep <- struct{}{}:
		default:
		}
	}
}

// Pause pauses plan execution.
func (p *Plan) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = "paused"
}

// Resume resumes a paused plan.
func (p *Plan) Resume() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.status == "paused" {
		p.status = "active"
		select {
		case p.nextStep <- struct{}{}:
		default:
		}
	}
}

// GetStatus returns the plan status.
func (p *Plan) GetStatus() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

// GetSteps returns a copy of the current steps.
func (p *Plan) GetSteps() []Step {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]Step, len(p.steps))
	copy(out, p.steps)
	return out
}

// GetSummary returns a concise runtime status fact for Harness packaging.
func (p *Plan) GetSummary() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.status == "idle" {
		return "No active work."
	}
	if p.status == "paused" {
		return "Work paused."
	}
	for i, s := range p.steps {
		if s.status == "pending" || s.status == "active" {
			return fmt.Sprintf("executing step %d/%d — %q", i+1, len(p.steps), s.description)
		}
	}
	return "All steps completed."
}

// ExecuteStep runs the current pending step using the provided infer function.
func (p *Plan) ExecuteStep(ctx context.Context, infer Inferrer, baseMessages []plugin.Message, tools []plugin.ToolSchema, onTool plugin.ToolHandler) error {
	// Find the current pending step index under the lock.
	p.mu.Lock()
	stepIdx := -1
	for i := range p.steps {
		if p.steps[i].status == "pending" {
			stepIdx = i
			p.steps[i].status = "active"
			break
		}
	}
	p.mu.Unlock()

	if stepIdx == -1 {
		return nil
	}

	// Build work context messages
	msgs := append([]plugin.Message(nil), baseMessages...)

	for _, m := range p.workCtx.GetHistory() {
		msgs = append(msgs, plugin.Message{Role: m.Role, Content: m.Content})
	}

	p.mu.Lock()
	stepFact, _ := json.Marshal(map[string]string{"type": "runtime.plan.step", "description": p.steps[stepIdx].description})
	msgs = append(msgs, plugin.Message{Role: "system", Content: string(stepFact)})
	p.mu.Unlock()

	// Call LLM (no user stream for work execution)
	result, err := infer(ctx, msgs, tools, onTool, nil)

	// Write step outcome under lock.
	p.mu.Lock()
	if err != nil {
		p.steps[stepIdx].status = "failed"
		p.steps[stepIdx].result = err.Error()
	} else {
		p.steps[stepIdx].status = "completed"
		p.steps[stepIdx].result = result
	}
	p.mu.Unlock()

	if err != nil {
		return err
	}

	// Store in work context
	p.workCtx.AddEntry("user", fmt.Sprintf("Step: %s", p.steps[stepIdx].description))
	p.workCtx.AddEntry("assistant", result)

	// Advance to next step
	p.advance()
	return nil
}

// advance signals the next step if one is available.
func (p *Plan) advance() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, s := range p.steps {
		if s.status == "pending" {
			select {
			case p.nextStep <- struct{}{}:
			default:
			}
			return
		}
	}
	p.status = "completed"
}
