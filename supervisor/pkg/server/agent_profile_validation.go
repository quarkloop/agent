package server

import (
	"fmt"
	"strings"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
)

type agentPluginValidationCatalog struct {
	tools            map[string]struct{}
	providers        map[string]struct{}
	services         map[string]struct{}
	serviceFunctions map[string]struct{}
}

func newAgentPluginValidationCatalog(installed []pluginmanager.InstalledPlugin) agentPluginValidationCatalog {
	catalog := agentPluginValidationCatalog{
		tools:            make(map[string]struct{}),
		providers:        make(map[string]struct{}),
		services:         make(map[string]struct{}),
		serviceFunctions: make(map[string]struct{}),
	}
	for _, item := range installed {
		if item.Manifest == nil {
			continue
		}
		switch item.Manifest.Type {
		case plugin.TypeTool:
			catalog.tools[item.Manifest.Name] = struct{}{}
		case plugin.TypeProvider:
			catalog.providers[item.Manifest.Name] = struct{}{}
		case plugin.TypeService:
			catalog.services[item.Manifest.Name] = struct{}{}
			if item.Manifest.Service == nil {
				continue
			}
			for _, function := range item.Manifest.Service.Functions {
				catalog.serviceFunctions[function.Name] = struct{}{}
			}
		}
	}
	return catalog
}

func enabledAgentPluginNames(entries []runtimePluginCatalogEntry) map[string]struct{} {
	names := make(map[string]struct{})
	for _, entry := range entries {
		if entry.Type == plugin.TypeAgent {
			names[entry.Name] = struct{}{}
			if entry.AgentProfile != nil {
				names[entry.AgentProfile.ID] = struct{}{}
			}
		}
	}
	return names
}

func validateEnabledAgentPluginContracts(installed []pluginmanager.InstalledPlugin, enabled map[string]struct{}, catalog agentPluginValidationCatalog) error {
	for _, item := range installed {
		if item.Manifest == nil || item.Manifest.Type != plugin.TypeAgent {
			continue
		}
		if _, ok := enabled[item.Manifest.Name]; !ok {
			continue
		}
		if item.Manifest.Agent == nil {
			continue
		}
		if err := validateConcreteRefs("agent plugin "+item.Manifest.Name, "tool", item.Manifest.Agent.Tools); err != nil {
			return err
		}
		if err := validateConcreteRefs("agent plugin "+item.Manifest.Name, "service function", item.Manifest.Agent.Services); err != nil {
			return err
		}
	}
	return nil
}

func validateRuntimeAgentProfiles(entries []runtimePluginCatalogEntry, catalog agentPluginValidationCatalog) error {
	for _, entry := range entries {
		if entry.Type != plugin.TypeAgent || entry.AgentProfile == nil {
			continue
		}
		profile := entry.AgentProfile
		label := "agent profile " + profile.ID
		if err := validateProviderRef(label, profile.Model.Provider, catalog); err != nil {
			return err
		}
		if err := validateToolRefs(label, profile.Permissions.Tools, catalog); err != nil {
			return err
		}
		if err := validateServiceFunctionRefs(label, profile.Permissions.Services, catalog); err != nil {
			return err
		}
	}
	return nil
}

func validateProviderRef(owner, provider string, catalog agentPluginValidationCatalog) error {
	provider = strings.TrimSpace(provider)
	if provider == "" || provider == "noop" {
		return nil
	}
	if _, ok := catalog.providers[provider]; !ok {
		if _, hasGateway := catalog.services["gateway"]; !hasGateway {
			return fmt.Errorf("%s model provider %q is not installed and gateway service is unavailable", owner, provider)
		}
	}
	return nil
}

func validateToolRefs(owner string, refs []string, catalog agentPluginValidationCatalog) error {
	if err := validateConcreteRefs(owner, "tool", refs); err != nil {
		return err
	}
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if _, ok := catalog.tools[ref]; !ok {
			return fmt.Errorf("%s tool permission %q is not installed", owner, ref)
		}
	}
	return nil
}

func validateServiceFunctionRefs(owner string, refs []string, catalog agentPluginValidationCatalog) error {
	if err := validateConcreteRefs(owner, "service function", refs); err != nil {
		return err
	}
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if _, ok := catalog.serviceFunctions[ref]; !ok {
			return fmt.Errorf("%s service function permission %q is not provided by installed service plugins", owner, ref)
		}
	}
	return nil
}

func validateConcreteRefs(owner, kind string, refs []string) error {
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			return fmt.Errorf("%s declares an empty %s permission", owner, kind)
		}
		if strings.Contains(ref, "*") {
			return fmt.Errorf("%s %s permission %q must be a concrete %s", owner, kind, ref, kind)
		}
	}
	return nil
}
