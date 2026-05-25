package agent

import (
	"context"
	"time"

	"github.com/quarkloop/runtime/pkg/execution"
	"github.com/quarkloop/runtime/pkg/hierarchy"
	"github.com/quarkloop/runtime/pkg/loop"
	"github.com/quarkloop/runtime/pkg/permissions"
)

func (a *Agent) Identity() *hierarchy.Identity {
	return a.identity
}

func (a *Agent) Agents() *hierarchy.Registry {
	return a.agents
}

func (a *Agent) Delegator() *hierarchy.Delegator {
	return a.delegator
}

func (a *Agent) Execution() *execution.ExecutionRuntime {
	return a.execution
}

func (a *Agent) Permissions() *permissions.Checker {
	return a.permissions
}

func (a *Agent) SpawnSubAgent(config *hierarchy.SpawnConfig) (*hierarchy.AgentEntry, error) {
	if config != nil && config.ProfileID != "" {
		if err := a.handoff.ValidateTarget(config.ProfileID); err != nil {
			return nil, err
		}
	}
	return a.agents.Spawn(a.ID, config)
}

func (a *Agent) DelegateWork(ctx context.Context, agentID, task string, timeout time.Duration) (hierarchy.WorkResult, error) {
	work := hierarchy.WorkItem{
		BaseMessage: loop.NewPriorityMessage("work_item", 5),
		AgentID:     agentID,
		Task:        task,
		Timeout:     timeout,
	}
	return a.delegator.DelegateAndWait(ctx, a.ID, work)
}
