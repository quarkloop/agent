package server

import (
	"context"
	"fmt"

	plugin "github.com/quarkloop/pkg/plugin"
	spacemodel "github.com/quarkloop/pkg/space"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
)

func (s *Server) selectedPlugins(spaceID string) ([]pluginmanager.InstalledPlugin, error) {
	data, err := s.store.Config(spaceID)
	if err != nil {
		return nil, err
	}
	config, err := spacemodel.ParseAndValidateConfig(data, spaceID)
	if err != nil {
		return nil, err
	}
	selected := make([]pluginmanager.InstalledPlugin, 0, len(config.Plugins)+1)
	seen := make(map[string]struct{}, len(config.Plugins)+1)
	add := func(ref string) error {
		item, err := s.pluginRegistry.Get(ref)
		if err != nil {
			return fmt.Errorf("resolve selected plugin %q: %w", ref, err)
		}
		if _, ok := seen[item.Manifest.Name]; ok {
			return nil
		}
		seen[item.Manifest.Name] = struct{}{}
		selected = append(selected, item)
		return nil
	}
	if err := add("quark-main"); err != nil {
		return nil, fmt.Errorf("required main agent: %w", err)
	}
	for _, ref := range config.Plugins {
		if err := add(ref.Ref); err != nil {
			return nil, err
		}
	}
	for _, ref := range config.Services {
		if err := add(ref.Name); err != nil {
			return nil, err
		}
	}
	return selected, nil
}

func (s *Server) ListSpacePlugins(_ context.Context, spaceID, typeFilter string) ([]pluginmanager.InstalledPlugin, error) {
	selected, err := s.selectedPlugins(spaceID)
	if err != nil {
		return nil, err
	}
	if typeFilter == "" {
		return selected, nil
	}
	filter := plugin.PluginType(typeFilter)
	out := make([]pluginmanager.InstalledPlugin, 0, len(selected))
	for _, item := range selected {
		if item.Manifest.Type == filter {
			out = append(out, item)
		}
	}
	return out, nil
}

func (s *Server) GetSpacePlugin(ctx context.Context, spaceID, name string) (pluginmanager.InstalledPlugin, error) {
	selected, err := s.ListSpacePlugins(ctx, spaceID, "")
	if err != nil {
		return pluginmanager.InstalledPlugin{}, err
	}
	for _, item := range selected {
		if item.Manifest.Name == name {
			return item, nil
		}
	}
	return pluginmanager.InstalledPlugin{}, fmt.Errorf("plugin %q is not selected in space %q", name, spaceID)
}

func (s *Server) InstallSpacePlugin(ctx context.Context, spaceID, ref string) (pluginmanager.InstalledPlugin, error) {
	item, err := s.pluginRegistry.Install(ctx, ref)
	if err != nil {
		return pluginmanager.InstalledPlugin{}, err
	}
	s.pluginSelectionMu.Lock()
	defer s.pluginSelectionMu.Unlock()
	config, err := s.readSpaceConfig(spaceID)
	if err != nil {
		return pluginmanager.InstalledPlugin{}, err
	}
	var serviceRef *spacemodel.ServiceRef
	if item.Manifest.Type == plugin.TypeService && !containsServiceRef(config.Services, item.Manifest.Name) {
		serviceRef = &spacemodel.ServiceRef{Name: item.Manifest.Name, Ref: item.Manifest.Name}
	}
	updated := config.WithPluginSelection(spacemodel.PluginRef{Ref: item.Manifest.Name}, serviceRef)
	if err := s.writeSpaceConfig(updated); err != nil {
		return pluginmanager.InstalledPlugin{}, err
	}
	return item, nil
}

func (s *Server) UninstallSpacePlugin(_ context.Context, spaceID, name string) error {
	if name == "quark-main" {
		return fmt.Errorf("required main agent cannot be removed from a space")
	}
	s.pluginSelectionMu.Lock()
	defer s.pluginSelectionMu.Unlock()
	config, err := s.readSpaceConfig(spaceID)
	if err != nil {
		return err
	}
	return s.writeSpaceConfig(config.WithoutPluginSelection(name))
}

func (s *Server) SearchPlugins(_ context.Context, query string) ([]pluginmanager.PluginSearchItem, error) {
	return s.pluginRegistry.Search(query)
}

func (s *Server) HubPluginInfo(_ context.Context, name string) (*pluginmanager.HubPlugin, error) {
	return s.pluginRegistry.GetHubInfo(name)
}

func (s *Server) readSpaceConfig(spaceID string) (*spacemodel.Config, error) {
	data, err := s.store.Config(spaceID)
	if err != nil {
		return nil, err
	}
	return spacemodel.ParseAndValidateConfig(data, spaceID)
}

func (s *Server) writeSpaceConfig(config *spacemodel.Config) error {
	data, err := spacemodel.MarshalConfig(config)
	if err != nil {
		return err
	}
	_, err = s.store.UpdateConfig(data)
	return err
}

func containsServiceRef(refs []spacemodel.ServiceRef, name string) bool {
	for _, ref := range refs {
		if ref.Name == name || pluginmanager.ReferenceName(ref.Ref) == name {
			return true
		}
	}
	return false
}
