package server

import (
	"strings"
	"testing"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
)

func TestAgentPluginValidationAcceptsConcreteInstalledContracts(t *testing.T) {
	installed := validationInstalledPlugins(
		validationService("gateway", "gateway_StreamGenerate"),
		validationService("io", "io_Read"),
		validationService("indexer", "indexer_QueryContext"),
		validationAgent("quark-knowledge", nil, []string{"io_Read", "indexer_QueryContext"}),
	)
	catalog := newAgentPluginValidationCatalog(installed)
	entries := []runtimePluginCatalogEntry{{
		Name: "quark-knowledge",
		Type: plugin.TypeAgent,
		AgentProfile: &plugin.AgentProfile{
			ID:    "quark-knowledge",
			Name:  "Quark Knowledge",
			Model: plugin.AgentProfileModel{Provider: "openrouter", Model: "openai/gpt-5-mini"},
			Permissions: plugin.AgentProfilePermission{
				Services: []string{"io_Read", "indexer_QueryContext"},
			},
		},
	}}

	if err := validateEnabledAgentPluginContracts(installed, enabledAgentPluginNames(entries), catalog); err != nil {
		t.Fatalf("validate enabled manifests: %v", err)
	}
	if err := validateRuntimeAgentProfiles(entries, catalog); err != nil {
		t.Fatalf("validate runtime profiles: %v", err)
	}
}

func TestAgentPluginValidationAllowsQuarkfileNarrowedServiceSubset(t *testing.T) {
	installed := validationInstalledPlugins(
		validationService("io", "io_Read"),
		validationAgent("quark-knowledge", nil, []string{"io_Read", "workflow_Start"}),
	)
	catalog := newAgentPluginValidationCatalog(installed)
	entries := []runtimePluginCatalogEntry{{
		Name: "quark-knowledge",
		Type: plugin.TypeAgent,
		AgentProfile: &plugin.AgentProfile{
			ID:   "quark-knowledge",
			Name: "Quark Knowledge",
			Permissions: plugin.AgentProfilePermission{
				Services: []string{"io_Read"},
			},
		},
	}}

	if err := validateEnabledAgentPluginContracts(installed, enabledAgentPluginNames(entries), catalog); err != nil {
		t.Fatalf("validate enabled manifests: %v", err)
	}
	if err := validateRuntimeAgentProfiles(entries, catalog); err != nil {
		t.Fatalf("validate narrowed profile: %v", err)
	}
}

func TestAgentPluginValidationAllowsGatewayBackedModelProviders(t *testing.T) {
	installed := validationInstalledPlugins(
		validationService("gateway", "gateway_Generate"),
	)
	catalog := newAgentPluginValidationCatalog(installed)
	entries := []runtimePluginCatalogEntry{{
		Name: "quark-knowledge",
		Type: plugin.TypeAgent,
		AgentProfile: &plugin.AgentProfile{
			ID:    "quark-knowledge",
			Name:  "Quark Knowledge",
			Model: plugin.AgentProfileModel{Provider: "openrouter", Model: "openai/gpt-5-mini"},
		},
	}}

	if err := validateRuntimeAgentProfiles(entries, catalog); err != nil {
		t.Fatalf("validate gateway-backed provider: %v", err)
	}
}

func TestAgentPluginValidationRejectsMissingServiceFunction(t *testing.T) {
	installed := validationInstalledPlugins(
		validationService("io", "io_Read"),
		validationService("indexer", "indexer_QueryContext"),
	)
	catalog := newAgentPluginValidationCatalog(installed)
	entries := []runtimePluginCatalogEntry{{
		Name: "quark-knowledge",
		Type: plugin.TypeAgent,
		AgentProfile: &plugin.AgentProfile{
			ID:   "quark-knowledge",
			Name: "Quark Knowledge",
			Permissions: plugin.AgentProfilePermission{
				Services: []string{"indexer_Missing"},
			},
		},
	}}

	err := validateRuntimeAgentProfiles(entries, catalog)
	if err == nil || !strings.Contains(err.Error(), "indexer_Missing") {
		t.Fatalf("expected missing service function error, got: %v", err)
	}
}

