package space_test

import (
	"strings"
	"testing"
	"time"

	"github.com/quarkloop/pkg/space"
)

func base() *space.Config {
	cfg := space.NewConfig("test-space", "/work/test-space")
	cfg.Model = space.Model{Provider: "anthropic", Name: "claude-sonnet-4.6", Env: []string{"ANTHROPIC_API_KEY"}}
	cfg.Plugins = []space.PluginRef{{Ref: "quark/service-io"}}
	return cfg
}

func TestValidateConfigValidMinimal(t *testing.T) {
	if err := space.ValidateConfig(base()); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestValidateConfigRequiredFields(t *testing.T) {
	tests := []struct {
		name string
		edit func(*space.Config)
		want string
	}{
		{name: "schema", edit: func(cfg *space.Config) { cfg.Schema = "" }, want: "schema"},
		{name: "name", edit: func(cfg *space.Config) { cfg.Name = "" }, want: "name"},
		{name: "version", edit: func(cfg *space.Config) { cfg.Version = "" }, want: "version"},
		{name: "plugins", edit: func(cfg *space.Config) { cfg.Plugins = nil }, want: "plugins"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base()
			tc.edit(cfg)
			assertInvalid(t, cfg, tc.want)
		})
	}
}

func TestValidateConfigRejectsMissingTimestamps(t *testing.T) {
	cfg := base()
	cfg.CreatedAt = time.Time{}
	assertInvalid(t, cfg, "created_at")
}

func TestValidateConfigModel(t *testing.T) {
	cfg := base()
	cfg.Model.Provider = ""
	assertInvalid(t, cfg, "model")

	cfg = base()
	cfg.Model.Name = ""
	assertInvalid(t, cfg, "model")

	cfg = base()
	cfg.Model.Provider = "made-up-provider"
	assertInvalid(t, cfg, "provider")

	for _, provider := range []string{"anthropic", "openai", "openrouter", "zhipu", "noop"} {
		cfg := base()
		cfg.Model.Provider = provider
		if err := space.ValidateConfig(cfg); err != nil {
			t.Errorf("provider %q should be valid, got: %v", provider, err)
		}
	}
}

func TestValidateConfigOptionalModel(t *testing.T) {
	cfg := base()
	cfg.Model = space.Model{}
	if err := space.ValidateConfig(cfg); err != nil {
		t.Fatalf("expected valid without model section, got: %v", err)
	}
}

func TestValidateConfigRoutingAndGateway(t *testing.T) {
	cfg := base()
	cfg.Gateway.TokenBudgetPerHour = -100
	assertInvalid(t, cfg, "token_budget_per_hour")

	cfg = base()
	cfg.Routing.Rules = []space.RoutingRuleEntry{{Match: "[invalid", Provider: "openai", Model: "gpt-5"}}
	assertInvalid(t, cfg, "regex")

	cfg = base()
	cfg.Routing.Rules = []space.RoutingRuleEntry{{Match: "code_.*"}}
	assertInvalid(t, cfg, "provider or model")

	cfg = base()
	cfg.Routing.Rules = []space.RoutingRuleEntry{{Match: "code_.*", Provider: "anthropic", Model: "claude-sonnet-4.6"}}
	if err := space.ValidateConfig(cfg); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestValidateConfigSpaceName(t *testing.T) {
	cfg := base()
	cfg.Name = "invalid name"
	assertInvalid(t, cfg, "name")
}

func TestValidateName(t *testing.T) {
	valid := []string{"test-space", "test_space", "test.space", "space123"}
	for _, name := range valid {
		if err := space.ValidateName(name); err != nil {
			t.Errorf("name %q should be valid, got: %v", name, err)
		}
	}
	invalid := []string{"", " invalid", "invalid ", ".", "..", "../bad", "bad/name", "bad name", "-bad"}
	for _, name := range invalid {
		if err := space.ValidateName(name); err == nil {
			t.Errorf("name %q should be invalid", name)
		}
	}
}

func TestMarshalAndParseConfig(t *testing.T) {
	data, err := space.MarshalConfig(space.NewConfig("123", "/work/123"))
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	cfg, err := space.ParseAndValidateConfig(data, "123")
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.Name != "123" {
		t.Fatalf("name = %q, want 123", cfg.Name)
	}
}

func TestLifecycleTransitionsReturnUpdatedCopies(t *testing.T) {
	cfg := space.NewConfig("docs", "/work/docs")
	originalCreated := cfg.CreatedAt
	now := originalCreated.Add(time.Hour)
	created := cfg.WithCreatedTimestamps(now)
	if !cfg.CreatedAt.Equal(originalCreated) || !created.CreatedAt.Equal(now) || !created.UpdatedAt.Equal(now) {
		t.Fatalf("created transition original=%s copy=%s/%s", cfg.CreatedAt, created.CreatedAt, created.UpdatedAt)
	}
	updated := created.WithUpdatedTimestamp(created.CreatedAt, now.Add(time.Hour))
	if !created.UpdatedAt.Equal(now) || !updated.CreatedAt.Equal(now) || !updated.UpdatedAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("updated transition source=%s copy=%s/%s", created.UpdatedAt, updated.CreatedAt, updated.UpdatedAt)
	}
}

