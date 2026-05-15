// Package hierarchy provides agent hierarchy management for multi-agent systems.
// It supports Main Agent / Sub-Agent tree structures with work delegation.
package hierarchy

// Role represents an agent's role in the hierarchy.
type Role string

const (
	// RoleMain is the root agent that manages sub-agents.
	RoleMain Role = "main_agent"

	// RoleSub is a worker agent spawned by a main or parent agent.
	RoleSub Role = "sub_agent"
)

// String returns the string representation of the role.
func (r Role) String() string {
	return string(r)
}

// IsMain returns true if this is the main agent role.
func (r Role) IsMain() bool {
	return r == RoleMain
}

// IsSub returns true if this is a sub-agent role.
func (r Role) IsSub() bool {
	return r == RoleSub
}

// Identity holds an agent's identity information within the hierarchy.
type Identity struct {
	// ID is the unique identifier for this agent.
	ID string

	// Role is this agent's role (main_agent or sub_agent).
	Role Role

	// ParentID is the ID of the parent agent. Empty for main agents.
	ParentID string

	// Name is a human-readable name for the agent.
	Name string

	// Description describes what this agent does.
	Description string

	// ProfileID identifies the resolved agent profile this hierarchy entry uses.
	ProfileID string
}

// IsRoot returns true if this agent has no parent (is the main agent).
func (id *Identity) IsRoot() bool {
	return id.ParentID == ""
}

// NewMainIdentity creates a new main agent identity.
func NewMainIdentity(id, name, description string) *Identity {
	return &Identity{
		ID:          id,
		Role:        RoleMain,
		ParentID:    "",
		Name:        name,
		Description: description,
	}
}

// NewSubIdentity creates a new sub-agent identity.
func NewSubIdentity(id, parentID, name, description string) *Identity {
	return &Identity{
		ID:          id,
		Role:        RoleSub,
		ParentID:    parentID,
		Name:        name,
		Description: description,
	}
}

// WithProfile returns a copy of the identity with its resolved profile ID set.
func (id *Identity) WithProfile(profileID string) *Identity {
	if id == nil {
		return nil
	}
	next := *id
	next.ProfileID = profileID
	return &next
}
