package space

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

//go:embed Quarkfile.tmpl
var quarkfileTmpl []byte

// Quarkfile is the parsed representation of a space Quarkfile.
type Quarkfile struct {
	Quark        string         `yaml:"quark"`
	Meta         Meta           `yaml:"meta"`
	Model        Model          `yaml:"model,omitempty"`
	Routing      RoutingSection `yaml:"routing,omitempty"`
	Plugins      []PluginRef    `yaml:"plugins"`
	Agents       []AgentRef     `yaml:"agents,omitempty"`
	Services     []ServiceRef   `yaml:"services,omitempty"`
	Permissions  Permissions    `yaml:"permissions,omitempty"`
	Capabilities Capabilities   `yaml:"capabilities,omitempty"`
	Gateway      Gateway        `yaml:"gateway,omitempty"`
}

// Meta holds human-readable identity for the space.
type Meta struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description,omitempty"`
	Version     string            `yaml:"version,omitempty"`
	Author      string            `yaml:"author,omitempty"`
	Labels      map[string]string `yaml:"labels,omitempty"`
}

// Model specifies the LLM provider and model name.
type Model struct {
	Provider string   `yaml:"provider"`
	Name     string   `yaml:"name"`
	Env      []string `yaml:"env,omitempty"`
}

// RoutingSection configures model routing rules and fallback chain.
type RoutingSection struct {
	Rules    []RoutingRuleEntry `yaml:"rules,omitempty"`
	Fallback []ModelRef         `yaml:"fallback,omitempty"`
}

// RoutingRuleEntry maps a regex pattern to a target model.
type RoutingRuleEntry struct {
	Match    string `yaml:"match"`
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
}

// ModelRef specifies a provider and model name.
type ModelRef struct {
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
}

// PluginRef declares a plugin dependency with optional per-plugin config.
type PluginRef struct {
	Ref    string         `yaml:"ref"`
	Config map[string]any `yaml:"config,omitempty"`
}

// AgentRef declares space-level overrides for an installed agent profile.
//
// Quarkfile overrides are intentionally partial. Supervisor applies them over
// the installed agent plugin profile and rejects overrides that exceed profile
// maximum permissions.
type AgentRef struct {
	Profile  string                `yaml:"profile"`
	Enabled  *bool                 `yaml:"enabled,omitempty"`
	Model    AgentModelOverride    `yaml:"model,omitempty"`
	Services []string              `yaml:"services,omitempty"`
	Tools    []string              `yaml:"tools,omitempty"`
	Approval AgentApprovalOverride `yaml:"approval,omitempty"`
	Memory   AgentMemoryOverride   `yaml:"memory,omitempty"`
}

// AgentModelOverride narrows or replaces the default LLM selection for one
// agent profile.
type AgentModelOverride struct {
	Provider string   `yaml:"provider,omitempty"`
	Name     string   `yaml:"name,omitempty"`
	Env      []string `yaml:"env,omitempty"`
}

func (m AgentModelOverride) IsZero() bool {
	return m.Provider == "" && m.Name == "" && len(m.Env) == 0
}

// AgentApprovalOverride adjusts approval policy for one agent profile.
type AgentApprovalOverride struct {
	Policy      string   `yaml:"policy,omitempty"`
	RequiredFor []string `yaml:"required_for,omitempty"`
}

// AgentMemoryOverride narrows the memory scope or collections visible to one
// agent profile.
type AgentMemoryOverride struct {
	Scope       string   `yaml:"scope,omitempty"`
	Collections []string `yaml:"collections,omitempty"`
}

// ServiceRef declares service intent. Supervisor resolves the concrete
// endpoint and passes a normalized catalog to runtime.
type ServiceRef struct {
	Name       string         `yaml:"name"`
	Ref        string         `yaml:"ref,omitempty"`
	Mode       string         `yaml:"mode,omitempty"` // local | online
	Address    string         `yaml:"address,omitempty"`
	AddressEnv string         `yaml:"address_env,omitempty"`
	Config     map[string]any `yaml:"config,omitempty"`
}

// Permissions defines policy constraints enforced by the agent runtime.
type Permissions struct {
	Filesystem FilesystemPermissions `yaml:"filesystem,omitempty"`
	Network    NetworkPermissions    `yaml:"network,omitempty"`
	Tools      ToolPermissions       `yaml:"tools,omitempty"`
	Audit      AuditPermissions      `yaml:"audit,omitempty"`
}

// FilesystemPermissions controls which paths the agent can access.
type FilesystemPermissions struct {
	AllowedPaths []string `yaml:"allowed_paths,omitempty"`
	ReadOnly     []string `yaml:"read_only,omitempty"`
}

// NetworkPermissions controls which hosts the agent can reach.
type NetworkPermissions struct {
	AllowedHosts []string `yaml:"allowed_hosts,omitempty"`
	Deny         []string `yaml:"deny,omitempty"`
}

// ToolPermissions controls which tools the agent can invoke.
type ToolPermissions struct {
	Allowed []string `yaml:"allowed,omitempty"`
	Denied  []string `yaml:"denied,omitempty"`
}

