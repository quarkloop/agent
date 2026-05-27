package space

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

// ValidateConfig performs semantic validation of the authoritative space
// configuration persisted by the Space service.
func ValidateConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("space config is nil")
	}
	if cfg.Schema != ConfigSchema {
		return fmt.Errorf("schema must be %q", ConfigSchema)
	}
	if cfg.Name == "" {
		return fmt.Errorf("missing required field: name")
	}
	if err := ValidateName(cfg.Name); err != nil {
		return fmt.Errorf("name: %w", err)
	}
	if cfg.Version == "" {
		return fmt.Errorf("missing required field: version")
	}
	if cfg.CreatedAt.IsZero() || cfg.UpdatedAt.IsZero() {
		return fmt.Errorf("created_at and updated_at are required")
	}
	if len(cfg.Plugins) == 0 {
		return fmt.Errorf("missing required field: plugins (must have at least one)")
	}
	for i, p := range cfg.Plugins {
		if p.Ref == "" {
			return fmt.Errorf("plugins[%d]: missing ref", i)
		}
	}
	if err := validateAgents(cfg.Agents); err != nil {
		return err
	}
	if err := validateServices(cfg.Services); err != nil {
		return err
	}
	if err := validateModel(cfg.Model); err != nil {
		return err
	}
	if err := validateEnvVars(cfg.EnvironmentVariables()); err != nil {
		return err
	}
	if err := validateRoutingGatewayCapabilities(cfg.Routing, cfg.Gateway, cfg.Capabilities); err != nil {
		return err
	}
	return validatePermissionValue(cfg.Permissions)
}

func validateRoutingGatewayCapabilities(routing RoutingSection, gateway Gateway, capabilities Capabilities) error {
	for i, rule := range routing.Rules {
		if rule.Match == "" {
			return fmt.Errorf("routing.rules[%d]: missing match pattern", i)
		}
		if _, err := regexp.Compile(rule.Match); err != nil {
			return fmt.Errorf("routing.rules[%d]: invalid regex %q: %w", i, rule.Match, err)
		}
		if rule.Provider == "" || rule.Model == "" {
			return fmt.Errorf("routing.rules[%d]: missing provider or model", i)
		}
	}

	if gateway.TokenBudgetPerHour < 0 {
		return fmt.Errorf("gateway.token_budget_per_hour must be >= 0, got %d", gateway.TokenBudgetPerHour)
	}
	if capabilities.ApprovalPolicy != "" {
		switch capabilities.ApprovalPolicy {
		case "auto", "required":
		default:
			return fmt.Errorf("capabilities.approval_policy must be auto or required")
		}
	}

	return nil
}

func validateModel(model Model) error {
	if model.IsZero() {
		return nil
	}
	if model.Provider == "" {
		return fmt.Errorf("model section present but missing provider")
	}
	if model.Name == "" {
		return fmt.Errorf("model section present but missing name")
	}
	validProviders := map[string]bool{"anthropic": true, "openai": true, "openrouter": true}
	if !validProviders[model.Provider] {
		return fmt.Errorf("invalid model provider %q (supported: anthropic, openai, openrouter)", model.Provider)
	}
	return nil
}

func validateAgentModel(model AgentModelOverride) error {
	if model.IsZero() {
		return nil
	}
	if model.Provider == "" {
		return fmt.Errorf("agent model override present but missing provider")
	}
	if model.Name == "" {
		return fmt.Errorf("agent model override present but missing name")
	}
	return validateModel(Model{Provider: model.Provider, Name: model.Name, Env: model.Env})
}

func validateEnvVars(names []string) error {
	for _, name := range names {
		if err := validateEnvVarName(name, false); err != nil {
			return err
		}
	}
	return nil
}

func validateEnvVarName(name string, allowQuarkReserved bool) error {
	envName := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	if !envName.MatchString(name) {
		return fmt.Errorf("env: invalid environment variable name %q", name)
	}
	if !allowQuarkReserved && strings.HasPrefix(name, "QUARK_") {
		return fmt.Errorf("env: %s is reserved for quark runtime variables", name)
	}
	return nil
}

func validateAgents(agents []AgentRef) error {
	seen := make(map[string]bool, len(agents))
	for i, agent := range agents {
		if agent.Profile == "" {
			return fmt.Errorf("agents[%d]: missing profile", i)
		}
		if seen[agent.Profile] {
			return fmt.Errorf("agents[%d]: duplicate profile %q", i, agent.Profile)
		}
		seen[agent.Profile] = true
		if err := validateAgentModel(agent.Model); err != nil {
			return fmt.Errorf("agents[%d].model: %w", i, err)
		}
		if err := validateEnvVars(agent.Model.Env); err != nil {
			return fmt.Errorf("agents[%d].model: %w", i, err)
		}
		if err := validatePatterns(agent.Tools, fmt.Sprintf("agents[%d].tools", i)); err != nil {
			return err
		}
		if err := validatePatterns(agent.Services, fmt.Sprintf("agents[%d].services", i)); err != nil {
			return err
		}
		if agent.Approval.Policy != "" {
			switch agent.Approval.Policy {
			case "auto", "required":
			default:
				return fmt.Errorf("agents[%d].approval.policy must be auto or required", i)
			}
		}
		switch agent.Memory.Scope {
		case "", "session", "space", "none":
		default:
			return fmt.Errorf("agents[%d].memory.scope must be session, space, or none", i)
		}
	}
	return nil
}

func validatePatterns(patterns []string, field string) error {
	for i, pattern := range patterns {
		if strings.TrimSpace(pattern) == "" {
			return fmt.Errorf("%s[%d]: empty pattern", field, i)
		}
	}
	return nil
}

func validateServices(services []ServiceRef) error {
	seen := make(map[string]bool, len(services))
	for i, service := range services {
		if service.Name == "" {
			return fmt.Errorf("services[%d]: missing name", i)
		}
		if seen[service.Name] {
			return fmt.Errorf("services[%d]: duplicate service %q", i, service.Name)
		}
		seen[service.Name] = true
	}
	return nil
}

func validatePermissionValue(perms Permissions) error {
	for _, entry := range perms.Network.Deny {
		if _, _, err := net.ParseCIDR(entry); err != nil {
			if net.ParseIP(entry) == nil && !isValidHostname(entry) {
				return fmt.Errorf("permissions.network.deny: invalid CIDR, IP, or hostname %q", entry)
			}
		}
	}
	if perms.Audit.RetentionDays < 0 {
		return fmt.Errorf("permissions.audit.retention_days must be >= 0, got %d", perms.Audit.RetentionDays)
	}
	return nil
}

func isValidHostname(s string) bool {
	s = strings.TrimSuffix(s, ".")
	if len(s) == 0 || len(s) > 253 {
		return false
	}
	labelPattern := regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)
	for _, label := range strings.Split(s, ".") {
		if len(label) == 0 || len(label) > 63 {
			return false
		}
		if !labelPattern.MatchString(label) {
			return false
		}
	}
	return true
}
