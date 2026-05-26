package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

	// Plugin loading mode applies only to executable tool plugins.
	Mode  PluginMode   `yaml:"mode"`
	Build *BuildConfig `yaml:"build,omitempty"`

	// Type-specific nested configs (only one should be set based on Type)
	Tool    *ToolConfig    `yaml:"tool,omitempty"`
	Agent   *AgentConfig   `yaml:"agent,omitempty"`
	Skill   *SkillConfig   `yaml:"skill,omitempty"`
	Service *ServiceConfig `yaml:"service,omitempty"`
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

// ServiceConfig declares a NATS-native service exposed through the plugin catalog.
type ServiceConfig struct {
	// Transport declares the service-function transport. vNext supports NATS.
	Transport string `yaml:"transport,omitempty"`
	// SubjectPrefix is the NATS subject prefix for this service, such as
	// svc.indexer.v1.
	SubjectPrefix string `yaml:"subject_prefix,omitempty"`
	// QueueGroup is the queue group used by service workers.
	QueueGroup string `yaml:"queue_group,omitempty"`
	// Health declares how supervisor checks service liveness/readiness.
	Health ServiceHealthConfig `yaml:"health,omitempty"`
	// Readiness declares startup readiness requirements.
	Readiness ServiceReadinessConfig `yaml:"readiness,omitempty"`
	// Skill names the service skill file relative to the plugin directory.
	Skill string `yaml:"skill,omitempty"`
	// Readme names the service README file relative to the plugin directory.
	Readme string `yaml:"readme,omitempty"`
	// ProtoServices lists protobuf service names that define request/response
	// schemas for this service.
	ProtoServices []string `yaml:"proto_services,omitempty"`
	// Functions declares the agent-facing service functions exposed by the
	// service plugin.
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
	// Name is the generated runtime tool-call name, such as indexer_QueryContext.
	Name string `yaml:"name"`
	// Subject is the NATS service-function request subject.
	Subject string `yaml:"subject,omitempty"`
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
	case TypeTool, TypeAgent, TypeSkill, TypeService:
		// valid
	default:
		return fmt.Errorf("invalid type: %s (must be tool, agent, skill, or service)", m.Type)
	}

	// Validate type-specific config is present
	switch m.Type {
	case TypeTool:
		if m.Mode == "" {
			return fmt.Errorf("mode is required for tool plugins")
		}
		switch m.Mode {
		case ModeLib, ModeAPI, ModeCLI:
			// valid executable tool modes
		default:
			return fmt.Errorf("invalid tool mode: %s (must be lib, api, or cli)", m.Mode)
		}
		if m.Tool == nil {
			return fmt.Errorf("tool config is required for tool plugins")
		}
		if m.Tool.Schema.Name == "" {
			return fmt.Errorf("tool.schema.name is required")
		}
	case TypeAgent:
		if m.Mode != "" {
			return fmt.Errorf("agent plugins must not declare an execution mode")
		}
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
		if m.Mode != "" {
			return fmt.Errorf("skill plugins must not declare an execution mode")
		}
		// Skill config is optional
	case TypeService:
		if m.Mode != "" {
			return fmt.Errorf("service plugins must not declare an execution mode")
		}
		if m.Service == nil {
			return fmt.Errorf("service config is required for service plugins")
		}
		if m.Service.Transport == "" {
			m.Service.Transport = "nats"
		}
		if m.Service.Transport != "nats" {
			return fmt.Errorf("service.transport must be nats")
		}
		if m.Service.SubjectPrefix == "" {
			m.Service.SubjectPrefix = fmt.Sprintf("svc.%s.v1", manifestSubjectToken(m.Name))
		}
		if err := validateServiceSubjectPrefix(m.Service.SubjectPrefix); err != nil {
			return fmt.Errorf("service.subject_prefix: %w", err)
		}
		expectedQueueGroup := fmt.Sprintf("q.service.v1.%s", manifestSubjectToken(m.Name))
		if m.Service.QueueGroup == "" {
			m.Service.QueueGroup = expectedQueueGroup
		}
		if err := validateQueueGroup(m.Service.QueueGroup); err != nil {
			return fmt.Errorf("service.queue_group: %w", err)
		}
		if m.Service.QueueGroup != expectedQueueGroup {
			return fmt.Errorf("service.queue_group %q must match responder group %q", m.Service.QueueGroup, expectedQueueGroup)
		}
		if m.Service.Skill == "" {
			m.Service.Skill = "SKILL.md"
		}
		if m.Service.Readme == "" {
			m.Service.Readme = "README.md"
		}
		if m.Service.Health.Protocol == "" {
			m.Service.Health.Protocol = "nats_service"
		}
		if m.Service.Health.Timeout == "" {
			m.Service.Health.Timeout = "5s"
		}
		if m.Service.Health.Protocol != "nats_service" {
			return fmt.Errorf("service.health.protocol must be nats_service")
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
		for i := range m.Service.Functions {
			if m.Service.Functions[i].Owner == "" {
				m.Service.Functions[i].Owner = m.Name
			}
			if m.Service.Functions[i].Subject == "" {
				subject, err := serviceFunctionSubject(m.Service.Functions[i].Owner, m.Service.Functions[i].Name)
				if err != nil {
					return fmt.Errorf("service.functions[%d].subject: %w", i, err)
				}
				m.Service.Functions[i].Subject = subject
			}
			if err := m.Service.Functions[i].Validate(); err != nil {
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
	if f.Subject != "" {
		if err := validateServiceFunctionSubject(f.Subject); err != nil {
			return fmt.Errorf("subject: %w", err)
		}
		if expected, err := serviceFunctionSubject(firstNonEmpty(f.Owner, serviceOwnerFromName(f.Name)), f.Name); err == nil && f.Subject != expected {
			return fmt.Errorf("subject %q does not match expected %q", f.Subject, expected)
		}
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

func serviceFunctionSubject(owner, functionName string) (string, error) {
	ownerToken := manifestSubjectToken(owner)
	function := functionToken(owner, functionName)
	if ownerToken == "" || function == "" {
		return "", fmt.Errorf("owner and function name are required")
	}
	return "svc." + ownerToken + ".v1." + function, nil
}

func serviceOwnerFromName(functionName string) string {
	owner, _, ok := strings.Cut(functionName, "_")
	if !ok {
		return ""
	}
	return owner
}

func functionToken(owner, functionName string) string {
	ownerToken := manifestSubjectToken(owner)
	function := strings.TrimSpace(functionName)
	for _, prefix := range []string{strings.TrimSpace(owner) + "_", ownerToken + "_"} {
		if prefix != "_" && strings.HasPrefix(function, prefix) {
			function = strings.TrimPrefix(function, prefix)
			break
		}
	}
	return manifestSubjectToken(function)
}

func validateServiceSubjectPrefix(subject string) error {
	parts := strings.Split(subject, ".")
	if len(parts) != 3 || parts[0] != "svc" || parts[2] != "v1" {
		return fmt.Errorf("%q must match svc.<service>.v1", subject)
	}
	if parts[1] == "" || parts[1] != manifestSubjectToken(parts[1]) {
		return fmt.Errorf("%q has invalid service token", subject)
	}
	return nil
}

func validateServiceFunctionSubject(subject string) error {
	parts := strings.Split(subject, ".")
	if len(parts) != 4 || parts[0] != "svc" || parts[2] != "v1" {
		return fmt.Errorf("%q must match svc.<service>.v1.<function>", subject)
	}
	for _, part := range []string{parts[1], parts[3]} {
		if part == "" || part != manifestSubjectToken(part) {
			return fmt.Errorf("%q contains invalid subject token %q", subject, part)
		}
	}
	return nil
}

func validateQueueGroup(queue string) error {
	queue = strings.TrimSpace(queue)
	if queue == "" {
		return fmt.Errorf("queue group is required")
	}
	for _, part := range strings.Split(queue, ".") {
		if part == "" || part != manifestSubjectToken(part) {
			return fmt.Errorf("queue group %q contains invalid token %q", queue, part)
		}
	}
	return nil
}

func manifestSubjectToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	prevLowerOrDigit := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastUnderscore = false
			prevLowerOrDigit = true
		case r >= 'A' && r <= 'Z':
			if prevLowerOrDigit && !lastUnderscore && b.Len() > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r + ('a' - 'A'))
			lastUnderscore = false
			prevLowerOrDigit = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
			prevLowerOrDigit = true
		case r == '_' || r == '-' || r == '.':
			if !lastUnderscore && b.Len() > 0 {
				b.WriteByte('_')
				lastUnderscore = true
			}
			prevLowerOrDigit = false
		}
	}
	return strings.Trim(b.String(), "_")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
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