func TestPluginSelectionTransitionsDoNotMutateSource(t *testing.T) {
	cfg := space.NewConfig("docs", "/work/docs")
	service := space.ServiceRef{Name: "indexer", Ref: "indexer"}
	selected := cfg.WithPluginSelection(space.PluginRef{Ref: "indexer"}, &service)
	if len(cfg.Plugins) != 1 || len(cfg.Services) != 0 {
		t.Fatalf("source was mutated: %+v", cfg)
	}
	if len(selected.Plugins) != 2 || len(selected.Services) != 1 {
		t.Fatalf("selected config = %+v", selected)
	}
	removed := selected.WithoutPluginSelection("indexer")
	if len(selected.Services) != 1 || len(removed.Plugins) != 1 || len(removed.Services) != 0 {
		t.Fatalf("removed selection source=%+v output=%+v", selected, removed)
	}
}

func TestValidateConfigEnvNames(t *testing.T) {
	cfg := base()
	cfg.Model.Env = []string{"1INVALID"}
	assertInvalid(t, cfg, "environment variable")

	cfg = base()
	cfg.Model.Env = []string{"QUARK_SPACE"}
	assertInvalid(t, cfg, "reserved")
}

func TestValidateConfigServices(t *testing.T) {
	cfg := base()
	cfg.Services = []space.ServiceRef{{Name: "indexer", Ref: "quark/service-indexer"}}
	if err := space.ValidateConfig(cfg); err != nil {
		t.Fatalf("expected valid services, got: %v", err)
	}
	cfg.Services = append(cfg.Services, space.ServiceRef{Name: "indexer"})
	assertInvalid(t, cfg, "duplicate")
}

func TestValidateConfigAgentOverrides(t *testing.T) {
	enabled := true
	cfg := base()
	cfg.Agents = []space.AgentRef{{
		Profile: "quark-knowledge", Enabled: &enabled, Services: []string{"indexer.*", "gateway_Embed"}, Tools: []string{"fs"},
		Model:    space.AgentModelOverride{Provider: "openrouter", Name: "openai/gpt-5-mini", Env: []string{"OPENROUTER_API_KEY"}},
		Approval: space.AgentApprovalOverride{Policy: "required", RequiredFor: []string{"workspace.write"}},
		Memory:   space.AgentMemoryOverride{Scope: "space", Collections: []string{"project_facts"}},
	}}
	if err := space.ValidateConfig(cfg); err != nil {
		t.Fatalf("expected valid agent override, got: %v", err)
	}
	env := cfg.EnvironmentVariables()
	if !contains(env, "ANTHROPIC_API_KEY") || !contains(env, "OPENROUTER_API_KEY") {
		t.Fatalf("agent model env was not included: %v", env)
	}

	cfg = base()
	cfg.Agents = []space.AgentRef{{Profile: "quark-knowledge"}, {Profile: "quark-knowledge"}}
	assertInvalid(t, cfg, "duplicate profile")
	cfg = base()
	cfg.Agents = []space.AgentRef{{Profile: "quark-knowledge", Approval: space.AgentApprovalOverride{Policy: "sometimes"}}}
	assertInvalid(t, cfg, "approval.policy")
	cfg = base()
	cfg.Agents = []space.AgentRef{{Profile: "quark-knowledge", Model: space.AgentModelOverride{Name: "model-without-provider"}}}
	assertInvalid(t, cfg, "missing provider")
	cfg = base()
	cfg.Agents = []space.AgentRef{{Profile: "quark-knowledge", Services: []string{""}}}
	assertInvalid(t, cfg, "empty pattern")
}

func TestValidateConfigNegativeRetentionPolicy(t *testing.T) {
	cfg := base()
	cfg.Permissions.Audit.RetentionDays = -5
	assertInvalid(t, cfg, "retention_days")
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func assertInvalid(t *testing.T, cfg *space.Config, wantSubstring string) {
	t.Helper()
	err := space.ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), wantSubstring) {
		t.Errorf("expected error containing %q, got: %v", wantSubstring, err)
	}
}