func TestAgentPluginValidationRejectsMissingServiceAndGateway(t *testing.T) {
	installed := validationInstalledPlugins(validationService("indexer", "indexer_QueryContext"))
	catalog := newAgentPluginValidationCatalog(installed)
	entries := []runtimePluginCatalogEntry{{
		Name: "quark-devops",
		Type: plugin.TypeAgent,
		AgentProfile: &plugin.AgentProfile{
			ID:    "quark-devops",
			Name:  "Quark DevOps",
			Model: plugin.AgentProfileModel{Provider: "openrouter", Model: "openai/gpt-5-mini"},
			Permissions: plugin.AgentProfilePermission{
				Services: []string{"io_Read", "indexer_QueryContext"},
			},
		},
	}}

	err := validateRuntimeAgentProfiles(entries, catalog)
	if err == nil || !strings.Contains(err.Error(), "openrouter") {
		t.Fatalf("expected missing gateway error first, got: %v", err)
	}

	installed = append(installed, validationService("gateway", "gateway_StreamGenerate"))
	catalog = newAgentPluginValidationCatalog(installed)
	err = validateRuntimeAgentProfiles(entries, catalog)
	if err == nil || !strings.Contains(err.Error(), "io_Read") {
		t.Fatalf("expected missing service function error, got: %v", err)
	}
}

func TestAgentPluginValidationRejectsWildcardServicePermissions(t *testing.T) {
	installed := validationInstalledPlugins(
		validationService("io", "io_Read"),
		validationService("indexer", "indexer_QueryContext"),
		validationAgent("quark-knowledge", nil, []string{"io_Read", "indexer.*"}),
	)
	catalog := newAgentPluginValidationCatalog(installed)
	entries := []runtimePluginCatalogEntry{{
		Name: "quark-knowledge",
		Type: plugin.TypeAgent,
		AgentProfile: &plugin.AgentProfile{
			ID:   "quark-knowledge",
			Name: "Quark Knowledge",
			Permissions: plugin.AgentProfilePermission{
				Services: []string{"indexer_QueryContext"},
			},
		},
	}}

	err := validateEnabledAgentPluginContracts(installed, enabledAgentPluginNames(entries), catalog)
	if err == nil || !strings.Contains(err.Error(), "concrete service function") {
		t.Fatalf("expected wildcard manifest rejection, got: %v", err)
	}

	entries[0].AgentProfile.Permissions.Services = []string{"indexer.*"}
	installed[2] = validationAgent("quark-knowledge", nil, []string{"io_Read", "indexer_QueryContext"})
	catalog = newAgentPluginValidationCatalog(installed)
	err = validateRuntimeAgentProfiles(entries, catalog)
	if err == nil || !strings.Contains(err.Error(), "concrete service function") {
		t.Fatalf("expected wildcard profile rejection, got: %v", err)
	}
}

func validationInstalledPlugins(items ...pluginmanager.InstalledPlugin) []pluginmanager.InstalledPlugin {
	return items
}

func validationService(name string, functions ...string) pluginmanager.InstalledPlugin {
	configs := make([]plugin.ServiceFunctionConfig, 0, len(functions))
	for _, function := range functions {
		configs = append(configs, plugin.ServiceFunctionConfig{Name: function})
	}
	return pluginmanager.InstalledPlugin{
		Manifest: &plugin.Manifest{
			Name: name,
			Type: plugin.TypeService,
			Service: &plugin.ServiceConfig{
				Functions: configs,
			},
		},
	}
}

func validationAgent(name string, tools, services []string) pluginmanager.InstalledPlugin {
	return pluginmanager.InstalledPlugin{
		Manifest: &plugin.Manifest{
			Name: name,
			Type: plugin.TypeAgent,
			Agent: &plugin.AgentConfig{
				Tools:    append([]string(nil), tools...),
				Services: append([]string(nil), services...),
			},
		},
	}
}