// AuditPermissions controls logging and retention policies.
type AuditPermissions struct {
	LogToolCalls    bool `yaml:"log_tool_calls"`
	LogLLMResponses bool `yaml:"log_llm_responses"`
	RetentionDays   int  `yaml:"retention_days,omitempty"`
}

// Capabilities declares what agents in this space are allowed to do.
type Capabilities struct {
	SpawnAgents    bool   `yaml:"spawn_agents"`
	MaxWorkers     int    `yaml:"max_workers,omitempty"`
	CreatePlans    bool   `yaml:"create_plans"`
	ApprovalPolicy string `yaml:"approval_policy,omitempty"`
}

// Gateway configures model gateway resource limits.
type Gateway struct {
	TokenBudgetPerHour int `yaml:"token_budget_per_hour,omitempty"`
}

// ReadQuarkfile reads raw Quarkfile bytes from dir.
func ReadQuarkfile(dir string) ([]byte, error) {
	path := filepath.Join(dir, QuarkfileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Quarkfile: %w", err)
	}
	return data, nil
}

// WriteQuarkfile writes raw Quarkfile bytes into dir.
func WriteQuarkfile(dir string, data []byte) error {
	path := filepath.Join(dir, QuarkfileName)
	return WriteQuarkfileFile(path, data)
}

// ReadQuarkfileFile reads raw Quarkfile bytes from path.
func ReadQuarkfileFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Quarkfile: %w", err)
	}
	return data, nil
}

// WriteQuarkfileFile atomically writes raw Quarkfile bytes to path.
func WriteQuarkfileFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create Quarkfile dir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write Quarkfile: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename Quarkfile: %w", err)
	}
	return nil
}

// ParseQuarkfile unmarshals raw Quarkfile bytes.
func ParseQuarkfile(data []byte) (*Quarkfile, error) {
	var q Quarkfile
	if err := yaml.Unmarshal(data, &q); err != nil {
		return nil, fmt.Errorf("parse Quarkfile: %w", err)
	}
	return &q, nil
}

// ParseAndValidateQuarkfile parses and validates raw Quarkfile bytes.
func ParseAndValidateQuarkfile(data []byte) (*Quarkfile, error) {
	qf, err := ParseQuarkfile(data)
	if err != nil {
		return nil, err
	}
	if err := ValidateQuarkfile(qf); err != nil {
		return nil, fmt.Errorf("invalid Quarkfile: %w", err)
	}
	return qf, nil
}

// ParseAndValidateQuarkfileForSpace validates that the Quarkfile belongs to
// spaceName.
func ParseAndValidateQuarkfileForSpace(data []byte, spaceName string) (*Quarkfile, error) {
	qf, err := ParseAndValidateQuarkfile(data)
	if err != nil {
		return nil, err
	}
	if qf.Meta.Name != spaceName {
		return nil, fmt.Errorf("Quarkfile meta.name %q does not match space name %q", qf.Meta.Name, spaceName)
	}
	return qf, nil
}

// NameFromQuarkfile extracts meta.name from raw Quarkfile bytes.
func NameFromQuarkfile(data []byte) (string, error) {
	q, err := ParseQuarkfile(data)
	if err != nil {
		return "", err
	}
	if q.Meta.Name == "" {
		return "", fmt.Errorf("Quarkfile missing meta.name")
	}
	if err := ValidateName(q.Meta.Name); err != nil {
		return "", fmt.Errorf("Quarkfile meta.name: %w", err)
	}
	return q.Meta.Name, nil
}

// NameFromDir reads the Quarkfile in dir and returns meta.name.
func NameFromDir(dir string) (string, error) {
	data, err := ReadQuarkfile(dir)
	if err != nil {
		return "", err
	}
	return NameFromQuarkfile(data)
}

// CurrentName returns the space name from the Quarkfile in the current working
// directory.
func CurrentName() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	return NameFromDir(cwd)
}

// DefaultQuarkfile returns default Quarkfile contents for a new space.
func DefaultQuarkfile(name string) []byte {
	return fmt.Appendf(nil, string(quarkfileTmpl), name)
}

// SchemaVersion returns the declared Quarkfile schema version.
func (qf *Quarkfile) SchemaVersion() string {
	if qf == nil {
		return ""
	}
	return qf.Quark
}

// IsZero reports whether the model declaration is empty.
func (m Model) IsZero() bool {
	return m.Provider == "" && m.Name == "" && len(m.Env) == 0
}

// DefaultModel returns the configured model.
func (qf *Quarkfile) DefaultModel() (Model, bool) {
	if qf == nil {
		return Model{}, false
	}
	if !qf.Model.IsZero() {
		return qf.Model, true
	}
	return Model{}, false
}

// EnvironmentVariables returns env var names declared by Quarkfile model
// sections. Runtime-owned QUARK_* variables are validated separately.
func (qf *Quarkfile) EnvironmentVariables() []string {
	if qf == nil {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(qf.Model.Env))
	add := func(names []string) {
		for _, name := range names {
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, name)
		}
	}
	add(qf.Model.Env)
	for _, agent := range qf.Agents {
		add(agent.Model.Env)
	}
	return out
}
