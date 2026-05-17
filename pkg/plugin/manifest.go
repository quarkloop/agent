package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Manifest is the plugin declaration loaded from manifest.yaml.
type Manifest struct {
	Name        string     `yaml:"name"`
	Version     string     `yaml:"version"`
	Type        PluginType `yaml:"type"`
	Description string     `yaml:"description"`
	Author      string     `yaml:"author,omitempty"`
	License     string     `yaml:"license,omitempty"`
	Repository  string     `yaml:"repository,omitempty"`

	// Plugin loading mode
	Mode  PluginMode   `yaml:"mode"`
	Build *BuildConfig `yaml:"build,omitempty"`

	// Type-specific nested configs (only one should be set based on Type)
	Tool     *ToolConfig             `yaml:"tool,omitempty"`
	Provider *ProviderManifestConfig `yaml:"provider,omitempty"`
	Agent    *AgentConfig            `yaml:"agent,omitempty"`
	Skill    *SkillConfig            `yaml:"skill,omitempty"`
	Service  *ServiceConfig          `yaml:"service,omitempty"`
}

// BuildConfig holds build-related configuration.
type BuildConfig struct {
	LibTarget     string `yaml:"lib_target,omitempty"`      // Output .so file name
	APITarget     string `yaml:"api_target,omitempty"`      // Output binary name (api mode)
	APIEntryPoint string `yaml:"api_entry_point,omitempty"` // Main package for api mode
}

// AgentConfig holds agent-specific configuration from the manifest.
type AgentConfig struct {
	// Profile names the PROFILE.yaml file relative to the plugin directory.
	Profile string `yaml:"profile,omitempty"`
	// System names the SYSTEM.md file relative to the plugin directory.
	System string `yaml:"system,omitempty"`
	// Skill names the SKILL.md file relative to the plugin directory.
	Skill string `yaml:"skill,omitempty"`
	// Prompt is kept for compatibility with early agent manifests. New agent
	// plugins should use System and Profile.
	Prompt string `yaml:"prompt,omitempty"`
	// Tools declares required tool plugin names or patterns.
	Tools []string `yaml:"tools,omitempty"`
	// Services declares required service plugin names, functions, or patterns.
	Services []string `yaml:"services,omitempty"`
	// Skills declares required skill plugin names or files.
	Skills []string `yaml:"skills,omitempty"`
}

// SkillConfig holds skill-specific configuration from the manifest.
type SkillConfig struct {
	// Future: skill-specific config
}

// ServiceConfig declares a gRPC service exposed through the plugin catalog.
type ServiceConfig struct {
	// AddressEnv names the environment variable that contains the service
	// address. The supervisor resolves it before runtime startup.
	AddressEnv string `yaml:"address_env,omitempty"`
	// DefaultAddress is used when AddressEnv is empty or unset.
	DefaultAddress string `yaml:"default_address,omitempty"`
	// Health declares how supervisor checks service liveness/readiness.
	Health ServiceHealthConfig `yaml:"health,omitempty"`
	// Readiness declares startup readiness requirements.
	Readiness ServiceReadinessConfig `yaml:"readiness,omitempty"`
	// Skill names the service skill file relative to the plugin directory.
	Skill string `yaml:"skill,omitempty"`
	// Readme names the service README file relative to the plugin directory.
	Readme string `yaml:"readme,omitempty"`
	// ProtoServices lists protobuf service names exposed by this service.
	ProtoServices []string `yaml:"proto_services,omitempty"`
	// Functions declares the agent-facing service functions exposed by the
	// service plugin. These map to transport-level gRPC RPC methods.
	Functions []ServiceFunctionConfig `yaml:"functions,omitempty"`
}

type ServiceHealthConfig struct {
	Protocol string `yaml:"protocol,omitempty"`
	Service  string `yaml:"service,omitempty"`
	Timeout  string `yaml:"timeout,omitempty"`
}

type ServiceReadinessConfig struct {
	Required   bool   `yaml:"required,omitempty"`
	MinVersion string `yaml:"min_version,omitempty"`
}

