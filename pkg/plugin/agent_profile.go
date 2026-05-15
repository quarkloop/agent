package plugin

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// AgentProfile is the declarative runtime contract for an agent role.
type AgentProfile struct {
	ID          string                 `json:"id" yaml:"id"`
	Name        string                 `json:"name" yaml:"name"`
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
	return nil
}
