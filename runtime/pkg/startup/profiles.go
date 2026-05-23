package startup

import (
	"fmt"
	"sort"
	"strings"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/runtime/pkg/agent"
	"github.com/quarkloop/runtime/pkg/permissions"
	"github.com/quarkloop/runtime/pkg/pluginmanager"
)

func PermissionPolicy(profile *plugin.AgentProfile) *permissions.Policy {
	if profile == nil {
		return nil
	}
	allowed := make([]string, 0, len(profile.Permissions.Tools)+len(profile.Permissions.Services))
	seen := make(map[string]struct{}, cap(allowed))
	add := func(values []string) {
		for _, value := range values {
			name := strings.TrimSpace(value)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			allowed = append(allowed, name)
		}
	}
	add(profile.Permissions.Tools)
	add(profile.Permissions.Services)
	return &permissions.Policy{RestrictTools: true, AllowedTools: allowed}
}

func AgentProfile(item pluginmanager.CatalogPlugin) agent.Profile {
	if item.AgentProfile == nil {
		return agent.Profile{SystemPrompt: item.SystemPrompt}
	}
	return agent.Profile{
		ID:             item.AgentProfile.ID,
		Name:           item.AgentProfile.Name,
		Description:    item.AgentProfile.Description,
		SystemPrompt:   item.SystemPrompt,
		HandoffTargets: append([]string(nil), item.AgentProfile.Handoff.CanDelegateTo...),
	}
}

func ResolveAgentPlugin(catalog *pluginmanager.Catalog, requested string) (pluginmanager.CatalogPlugin, error) {
	if catalog == nil || catalog.Empty() {
		return pluginmanager.CatalogPlugin{}, nil
	}
	agents := make([]pluginmanager.CatalogPlugin, 0)
	for _, item := range catalog.Plugins {
		if item.Type == plugin.TypeAgent && item.AgentProfile != nil {
			agents = append(agents, item)
		}
	}
	if len(agents) == 0 {
		return pluginmanager.CatalogPlugin{}, nil
	}
	requested = strings.TrimSpace(requested)
	if requested != "" {
		for _, item := range agents {
			if item.Name == requested || item.AgentProfile.ID == requested {
				return item, nil
			}
		}
		return pluginmanager.CatalogPlugin{}, fmt.Errorf("agent profile %q not found in supervisor-resolved catalog", requested)
	}
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].AgentProfile.ID < agents[j].AgentProfile.ID
	})
	return agents[0], nil
}

func ResolveModelSelection(profile *plugin.AgentProfile, envProvider, envModel string) (string, string) {
	if profile != nil && profile.Model.Provider != "" && profile.Model.Model != "" {
		return profile.Model.Provider, profile.Model.Model
	}
	return envProvider, envModel
}
