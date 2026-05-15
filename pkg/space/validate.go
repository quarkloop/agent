package space

import (
	"fmt"
	"net"
	"regexp"
	"strings"
)

// ValidateQuarkfile performs semantic validation of qf.
func ValidateQuarkfile(qf *Quarkfile) error {
	if qf == nil {
		return fmt.Errorf("quarkfile is nil")
	}
	if qf.SchemaVersion() == "" {
		return fmt.Errorf("missing required field: quark")
	}
	if qf.Meta.Name == "" {
		return fmt.Errorf("missing required field: meta.name")
	}
	if err := ValidateName(qf.Meta.Name); err != nil {
		return fmt.Errorf("meta.name: %w", err)
	}
	if qf.Meta.Version == "" {
		return fmt.Errorf("missing required field: meta.version")
	}
	if len(qf.Plugins) == 0 {
		return fmt.Errorf("missing required field: plugins (must have at least one)")
	}
	for i, p := range qf.Plugins {
		if p.Ref == "" {
			return fmt.Errorf("plugins[%d]: missing ref", i)
		}
	}
	if err := validateAgents(qf.Agents); err != nil {
		return err
	}
	if err := validateServices(qf.Services); err != nil {
		return err
	}
	if err := validateEmbedding(qf.Embedding); err != nil {
		return err
	}

	if err := validateModel(qf.Model); err != nil {
		return err
	}
	if err := validateEnvVars(qf.EnvironmentVariables()); err != nil {
		return err
	}

	for i, rule := range qf.Routing.Rules {
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

	if qf.Gateway.TokenBudgetPerHour < 0 {
		return fmt.Errorf("gateway.token_budget_per_hour must be >= 0, got %d", qf.Gateway.TokenBudgetPerHour)
	}
	if qf.Capabilities.ApprovalPolicy != "" {
		switch qf.Capabilities.ApprovalPolicy {
		case "auto", "required":
		default:
			return fmt.Errorf("capabilities.approval_policy must be auto or required")
		}
	}

	return validatePermissions(qf)
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
	validProviders := map[string]bool{"anthropic": true, "openai": true, "openrouter": true, "zhipu": true, "noop": true}
	if !validProviders[model.Provider] {
		return fmt.Errorf("invalid model provider %q (supported: anthropic, openai, openrouter, zhipu, noop)", model.Provider)
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
		switch service.Mode {
		case "", "local", "online":
		default:
			return fmt.Errorf("services[%d]: mode must be local or online", i)
		}
		if service.AddressEnv != "" {
			if err := validateEnvVarName(service.AddressEnv, true); err != nil {
				return fmt.Errorf("services[%d].address_env: %w", i, err)
			}
		}
	}
	return nil
}

func validateEmbedding(embedding EmbeddingRef) error {
	if embedding.IsZero() {
		return nil
	}
	if embedding.Dimensions < 0 {
		return fmt.Errorf("embedding.dimensions must be >= 0, got %d", embedding.Dimensions)
	}
	if embedding.EndpointEnv != "" {
		if err := validateEnvVarName(embedding.EndpointEnv, true); err != nil {
			return fmt.Errorf("embedding.endpoint_env: %w", err)
		}
	}
	return nil
}

func validatePermissions(qf *Quarkfile) error {
	perms := qf.Permissions
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
