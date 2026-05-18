package server

import (
	"strings"
	"testing"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
)

func TestAgentPluginValidationAcceptsConcreteInstalledContracts(t *testing.T) {
	installed := validationInstalledPlugins(
		validationTool("fs"),
		validationProvider("openrouter"),
		validationService("indexer", "indexer_QueryContext"),
		validationAgent("quark-knowledge", []string{"fs"}, []string{"indexer_QueryContext"}),
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
				Tools:    []string{"fs"},
				Services: []string{"indexer_QueryContext"},
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

func TestAgentPluginValidationRejectsMissingServiceFunction(t *testing.T) {
	installed := validationInstalledPlugins(
		validationTool("fs"),
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
				Tools:    []string{"fs"},
				Services: []string{"indexer_Missing"},
			},
		},
	}}

	err := validateRuntimeAgentProfiles(entries, catalog)
	if err == nil || !strings.Contains(err.Error(), "indexer_Missing") {
		t.Fatalf("expected missing service function error, got: %v", err)
	}
}

func TestAgentPluginValidationRejectsMissingToolAndProvider(t *testing.T) {
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
				Tools:    []string{"fs"},
				Services: []string{"indexer_QueryContext"},
			},
		},
	}}

	err := validateRuntimeAgentProfiles(entries, catalog)
	if err == nil || !strings.Contains(err.Error(), "openrouter") {
		t.Fatalf("expected missing provider error first, got: %v", err)
	}

	installed = append(installed, validationProvider("openrouter"))
	catalog = newAgentPluginValidationCatalog(installed)
	err = validateRuntimeAgentProfiles(entries, catalog)
	if err == nil || !strings.Contains(err.Error(), "fs") {
		t.Fatalf("expected missing tool error, got: %v", err)
	}
}

func TestAgentPluginValidationRejectsWildcardServicePermissions(t *testing.T) {
	installed := validationInstalledPlugins(
		validationTool("fs"),
		validationService("indexer", "indexer_QueryContext"),
		validationAgent("quark-knowledge", []string{"fs"}, []string{"indexer.*"}),
	)
	catalog := newAgentPluginValidationCatalog(installed)
	entries := []runtimePluginCatalogEntry{{
		Name: "quark-knowledge",
		Type: plugin.TypeAgent,
		AgentProfile: &plugin.AgentProfile{
			ID:   "quark-knowledge",
			Name: "Quark Knowledge",
			Permissions: plugin.AgentProfilePermission{
				Tools:    []string{"fs"},
				Services: []string{"indexer_QueryContext"},
			},
		},
	}}

	err := validateEnabledAgentPluginContracts(installed, enabledAgentPluginNames(entries), catalog)
	if err == nil || !strings.Contains(err.Error(), "concrete service function") {
		t.Fatalf("expected wildcard manifest rejection, got: %v", err)
	}

	entries[0].AgentProfile.Permissions.Services = []string{"indexer.*"}
	installed[2] = validationAgent("quark-knowledge", []string{"fs"}, []string{"indexer_QueryContext"})
	catalog = newAgentPluginValidationCatalog(installed)
	err = validateRuntimeAgentProfiles(entries, catalog)
	if err == nil || !strings.Contains(err.Error(), "concrete service function") {
		t.Fatalf("expected wildcard profile rejection, got: %v", err)
	}
}

func validationInstalledPlugins(items ...pluginmanager.InstalledPlugin) []pluginmanager.InstalledPlugin {
	return items
}

func validationTool(name string) pluginmanager.InstalledPlugin {
	return pluginmanager.InstalledPlugin{Manifest: &plugin.Manifest{Name: name, Type: plugin.TypeTool}}
}

func validationProvider(name string) pluginmanager.InstalledPlugin {
	return pluginmanager.InstalledPlugin{Manifest: &plugin.Manifest{Name: name, Type: plugin.TypeProvider}}
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
