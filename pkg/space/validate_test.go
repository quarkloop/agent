package space_test

import (
	"strings"
	"testing"

	"github.com/quarkloop/pkg/space"
)

func base() *space.Quarkfile {
	return &space.Quarkfile{
		Quark:   "1.0",
		Meta:    space.Meta{Name: "test-space", Version: "0.1.0"},
		Model:   space.Model{Provider: "anthropic", Name: "claude-sonnet-4.6", Env: []string{"ANTHROPIC_API_KEY"}},
		Plugins: []space.PluginRef{{Ref: "quark/service-io"}},
	}
}

func TestValidateQuarkfileValidMinimal(t *testing.T) {
	if err := space.ValidateQuarkfile(base()); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestValidateQuarkfileRequiredFields(t *testing.T) {
	tests := []struct {
		name string
		edit func(*space.Quarkfile)
		want string
	}{
		{name: "quark", edit: func(qf *space.Quarkfile) { qf.Quark = "" }, want: "quark"},
		{name: "meta name", edit: func(qf *space.Quarkfile) { qf.Meta.Name = "" }, want: "meta.name"},
		{name: "meta version", edit: func(qf *space.Quarkfile) { qf.Meta.Version = "" }, want: "meta.version"},
		{name: "plugins", edit: func(qf *space.Quarkfile) { qf.Plugins = nil }, want: "plugins"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			qf := base()
			tc.edit(qf)
			assertInvalid(t, qf, tc.want)
		})
	}
}

func TestValidateQuarkfileModel(t *testing.T) {
	qf := base()
	qf.Model.Provider = ""
	assertInvalid(t, qf, "model")

	qf = base()
	qf.Model.Name = ""
	assertInvalid(t, qf, "model")

	qf = base()
	qf.Model.Provider = "made-up-provider"
	assertInvalid(t, qf, "provider")

	for _, p := range []string{"anthropic", "openai", "openrouter", "zhipu", "noop"} {
		qf := base()
		qf.Model.Provider = p
		if err := space.ValidateQuarkfile(qf); err != nil {
			t.Errorf("provider %q should be valid, got: %v", p, err)
		}
	}
}

func TestValidateQuarkfileOptionalModel(t *testing.T) {
	qf := base()
	qf.Model = space.Model{}
	if err := space.ValidateQuarkfile(qf); err != nil {
		t.Fatalf("expected valid without model section, got: %v", err)
	}
}

func TestValidateQuarkfileRoutingAndGateway(t *testing.T) {
	qf := base()
	qf.Gateway.TokenBudgetPerHour = -100
	assertInvalid(t, qf, "token_budget_per_hour")

	qf = base()
	qf.Routing.Rules = []space.RoutingRuleEntry{{Match: "[invalid", Provider: "openai", Model: "gpt-5"}}
	assertInvalid(t, qf, "regex")

	qf = base()
	qf.Routing.Rules = []space.RoutingRuleEntry{{Match: "code_.*"}}
	assertInvalid(t, qf, "provider or model")

	qf = base()
	qf.Routing.Rules = []space.RoutingRuleEntry{{Match: "code_.*", Provider: "anthropic", Model: "claude-sonnet-4.6"}}
	if err := space.ValidateQuarkfile(qf); err != nil {
		t.Fatalf("expected valid, got: %v", err)
	}
}

func TestValidateQuarkfileSpaceName(t *testing.T) {
	qf := base()
	qf.Meta.Name = "invalid name"
	assertInvalid(t, qf, "meta.name")
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

func TestDefaultQuarkfileQuotesName(t *testing.T) {
	qf, err := space.ParseAndValidateQuarkfile(space.DefaultQuarkfile("123"))
	if err != nil {
		t.Fatalf("default Quarkfile should parse, got: %v", err)
	}
	if qf.Meta.Name != "123" {
		t.Fatalf("meta.name = %q, want 123", qf.Meta.Name)
	}
}

func TestValidateQuarkfileEnvNames(t *testing.T) {
	qf := base()
	qf.Model.Env = []string{"1INVALID"}
	assertInvalid(t, qf, "environment variable")

	qf = base()
	qf.Model.Env = []string{"QUARK_SPACE"}
	assertInvalid(t, qf, "reserved")
}

func TestValidateQuarkfileServices(t *testing.T) {
	qf := base()
	qf.Services = []space.ServiceRef{{
		Name:       "indexer",
		Ref:        "quark/service-indexer",
		Mode:       "local",
		AddressEnv: "QUARK_INDEXER_ADDR",
	}}
	if err := space.ValidateQuarkfile(qf); err != nil {
		t.Fatalf("expected valid services, got: %v", err)
	}

	qf.Services[0].Mode = "sideways"
	assertInvalid(t, qf, "mode")

}

func TestValidateQuarkfileAgentOverrides(t *testing.T) {
	enabled := true
	qf := base()
	qf.Agents = []space.AgentRef{{
		Profile:  "quark-knowledge",
		Enabled:  &enabled,
		Services: []string{"indexer.*", "gateway_Embed"},
		Tools:    []string{"fs"},
		Model: space.AgentModelOverride{
			Provider: "openrouter",
			Name:     "openai/gpt-5-mini",
			Env:      []string{"OPENROUTER_API_KEY"},
		},
		Approval: space.AgentApprovalOverride{
			Policy:      "required",
			RequiredFor: []string{"workspace.write"},
		},
		Memory: space.AgentMemoryOverride{Scope: "space", Collections: []string{"project_facts"}},
	}}
	if err := space.ValidateQuarkfile(qf); err != nil {
		t.Fatalf("expected valid agent override, got: %v", err)
	}
	env := qf.EnvironmentVariables()
	if !contains(env, "ANTHROPIC_API_KEY") || !contains(env, "OPENROUTER_API_KEY") {
		t.Fatalf("agent model env was not included: %v", env)
	}

	qf = base()
	qf.Agents = []space.AgentRef{{Profile: "quark-knowledge"}, {Profile: "quark-knowledge"}}
	assertInvalid(t, qf, "duplicate profile")

	qf = base()
	qf.Agents = []space.AgentRef{{Profile: "quark-knowledge", Approval: space.AgentApprovalOverride{Policy: "sometimes"}}}
	assertInvalid(t, qf, "approval.policy")

	qf = base()
	qf.Agents = []space.AgentRef{{Profile: "quark-knowledge", Model: space.AgentModelOverride{Name: "model-without-provider"}}}
	assertInvalid(t, qf, "missing provider")

	qf = base()
	qf.Agents = []space.AgentRef{{Profile: "quark-knowledge", Services: []string{""}}}
	assertInvalid(t, qf, "empty pattern")
}

func TestValidateQuarkfileNegativeRetentionPolicy(t *testing.T) {
	qf := base()
	qf.Permissions.Audit.RetentionDays = -5
	assertInvalid(t, qf, "retention_days")
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func assertInvalid(t *testing.T, qf *space.Quarkfile, wantSubstring string) {
	t.Helper()
	err := space.ValidateQuarkfile(qf)
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), wantSubstring) {
		t.Errorf("expected error containing %q, got: %v", wantSubstring, err)
	}
}
