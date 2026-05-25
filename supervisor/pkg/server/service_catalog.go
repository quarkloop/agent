package server

import (
	"context"
	"fmt"

	plugin "github.com/quarkloop/pkg/plugin"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
)

func (s *Server) resolveServicePluginCatalog(ctx context.Context, space string) ([]*servicev1.ServiceDescriptor, error) {
	_ = ctx
	installed, err := s.selectedPlugins(space)
	if err != nil {
		return nil, fmt.Errorf("resolve selected plugins: %w", err)
	}
	serviceConfig, err := s.serviceConfigByPluginName(space)
	if err != nil {
		return nil, err
	}

	descriptors := make([]*servicev1.ServiceDescriptor, 0, len(installed))
	for _, item := range installed {
		if item.Manifest.Type != plugin.TypeService {
			continue
		}
		_, selected := servicePluginConfig(serviceConfig, item.Manifest)
		if !selected {
			continue
		}
		pluginDescriptors, err := descriptorsFromServiceManifest(item.Manifest)
		if err != nil {
			return nil, fmt.Errorf("service plugin %s manifest descriptors: %w", item.Manifest.Name, err)
		}
		skill := loadServicePluginSkill(item)
		if skill != nil {
			for _, desc := range pluginDescriptors {
				desc.Skills = replaceSkill(desc.GetSkills(), skill)
			}
		}
		if err := validateServicePluginDescriptors(pluginDescriptors, item.Manifest); err != nil {
			return nil, fmt.Errorf("service plugin %s descriptor: %w", item.Manifest.Name, err)
		}
		if err := s.importServiceFunctionRoutes(space, pluginDescriptors); err != nil {
			return nil, fmt.Errorf("service plugin %s nats imports: %w", item.Manifest.Name, err)
		}
		descriptors = append(descriptors, pluginDescriptors...)
	}
	return descriptors, nil
}
