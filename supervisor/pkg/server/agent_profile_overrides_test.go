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
			Profile: "quark-main",
			Enabled: &enabled,
		}, {
			Profile:  "quark-knowledge",
			Enabled:  &enabled,
			Services: []string{"io_Read", "indexer_QueryContext", "embedding_Embed"},
			Model:    spacemodel.AgentModelOverride{Provider: "openrouter", Name: "openai/gpt-5-mini"},
			Approval: spacemodel.AgentApprovalOverride{
				Policy:      "required",
				RequiredFor: []string{"workspace.write"},
			},
			Memory: spacemodel.AgentMemoryOverride{Scope: "session", Collections: []string{"project_facts"}},
		}},
	}
	entries := []runtimePluginCatalogEntry{mainAgentEntry("quark-main"), agentEntry("quark-knowledge"), agentEntry("quark-devops")}

	got, selected, err := newAgentProfileOverrideResolver(qf, agentValidationCatalog()).apply(entries)
	if err != nil {
		t.Fatalf("apply overrides: %v", err)
	}
	if selected != "quark-main" {
		t.Fatalf("selected = %q", selected)
	}
	if len(got) != 2 {
		t.Fatalf("entries = %+v", got)
	}
	mainProfile := findAgentProfile(t, got, "quark-main")
	if !sameStrings(mainProfile.Handoff.CanDelegateTo, []string{"quark-knowledge"}) {
		t.Fatalf("main handoff targets = %+v", mainProfile.Handoff.CanDelegateTo)
	}
	profile := findAgentProfile(t, got, "quark-knowledge")
	if profile.Model.Provider != "openrouter" || profile.Model.Model != "openai/gpt-5-mini" {
		t.Fatalf("model = %+v", profile.Model)
	}
	if !sameStrings(profile.Permissions.Services, []string{"io_Read", "indexer_QueryContext", "embedding_Embed"}) {
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
		Profile: "quark-main",
	}, {
		Profile:  "quark-knowledge",
		Services: []string{"deploy.*"},
	}}}

	if _, _, err := newAgentProfileOverrideResolver(qf, agentValidationCatalog()).apply([]runtimePluginCatalogEntry{mainAgentEntry("quark-main"), agentEntry("quark-knowledge")}); err == nil {
		t.Fatal("permission expansion unexpectedly succeeded")
	}
}

func TestAgentProfileOverrideResolverRejectsUnknownProfile(t *testing.T) {
	qf := &spacemodel.Quarkfile{Agents: []spacemodel.AgentRef{{Profile: "quark-main"}, {Profile: "missing-agent"}}}

	if _, _, err := newAgentProfileOverrideResolver(qf, agentValidationCatalog()).apply([]runtimePluginCatalogEntry{mainAgentEntry("quark-main"), agentEntry("quark-knowledge")}); err == nil {
		t.Fatal("unknown profile unexpectedly succeeded")
	}
}

func TestAgentProfileOverrideResolverAllowsEmptyPermissionNarrowing(t *testing.T) {
	qf := &spacemodel.Quarkfile{Agents: []spacemodel.AgentRef{{
		Profile: "quark-main",
	}, {
		Profile:  "quark-knowledge",
		Services: []string{},
		Tools:    []string{},
	}}}

	got, _, err := newAgentProfileOverrideResolver(qf, agentValidationCatalog()).apply([]runtimePluginCatalogEntry{mainAgentEntry("quark-main"), agentEntry("quark-knowledge")})
	if err != nil {
		t.Fatalf("apply overrides: %v", err)
	}
	profile := findAgentProfile(t, got, "quark-knowledge")
	if len(profile.Permissions.Services) != 0 || len(profile.Permissions.Tools) != 0 {
		t.Fatalf("permissions were not narrowed to empty: %+v", profile.Permissions)
	}
}

func TestAgentProfileOverrideResolverSelectsOnlyMainAgentByDefault(t *testing.T) {
	got, selected, err := newAgentProfileOverrideResolver(&spacemodel.Quarkfile{}, agentValidationCatalog("io_Read", "indexer_QueryContext")).apply([]runtimePluginCatalogEntry{
		mainAgentEntry("quark-main"),
		agentEntry("quark-system"),
		agentEntry("quark-devops"),
	})
	if err != nil {
		t.Fatalf("apply overrides: %v", err)
	}
	if len(got) != 1 || selected != "quark-main" {
		t.Fatalf("entries=%d selected=%q", len(got), selected)
	}
	if !sameStrings(got[0].AgentProfile.Permissions.Services, []string{"indexer_QueryContext", "io_Read"}) {
		t.Fatalf("main permissions = %+v", got[0].AgentProfile.Permissions.Services)
	}
	if len(got[0].AgentProfile.Handoff.CanDelegateTo) != 0 {
		t.Fatalf("disabled delegate handoff targets = %+v", got[0].AgentProfile.Handoff.CanDelegateTo)
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
				Services: []string{"io_Read", "io_Execute", "indexer_QueryContext", "embedding_Embed", "document_ExtractText"},
			},
			Approval: plugin.AgentProfileApproval{RequiredFor: []string{"workspace.write"}},
			Memory:   plugin.AgentProfileMemory{Scope: "space", Collections: []string{"project_facts"}},
		},
	}
}

func mainAgentEntry(id string) runtimePluginCatalogEntry {
	entry := agentEntry(id)
	entry.AgentProfile.Role = plugin.AgentProfileRoleMain
	entry.AgentProfile.Handoff.CanDelegateTo = []string{"quark-knowledge", "quark-devops", "quark-system"}
	return entry
}

func agentValidationCatalog(serviceFunctions ...string) agentPluginValidationCatalog {
	catalog := agentPluginValidationCatalog{
		tools:            make(map[string]struct{}),
		services:         make(map[string]struct{}),
		serviceFunctions: make(map[string]struct{}),
	}
	for _, function := range serviceFunctions {
		catalog.serviceFunctions[function] = struct{}{}
	}
	return catalog
}

func findAgentProfile(t *testing.T, entries []runtimePluginCatalogEntry, id string) *plugin.AgentProfile {
	t.Helper()
	for _, entry := range entries {
		if entry.AgentProfile != nil && entry.AgentProfile.ID == id {
			return entry.AgentProfile
		}
	}
	t.Fatalf("agent profile %q not found in %+v", id, entries)
	return nil
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
