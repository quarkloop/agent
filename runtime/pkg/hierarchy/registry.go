package hierarchy

import (
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
)

// AgentStatus represents the current status of an agent.
type AgentStatus string

const (
	StatusPending  AgentStatus = "pending"
	StatusRunning  AgentStatus = "running"
	StatusPaused   AgentStatus = "paused"
	StatusComplete AgentStatus = "complete"
	StatusFailed   AgentStatus = "failed"
)

// AgentEntry holds information about a registered agent.
type AgentEntry struct {
	Identity    *Identity
	Permissions *Permissions
	Status      AgentStatus
	Task        string
	Result      string
	Error       string
	CreatedAt   time.Time
	StartedAt   time.Time
	CompletedAt time.Time
}

// Registry tracks all agents within a runtime process.
// It maintains the hierarchy and enables work delegation.
type Registry struct {
	mu     sync.RWMutex
	agents map[string]*AgentEntry
	main   string              // ID of the main agent
	tree   map[string][]string // parent -> children mapping
}

// NewRegistry creates a new agent registry.
func NewRegistry() *Registry {
	return &Registry{
		agents: make(map[string]*AgentEntry),
		tree:   make(map[string][]string),
	}
}

// RegisterMain registers the main agent.
func (r *Registry) RegisterMain(id, name, description string, perms *Permissions) (*AgentEntry, error) {
	return r.RegisterMainWithProfile(id, name, description, "", perms)
}

// RegisterMainWithProfile registers the main agent with its resolved profile ID.
func (r *Registry) RegisterMainWithProfile(id, name, description, profileID string, perms *Permissions) (*AgentEntry, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.main != "" {
		return nil, errors.New("main agent already registered")
	}

	entry := &AgentEntry{
		Identity:    NewMainIdentity(id, name, description).WithProfile(profileID),
		Permissions: perms,
		Status:      StatusPending,
		CreatedAt:   time.Now(),
	}

	r.agents[id] = entry
	r.main = id
	r.tree[id] = nil

	return entry, nil
}

// Spawn creates a new sub-agent under the given parent.
func (r *Registry) Spawn(parentID string, config *SpawnConfig) (*AgentEntry, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	parent, ok := r.agents[parentID]
	if !ok {
		return nil, errors.New("parent agent not found")
	}

	// Check parent can spawn
	if parent.Permissions != nil && !parent.Permissions.CanSpawnAgents {
		return nil, errors.New("parent agent cannot spawn sub-agents")
	}

	// Check max sub-agents
	if parent.Permissions != nil {
		children := r.tree[parentID]
		if len(children) >= parent.Permissions.MaxSubAgents {
			return nil, errors.New("parent agent has reached max sub-agents")
		}
	}

	// Generate ID if not provided
	id := config.ID
	if id == "" {
		id = uuid.New().String()
	}

	// Check ID not already used
	if _, exists := r.agents[id]; exists {
		return nil, errors.New("agent ID already exists")
	}

	// Validate permissions are subset of parent
	perms := config.Permissions
	if perms == nil {
		perms = RestrictedPermissions()
	}
	if parent.Permissions != nil && !perms.IsSubsetOf(parent.Permissions) {
		return nil, errors.New("sub-agent permissions must be subset of parent")
	}

	entry := &AgentEntry{
		Identity:    NewSubIdentity(id, parentID, config.Name, config.Description).WithProfile(config.ProfileID),
		Permissions: perms,
		Status:      StatusPending,
		Task:        config.Task,
		CreatedAt:   time.Now(),
	}

	r.agents[id] = entry
	r.tree[parentID] = append(r.tree[parentID], id)
	r.tree[id] = nil

	return entry, nil
}

// Get returns the agent entry with the given ID.
func (r *Registry) Get(id string) (AgentEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.agents[id]
	if !ok {
		return AgentEntry{}, false
	}
	return *entry, true
}

// GetMain returns the main agent entry.
func (r *Registry) GetMain() (AgentEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.agents[r.main]
	if !ok {
		return AgentEntry{}, false
	}
	return *entry, true
}

// Children returns the IDs of all children of the given parent.
func (r *Registry) Children(parentID string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	children := r.tree[parentID]
	result := make([]string, len(children))
	copy(result, children)
	return result
}

// Descendants returns all descendant IDs (children, grandchildren, etc.) of the given agent.
func (r *Registry) Descendants(id string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []string
	var collect func(parentID string)
	collect = func(parentID string) {
		for _, childID := range r.tree[parentID] {
			result = append(result, childID)
			collect(childID)
		}
	}
	collect(id)
	return result
}

// Ancestors returns all ancestor IDs (parent, grandparent, etc.) up to root.
func (r *Registry) Ancestors(id string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []string
	current := r.agents[id]
	for current != nil && current.Identity.ParentID != "" {
		result = append(result, current.Identity.ParentID)
		current = r.agents[current.Identity.ParentID]
	}
	return result
}

// SetStatus updates the status of an agent.
func (r *Registry) SetStatus(id string, status AgentStatus) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.agents[id]
	if !ok {
		return false
	}

	entry.Status = status
	if status == StatusRunning && entry.StartedAt.IsZero() {
		entry.StartedAt = time.Now()
	}
	if status == StatusComplete || status == StatusFailed {
		entry.CompletedAt = time.Now()
	}
	return true
}

// SetResult sets the result of a completed agent.
func (r *Registry) SetResult(id, result string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.agents[id]
	if !ok {
		return false
	}
	entry.Result = result
	return true
}

// SetError sets the error for a failed agent.
func (r *Registry) SetError(id, errMsg string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.agents[id]
	if !ok {
		return false
	}
	entry.Error = errMsg
	return true
}

// Remove removes an agent and all its descendants from the registry.
func (r *Registry) Remove(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.agents[id]
	if !ok {
		return false
	}

	// Cannot remove main agent
	if id == r.main {
		return false
	}

	// Remove all descendants first
	var removeRecursive func(agentID string)
	removeRecursive = func(agentID string) {
		for _, childID := range r.tree[agentID] {
			removeRecursive(childID)
		}
		delete(r.agents, agentID)
		delete(r.tree, agentID)
	}
	removeRecursive(id)

	// Remove from parent's children list
	parentID := entry.Identity.ParentID
	children := r.tree[parentID]
	for i, childID := range children {
		if childID == id {
			r.tree[parentID] = append(children[:i], children[i+1:]...)
			break
		}
	}

	return true
}

// List returns copies of all agent entries.
func (r *Registry) List() []AgentEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]AgentEntry, 0, len(r.agents))
	for _, entry := range r.agents {
		result = append(result, *entry)
	}
	return result
}

// ListByStatus returns copies of all agents with the given status.
func (r *Registry) ListByStatus(status AgentStatus) []AgentEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []AgentEntry
	for _, entry := range r.agents {
		if entry.Status == status {
			result = append(result, *entry)
		}
	}
	return result
}

// Count returns the total number of agents.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.agents)
}

// Depth returns the depth of an agent in the hierarchy (0 for main).
func (r *Registry) Depth(id string) int {
	return len(r.Ancestors(id))
}