// ServiceFunctionConfig declares one agent-facing callable service function.
type ServiceFunctionConfig struct {
	// Owner is the service-function owner. It defaults to the service plugin
	// name when omitted.
	Owner string `yaml:"owner,omitempty"`
	// Name is the generated runtime tool-call name, such as indexer_GetContext.
	Name string `yaml:"name"`
	// Service is the protobuf service name, such as quark.indexer.v1.IndexerService.
	Service string `yaml:"service"`
	// Method is the protobuf RPC method name.
	Method string `yaml:"method"`
	// Request is the protobuf request message name.
	Request string `yaml:"request"`
	// Response is the protobuf response message name.
	Response string `yaml:"response"`
	// Description is the agent-facing service function description.
	Description string `yaml:"description"`
	// RiskLevel is a coarse execution risk label: read, write, or admin.
	RiskLevel string `yaml:"risk_level,omitempty"`
	// ApprovalRequired declares whether the function needs explicit approval.
	ApprovalRequired bool `yaml:"approval_required,omitempty"`
	// ApprovalRequirements names approval reasons or scopes when approval is required.
	ApprovalRequirements []string `yaml:"approval_requirements,omitempty"`
	// Streaming declares whether the function streams responses.
	Streaming bool `yaml:"streaming,omitempty"`
	// Idempotent declares whether repeating the call with the same request is safe.
	Idempotent bool `yaml:"idempotent,omitempty"`
	// Timeout declares the execution timeout for one service-function call.
	Timeout string `yaml:"timeout,omitempty"`
	// RetryPolicy declares explicit retry behavior for transient failures.
	RetryPolicy ServiceFunctionRetryPolicy `yaml:"retry_policy,omitempty"`
	// Examples provide small protobuf JSON request examples for docs/prompts.
	Examples []ServiceFunctionExample `yaml:"examples,omitempty"`
}

type ServiceFunctionRetryPolicy struct {
	MaxAttempts          int      `yaml:"max_attempts,omitempty"`
	RetryableCodes       []string `yaml:"retryable_codes,omitempty"`
	InitialBackoffMillis int      `yaml:"initial_backoff_millis,omitempty"`
	MaxBackoffMillis     int      `yaml:"max_backoff_millis,omitempty"`
}

type ServiceFunctionExample struct {
	Name        string `yaml:"name,omitempty"`
	Description string `yaml:"description,omitempty"`
	RequestJSON string `yaml:"request_json,omitempty"`
}

// ParseManifest reads and parses a manifest.yaml file.
func ParseManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("validate manifest: %w", err)
	}

	return &m, nil
}

// Validate checks that the manifest has all required fields and valid values.
func (m *Manifest) Validate() error {
	if m.Name == "" {
		return fmt.Errorf("name is required")
	}
	if m.Version == "" {
		return fmt.Errorf("version is required")
	}
	if m.Type == "" {
		return fmt.Errorf("type is required")
	}

	// Validate type
	switch m.Type {
	case TypeTool, TypeProvider, TypeAgent, TypeSkill, TypeService:
		// valid
	default:
		return fmt.Errorf("invalid type: %s (must be tool, provider, agent, skill, or service)", m.Type)
	}

	// Validate mode
	if m.Mode == "" {
		m.Mode = ModeAPI // default to api mode for backward compatibility
	}
	switch m.Mode {
	case ModeLib, ModeAPI, ModeCLI:
		// valid
	default:
		return fmt.Errorf("invalid mode: %s (must be lib, api, or cli)", m.Mode)
	}

	// Validate type-specific config is present
	switch m.Type {
	case TypeTool:
		if m.Tool == nil {
			return fmt.Errorf("tool config is required for tool plugins")
		}
		if m.Tool.Schema.Name == "" {
			return fmt.Errorf("tool.schema.name is required")
		}
	case TypeProvider:
		if m.Provider == nil {
			return fmt.Errorf("provider config is required for provider plugins")
		}
		if m.Provider.APIBase == "" {
			return fmt.Errorf("provider.api_base is required")
		}
		if m.Provider.AuthEnv == "" {
			return fmt.Errorf("provider.auth_env is required")
		}
	case TypeAgent:
		if m.Agent == nil {
			return fmt.Errorf("agent config is required for agent plugins")
		}
		if m.Agent.Profile == "" {
			m.Agent.Profile = "PROFILE.yaml"
		}
		if m.Agent.System == "" {
			if m.Agent.Prompt != "" {
				m.Agent.System = m.Agent.Prompt
			} else {
				m.Agent.System = "SYSTEM.md"
			}
		}
		if m.Agent.Skill == "" {
			m.Agent.Skill = "SKILL.md"
		}
	case TypeSkill:
		// Skill config is optional
	case TypeService:
		if m.Service == nil {
			return fmt.Errorf("service config is required for service plugins")
		}
		if m.Service.Skill == "" {
			m.Service.Skill = "SKILL.md"
		}
		if m.Service.Readme == "" {
			m.Service.Readme = "README.md"
		}
		if m.Service.Health.Protocol == "" {
			m.Service.Health.Protocol = "grpc_health_v1"
		}
		if m.Service.Health.Timeout == "" {
			m.Service.Health.Timeout = "5s"
		}
		if m.Service.Health.Protocol != "grpc_health_v1" {
			return fmt.Errorf("service.health.protocol must be grpc_health_v1")
		}
		if _, err := time.ParseDuration(m.Service.Health.Timeout); err != nil {
			return fmt.Errorf("service.health.timeout: %w", err)
		}
		if m.Service.Readiness.MinVersion == "" {
			m.Service.Readiness.MinVersion = m.Version
		}
		if len(m.Service.Functions) == 0 {
			return fmt.Errorf("service.functions is required for service plugins")
		}
		for i, function := range m.Service.Functions {
			if err := function.Validate(); err != nil {
				return fmt.Errorf("service.functions[%d]: %w", i, err)
			}
		}
	}

	return nil
}

