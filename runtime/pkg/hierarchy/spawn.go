package hierarchy

import (
	"errors"
	"time"
)

// SpawnConfig holds configuration for spawning a sub-agent.
type SpawnConfig struct {
	// ID is the unique identifier for the new agent.
	// If empty, one will be generated.
	ID string

	// Name is a human-readable name for the agent.
	Name string

	// Description describes what this agent will do.
	Description string

	// ProfileID is the resolved agent profile used by this sub-agent.
	ProfileID string

	// Task is the initial task or prompt for the agent.
	Task string

	// Permissions defines what the sub-agent is allowed to do.
	// Must be a subset of the parent's permissions.
	Permissions *Permissions

	// Timeout is the maximum duration the sub-agent can run.
	// Zero means no timeout.
	Timeout time.Duration

	// MaxSteps limits the number of work steps the agent can execute.
	// Zero means unlimited.
	MaxSteps int
}

// Permissions defines what an agent is allowed to do.
// Sub-agent permissions must be a subset of parent permissions.
type Permissions struct {
	// AllowedTools is the list of tools the agent can use.
	// Empty means all tools allowed by parent.
	AllowedTools []string

	// DeniedTools is the list of tools the agent cannot use.
	DeniedTools []string

	// AllowedPaths is the list of filesystem paths the agent can access.
	AllowedPaths []string

	// ReadOnlyPaths is the list of paths that are read-only.
	ReadOnlyPaths []string

	// AllowedHosts is the list of network hosts the agent can reach.
	AllowedHosts []string

	// DeniedHosts is the list of hosts the agent cannot reach.
	DeniedHosts []string

	// CanSpawnAgents indicates if this agent can spawn sub-agents.
	CanSpawnAgents bool

	// MaxSubAgents limits the number of sub-agents this agent can spawn.
	MaxSubAgents int
}

// DefaultPermissions returns a default permissive set of permissions.
func DefaultPermissions() *Permissions {
	return &Permissions{
		CanSpawnAgents: true,
		MaxSubAgents:   10,
	}
}

// RestrictedPermissions returns a restricted set of permissions for sub-agents.
func RestrictedPermissions() *Permissions {
	return &Permissions{
		CanSpawnAgents: false,
		MaxSubAgents:   0,
	}
}

// Validate checks if the spawn configuration is valid.
func (c *SpawnConfig) Validate() error {
	if c.Name == "" {
		return errors.New("spawn config: name is required")
	}
	if c.Task == "" {
		return errors.New("spawn config: task is required")
	}
	return nil
}

// IsToolAllowed checks if a tool is allowed by these permissions.
func (p *Permissions) IsToolAllowed(tool string) bool {
	// Check denied list first
	for _, denied := range p.DeniedTools {
		if denied == tool {
			return false
		}
	}

	// If allowed list is empty, all tools are allowed
	if len(p.AllowedTools) == 0 {
		return true
	}

	// Check if in allowed list
	for _, allowed := range p.AllowedTools {
		if allowed == tool {
			return true
		}
	}
	return false
}

// IsHostAllowed checks if a network host is allowed by these permissions.
func (p *Permissions) IsHostAllowed(host string) bool {
	// Check denied list first
	for _, denied := range p.DeniedHosts {
		if denied == host {
			return false
		}
	}

	// If allowed list is empty, all hosts are allowed
	if len(p.AllowedHosts) == 0 {
		return true
	}

	// Check if in allowed list
	for _, allowed := range p.AllowedHosts {
		if allowed == host {
			return true
		}
	}
	return false
}

// IsSubsetOf checks if these permissions are a subset of parent permissions.
// Sub-agent permissions must not exceed parent permissions.
func (p *Permissions) IsSubsetOf(parent *Permissions) bool {
	if parent == nil {
		return true // No parent permissions means no restrictions
	}

	// Sub-agent cannot spawn if parent cannot
	if p.CanSpawnAgents && !parent.CanSpawnAgents {
		return false
	}

	// Sub-agent cannot have more sub-agents than parent
	if p.MaxSubAgents > parent.MaxSubAgents {
		return false
	}

	// Check tool permissions
	for _, tool := range p.AllowedTools {
		if !parent.IsToolAllowed(tool) {
			return false
		}
	}

	// Check host permissions
	for _, host := range p.AllowedHosts {
		if !parent.IsHostAllowed(host) {
			return false
		}
	}

	return true
}

// Restrict creates a new Permissions that is the intersection of p and restrictions.
func (p *Permissions) Restrict(restrictions *Permissions) *Permissions {
	if restrictions == nil {
		return p
	}

	result := &Permissions{
		CanSpawnAgents: p.CanSpawnAgents && restrictions.CanSpawnAgents,
		MaxSubAgents:   min(p.MaxSubAgents, restrictions.MaxSubAgents),
	}

	// Combine denied lists
	result.DeniedTools = append(p.DeniedTools, restrictions.DeniedTools...)
	result.DeniedHosts = append(p.DeniedHosts, restrictions.DeniedHosts...)

	// Intersect allowed lists (if both have them)
	if len(p.AllowedTools) > 0 && len(restrictions.AllowedTools) > 0 {
		result.AllowedTools = intersect(p.AllowedTools, restrictions.AllowedTools)
	} else if len(restrictions.AllowedTools) > 0 {
		result.AllowedTools = restrictions.AllowedTools
	} else {
		result.AllowedTools = p.AllowedTools
	}

	if len(p.AllowedHosts) > 0 && len(restrictions.AllowedHosts) > 0 {
		result.AllowedHosts = intersect(p.AllowedHosts, restrictions.AllowedHosts)
	} else if len(restrictions.AllowedHosts) > 0 {
		result.AllowedHosts = restrictions.AllowedHosts
	} else {
		result.AllowedHosts = p.AllowedHosts
	}

	// Combine path lists
	if len(restrictions.AllowedPaths) > 0 {
		result.AllowedPaths = restrictions.AllowedPaths
	} else {
		result.AllowedPaths = p.AllowedPaths
	}
	result.ReadOnlyPaths = append(p.ReadOnlyPaths, restrictions.ReadOnlyPaths...)

	return result
}

// intersect returns elements present in both slices.
func intersect(a, b []string) []string {
	set := make(map[string]bool)
	for _, v := range b {
		set[v] = true
	}

	var result []string
	for _, v := range a {
		if set[v] {
			result = append(result, v)
		}
	}
	return result
}
