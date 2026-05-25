package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	plugin "github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	spacemodel "github.com/quarkloop/pkg/space"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
)

type runtimePluginCatalogEntry = plugin.RuntimeCatalogPlugin

func (s *Server) resolveRuntimePluginCatalog(ctx context.Context, space string) (plugin.RuntimeCatalog, string, error) {
	_ = ctx
	configBytes, err := s.store.Config(space)
	if err != nil {
		return plugin.RuntimeCatalog{}, "", fmt.Errorf("read space config: %w", err)
	}
	config, err := spacemodel.ParseAndValidateConfig(configBytes, space)
	if err != nil {
		return plugin.RuntimeCatalog{}, "", err
	}
	mgr, err := s.store.Plugins(space)
	if err != nil {
		return plugin.RuntimeCatalog{}, "", fmt.Errorf("open plugin store: %w", err)
	}
	installed, err := mgr.List()
	if err != nil {
		return plugin.RuntimeCatalog{}, "", fmt.Errorf("list plugins: %w", err)
	}
	validationCatalog := newAgentPluginValidationCatalog(installed)
	catalog := plugin.NewRuntimeCatalog(make([]plugin.RuntimeCatalogPlugin, 0, len(installed)))
	for _, item := range installed {
		switch item.Manifest.Type {
		case plugin.TypeTool, plugin.TypeAgent:
			entry, err := runtimePluginCatalogEntryFromInstalled(item)
			if err != nil {
				return plugin.RuntimeCatalog{}, "", fmt.Errorf("build runtime plugin catalog entry %s: %w", item.Manifest.Name, err)
			}
			catalog.Plugins = append(catalog.Plugins, entry)
		}
	}
	plugins, selectedAgent, err := newAgentProfileOverrideResolver(config, validationCatalog).apply(catalog.Plugins)
	if err != nil {
		return plugin.RuntimeCatalog{}, "", err
	}
	catalog.Plugins = plugins
	if err := validateEnabledAgentPluginContracts(installed, enabledAgentPluginNames(plugins), validationCatalog); err != nil {
		return plugin.RuntimeCatalog{}, "", err
	}
	if err := validateRuntimeAgentProfiles(plugins, validationCatalog); err != nil {
		return plugin.RuntimeCatalog{}, "", err
	}
	if err := catalog.Validate(); err != nil {
		return plugin.RuntimeCatalog{}, "", fmt.Errorf("validate runtime plugin catalog: %w", err)
	}
	return catalog, selectedAgent, nil
}

func runtimePluginCatalogEntryFromInstalled(item pluginmanager.InstalledPlugin) (runtimePluginCatalogEntry, error) {
	entry := runtimePluginCatalogEntry{
		Name:  item.Manifest.Name,
		Type:  item.Manifest.Type,
		Path:  item.Path,
		Skill: readPluginSkill(item.Path),
	}
	if item.Manifest.Tool != nil {
		schema := item.Manifest.Tool.Schema
		entry.Schema = &schema
	}
	if item.Manifest.Type == plugin.TypeAgent {
		profile, err := plugin.ParseAgentProfile(filepath.Join(item.Path, item.Manifest.Agent.Profile))
		if err != nil {
			return runtimePluginCatalogEntry{}, err
		}
		entry.AgentProfile = profile
		entry.SystemPrompt = readPluginFile(item.Path, item.Manifest.Agent.System)
		entry.Skill = readPluginFile(item.Path, item.Manifest.Agent.Skill)
	}
	return entry, nil
}

func readPluginSkill(pluginDir string) string {
	return readPluginFile(pluginDir, "SKILL.md")
}

func readPluginFile(pluginDir, name string) string {
	data, err := os.ReadFile(filepath.Join(pluginDir, name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (s *Server) runtimeServiceCatalogPayload(ctx context.Context, space string) ([]byte, error) {
	descriptors, err := s.resolveServicePluginCatalog(ctx, space)
	if err != nil {
		return nil, err
	}
	if len(descriptors) == 0 {
		return nil, nil
	}
	payload, err := servicekit.MarshalRuntimeServiceCatalog(descriptors)
	if err != nil {
		return nil, fmt.Errorf("marshal runtime service catalog: %w", err)
	}
	return payload, nil
}

func (s *Server) RuntimeCatalogSnapshot(ctx context.Context, space string) (clientcontract.RuntimeCatalogResponse, error) {
	catalog, selectedAgent, err := s.resolveRuntimePluginCatalog(ctx, space)
	if err != nil {
		return clientcontract.RuntimeCatalogResponse{}, err
	}
	pluginPayload, err := json.Marshal(catalog)
	if err != nil {
		return clientcontract.RuntimeCatalogResponse{}, fmt.Errorf("marshal runtime plugin catalog: %w", err)
	}
	servicePayload, err := s.runtimeServiceCatalogPayload(ctx, space)
	if err != nil {
		return clientcontract.RuntimeCatalogResponse{}, err
	}
	return clientcontract.RuntimeCatalogResponse{
		SpaceID:        space,
		PluginCatalog:  append(json.RawMessage(nil), pluginPayload...),
		ServiceCatalog: append(json.RawMessage(nil), servicePayload...),
		AgentProfile:   selectedAgent,
		GeneratedAt:    time.Now().UTC(),
	}, nil
}
