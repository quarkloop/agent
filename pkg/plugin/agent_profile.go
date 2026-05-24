package plugin

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	AgentProfileRoleMain     = "main"
	AgentProfileRoleDelegate = "delegate"
)

// AgentProfile is the declarative runtime contract for an agent role.
type AgentProfile struct {
	ID          string                 `json:"id" yaml:"id"`
	Name        string                 `json:"name" yaml:"name"`
	Role        string                 `json:"role,omitempty" yaml:"role,omitempty"`
	Description string                 `json:"description,omitempty" yaml:"description,omitempty"`
	Model       AgentProfileModel      `json:"model,omitempty" yaml:"model,omitempty"`
	Prompt      AgentProfilePrompt     `json:"prompt,omitempty" yaml:"prompt,omitempty"`
	Permissions AgentProfilePermission `json:"permissions,omitempty" yaml:"permissions,omitempty"`
	Memory      AgentProfileMemory     `json:"memory,omitempty" yaml:"memory,omitempty"`
	Approval    AgentProfileApproval   `json:"approval,omitempty" yaml:"approval,omitempty"`
	Handoff     AgentProfileHandoff    `json:"handoff,omitempty" yaml:"handoff,omitempty"`
	Evaluation  AgentProfileEvaluation `json:"evaluation,omitempty" yaml:"evaluation,omitempty"`
}

type AgentProfileModel struct {
	Provider string `json:"provider,omitempty" yaml:"provider,omitempty"`
	Model    string `json:"model,omitempty" yaml:"model,omitempty"`
}

type AgentProfilePrompt struct {
	System string   `json:"system,omitempty" yaml:"system,omitempty"`
	Skills []string `json:"skills,omitempty" yaml:"skills,omitempty"`
}

type AgentProfilePermission struct {
	Tools    []string `json:"tools,omitempty" yaml:"tools,omitempty"`
	Services []string `json:"services,omitempty" yaml:"services,omitempty"`
}

type AgentProfileMemory struct {
	Scope       string   `json:"scope,omitempty" yaml:"scope,omitempty"`
	Collections []string `json:"collections,omitempty" yaml:"collections,omitempty"`
}

type AgentProfileApproval struct {
	Policy      string   `json:"policy,omitempty" yaml:"policy,omitempty"`
	RequiredFor []string `json:"required_for,omitempty" yaml:"required_for,omitempty"`
}

type AgentProfileHandoff struct {
	CanDelegateTo []string `json:"can_delegate_to,omitempty" yaml:"can_delegate_to,omitempty"`
}

type AgentProfileEvaluation struct {
	RequiredChecks []string `json:"required_checks,omitempty" yaml:"required_checks,omitempty"`
}

// ParseAgentProfile reads and validates a PROFILE.yaml file.
func ParseAgentProfile(path string) (*AgentProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read agent profile: %w", err)
	}
	var profile AgentProfile
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return nil, fmt.Errorf("parse agent profile: %w", err)
	}
	if err := profile.Validate(); err != nil {
		return nil, fmt.Errorf("validate agent profile: %w", err)
	}
	return &profile, nil
}

// Validate checks that the profile has enough identity to be runtime-resolved.
func (p AgentProfile) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("id is required")
	}
	if p.Name == "" {
		return fmt.Errorf("name is required")
	}
	switch strings.TrimSpace(p.Role) {
	case "", AgentProfileRoleMain, AgentProfileRoleDelegate:
		return nil
	default:
		return fmt.Errorf("role must be %q or %q", AgentProfileRoleMain, AgentProfileRoleDelegate)
	}
}

// RoleOrDefault returns the normalized role. Profiles without an explicit role
// are treated as delegates so only an explicit main profile can become root.
func (p AgentProfile) RoleOrDefault() string {
	role := strings.TrimSpace(p.Role)
	if role == "" {
		return AgentProfileRoleDelegate
	}
	return role
}

func (p AgentProfile) IsMain() bool {
	return p.RoleOrDefault() == AgentProfileRoleMain
}

// Clone returns a value copy with independent slices.
func (p AgentProfile) Clone() AgentProfile {
	p.Prompt.Skills = copyStrings(p.Prompt.Skills)
	p.Permissions.Tools = copyStrings(p.Permissions.Tools)
	p.Permissions.Services = copyStrings(p.Permissions.Services)
	p.Memory.Collections = copyStrings(p.Memory.Collections)
	p.Approval.RequiredFor = copyStrings(p.Approval.RequiredFor)
	p.Handoff.CanDelegateTo = copyStrings(p.Handoff.CanDelegateTo)
	p.Evaluation.RequiredChecks = copyStrings(p.Evaluation.RequiredChecks)
	return p
}

// WithModel returns a copy with a resolved model selection.
func (p AgentProfile) WithModel(provider, model string) AgentProfile {
	next := p.Clone()
	next.Model.Provider = provider
	next.Model.Model = model
	return next
}

// WithPermissions returns a copy with resolved maximum tool and service
// permissions.
func (p AgentProfile) WithPermissions(tools, services []string) AgentProfile {
	next := p.Clone()
	if tools != nil {
		next.Permissions.Tools = copyStrings(tools)
	}
	if services != nil {
		next.Permissions.Services = copyStrings(services)
	}
	return next
}

// WithApproval returns a copy with resolved approval policy.
func (p AgentProfile) WithApproval(policy string, requiredFor []string) AgentProfile {
	next := p.Clone()
	if policy != "" {
		next.Approval.Policy = policy
	}
	if requiredFor != nil {
		next.Approval.RequiredFor = copyStrings(requiredFor)
	}
	return next
}

// WithMemory returns a copy with resolved memory scope.
func (p AgentProfile) WithMemory(scope string, collections []string) AgentProfile {
	next := p.Clone()
	if scope != "" {
		next.Memory.Scope = scope
	}
	if collections != nil {
		next.Memory.Collections = copyStrings(collections)
	}
	return next
}

func copyStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
