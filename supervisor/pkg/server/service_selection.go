package server

import (
	"fmt"
	"strings"

	plugin "github.com/quarkloop/pkg/plugin"
	spacemodel "github.com/quarkloop/pkg/space"
)

func (s *Server) serviceConfigByPluginName(space string) (map[string]spacemodel.ServiceRef, error) {
	data, err := s.store.Config(space)
	if err != nil {
		return nil, fmt.Errorf("read space config for service selection: %w", err)
	}
	config, err := spacemodel.ParseAndValidateConfig(data, space)
	if err != nil {
		return nil, fmt.Errorf("parse space config for service selection: %w", err)
	}
	out := make(map[string]spacemodel.ServiceRef, len(config.Services))
	for _, service := range config.Services {
		out[service.Name] = service
		if service.Ref != "" {
			out[pluginNameFromRef(service.Ref)] = service
		}
	}
	return out, nil
}

func servicePluginConfig(serviceConfig map[string]spacemodel.ServiceRef, manifest *plugin.Manifest) (spacemodel.ServiceRef, bool) {
	if manifest == nil {
		return spacemodel.ServiceRef{}, false
	}
	configured, ok := serviceConfig[manifest.Name]
	return configured, ok
}

func pluginNameFromRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	ref = strings.TrimSuffix(ref, "/")
	if idx := strings.LastIndex(ref, "/"); idx >= 0 {
		return ref[idx+1:]
	}
	return ref
}