// Validate checks that the service function contract is complete.
func (f ServiceFunctionConfig) Validate() error {
	if f.Name == "" {
		return fmt.Errorf("name is required")
	}
	if f.Service == "" {
		return fmt.Errorf("service is required")
	}
	if f.Method == "" {
		return fmt.Errorf("method is required")
	}
	if f.Request == "" {
		return fmt.Errorf("request is required")
	}
	if f.Response == "" {
		return fmt.Errorf("response is required")
	}
	if f.Description == "" {
		return fmt.Errorf("description is required")
	}
	switch f.RiskLevel {
	case "", "read", "write", "admin":
	default:
		return fmt.Errorf("risk_level must be read, write, or admin")
	}
	if f.Timeout != "" {
		if _, err := time.ParseDuration(f.Timeout); err != nil {
			return fmt.Errorf("timeout: %w", err)
		}
	}
	if f.RetryPolicy.MaxAttempts < 0 {
		return fmt.Errorf("retry_policy.max_attempts must be non-negative")
	}
	if f.RetryPolicy.InitialBackoffMillis < 0 {
		return fmt.Errorf("retry_policy.initial_backoff_millis must be non-negative")
	}
	if f.RetryPolicy.MaxBackoffMillis < 0 {
		return fmt.Errorf("retry_policy.max_backoff_millis must be non-negative")
	}
	for i, example := range f.Examples {
		if example.Name == "" {
			return fmt.Errorf("examples[%d].name is required", i)
		}
		if example.RequestJSON == "" {
			return fmt.Errorf("examples[%d].request_json is required", i)
		}
	}
	return nil
}

// LibTargetPath returns the path to the .so file for lib-mode plugins.
func (m *Manifest) LibTargetPath(pluginDir string) string {
	target := "plugin.so"
	if m.Build != nil && m.Build.LibTarget != "" {
		target = m.Build.LibTarget
	}
	return filepath.Join(pluginDir, target)
}

// APITargetPath returns the path to the binary for api-mode plugins.
func (m *Manifest) APITargetPath(binDir string) string {
	target := m.Name
	if m.Build != nil && m.Build.APITarget != "" {
		target = m.Build.APITarget
	}
	return filepath.Join(binDir, target)
}

// APIEntryPointPath returns the main package path for api-mode builds.
func (m *Manifest) APIEntryPointPath() string {
	if m.Build != nil && m.Build.APIEntryPoint != "" {
		return m.Build.APIEntryPoint
	}
	return filepath.Join("cmd", m.Name)
}
