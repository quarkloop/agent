package server

import (
	"testing"

	"github.com/quarkloop/pkg/plugin"
	spacemodel "github.com/quarkloop/pkg/space"
)

func TestAgentProfileOverrideResolverAppliesQuarkfileOverrides(t *testing.T) {
	enabled := true
	qf := &spacemodel.Quarkfile{
		Model:        spacemodel.Model{Provider: "anthropic", Name: "claude-sonnet-4"},
		Capabilities: spacemodel.Capabilities{ApprovalPolicy: "auto"},
		Agents: []spacemodel.AgentRef{{
			Profile:  "quark-knowledge",
			Enabled:  &enabled,
			Services: []string{"indexer_QueryContext", "embedding_Embed"},
			Tools:    []string{"fs"},
			Model:    spacemodel.AgentModelOverride{Provider: "openrouter", Name: "openai/gpt-5-mini"},
			Approval: spacemodel.AgentApprovalOverride{
				Policy:      "required",
				RequiredFor: []string{"workspace.write"},
			},
			Memory: spacemodel.AgentMemoryOverride{Scope: "session", Collections: []string{"project_facts"}},
		}},
	}
	entries := []runtimePluginCatalogEntry{agentEntry("quark-knowledge"), agentEntry("quark-devops")}

	got, selected, err := newAgentProfileOverrideResolver(qf).apply(entries)
	if err != nil {
		t.Fatalf("apply overrides: %v", err)
	}
	if selected != "quark-knowledge" {
		t.Fatalf("selected = %q", selected)
	}
	if len(got) != 1 {
		t.Fatalf("entries = %+v", got)
	}
	profile := got[0].AgentProfile
	if profile.Model.Provider != "openrouter" || profile.Model.Model != "openai/gpt-5-mini" {
		t.Fatalf("model = %+v", profile.Model)
	}
	if !sameStrings(profile.Permissions.Services, []string{"indexer_QueryContext", "embedding_Embed"}) {
		t.Fatalf("services = %+v", profile.Permissions.Services)
	}
	if profile.Approval.Policy != "required" || !sameStrings(profile.Approval.RequiredFor, []string{"workspace.write"}) {
		t.Fatalf("approval = %+v", profile.Approval)
	}
	if profile.Memory.Scope != "session" || !sameStrings(profile.Memory.Collections, []string{"project_facts"}) {
		t.Fatalf("memory = %+v", profile.Memory)
	}
}

func TestAgentProfileOverrideResolverRejectsPermissionExpansion(t *testing.T) {
	qf := &spacemodel.Quarkfile{Agents: []spacemodel.AgentRef{{
		Profile:  "quark-knowledge",
		Services: []string{"deploy.*"},
	}}}

	if _, _, err := newAgentProfileOverrideResolver(qf).apply([]runtimePluginCatalogEntry{agentEntry("quark-knowledge")}); err == nil {
		t.Fatal("permission expansion unexpectedly succeeded")
	}
}

func TestAgentProfileOverrideResolverRejectsUnknownProfile(t *testing.T) {
	qf := &spacemodel.Quarkfile{Agents: []spacemodel.AgentRef{{Profile: "missing-agent"}}}

	if _, _, err := newAgentProfileOverrideResolver(qf).apply([]runtimePluginCatalogEntry{agentEntry("quark-knowledge")}); err == nil {
		t.Fatal("unknown profile unexpectedly succeeded")
	}
}

func TestAgentProfileOverrideResolverAllowsEmptyPermissionNarrowing(t *testing.T) {
	qf := &spacemodel.Quarkfile{Agents: []spacemodel.AgentRef{{
		Profile:  "quark-knowledge",
		Services: []string{},
		Tools:    []string{},
	}}}

	got, _, err := newAgentProfileOverrideResolver(qf).apply([]runtimePluginCatalogEntry{agentEntry("quark-knowledge")})
	if err != nil {
		t.Fatalf("apply overrides: %v", err)
	}
	if len(got[0].AgentProfile.Permissions.Services) != 0 || len(got[0].AgentProfile.Permissions.Tools) != 0 {
		t.Fatalf("permissions were not narrowed to empty: %+v", got[0].AgentProfile.Permissions)
	}
}

func TestAgentProfileOverrideResolverSelectsDefaultDeterministically(t *testing.T) {
	got, selected, err := newAgentProfileOverrideResolver(&spacemodel.Quarkfile{}).apply([]runtimePluginCatalogEntry{
		agentEntry("quark-system"),
		agentEntry("quark-devops"),
	})
	if err != nil {
		t.Fatalf("apply overrides: %v", err)
	}
	if len(got) != 2 || selected != "quark-devops" {
		t.Fatalf("entries=%d selected=%q", len(got), selected)
	}
}

func agentEntry(id string) runtimePluginCatalogEntry {
	return runtimePluginCatalogEntry{
		Name: id,
		Type: plugin.TypeAgent,
		AgentProfile: &plugin.AgentProfile{
			ID:   id,
			Name: id,
			Model: plugin.AgentProfileModel{
				Provider: "anthropic",
				Model:    "claude-sonnet-4",
			},
			Permissions: plugin.AgentProfilePermission{
				Tools:    []string{"fs", "bash"},
				Services: []string{"indexer_QueryContext", "embedding_Embed", "document_ExtractText"},
			},
			Approval: plugin.AgentProfileApproval{RequiredFor: []string{"workspace.write"}},
			Memory:   plugin.AgentProfileMemory{Scope: "space", Collections: []string{"project_facts"}},
		},
	}
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
