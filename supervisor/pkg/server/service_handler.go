package server

import (
	"context"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
)

// inspectServices resolves the selected service-plugin catalog for the NATS
// control API. Service readiness here is descriptor readiness; runtime calls
// still receive transport failures from the owning service function.
func (s *Server) inspectServices(_ context.Context, space string) ([]clientcontract.ServiceInfo, error) {
	selected, err := s.selectedPlugins(space)
	if err != nil {
		return nil, err
	}
	serviceConfig, err := s.serviceConfigByPluginName(space)
	if err != nil {
		return nil, err
	}
	out := make([]clientcontract.ServiceInfo, 0, len(selected))
	for _, item := range selected {
		if item.Manifest.Type != plugin.TypeService {
			continue
		}
		_, configuredForSpace := servicePluginConfig(serviceConfig, item.Manifest)
		if !configuredForSpace {
			continue
		}
		out = append(out, inspectInstalledService(item))
	}
	return out, nil
}

func inspectInstalledService(item pluginmanager.InstalledPlugin) clientcontract.ServiceInfo {
	info := clientcontract.ServiceInfo{
		Name:        item.Manifest.Name,
		Type:        string(item.Manifest.Type),
		Version:     item.Manifest.Version,
		Description: item.Manifest.Description,
		Status:      clientcontract.ServiceStatusUnavailable,
	}
	if item.Manifest.Service == nil {
		info.Diagnostics = append(info.Diagnostics, "service manifest is missing service config")
		return info
	}
	info.HealthService = serviceHealthName(item.Manifest)
	info.MinVersion = item.Manifest.Service.Readiness.MinVersion
	info.Functions = serviceFunctionsForContract(item.Manifest.Service.Functions)
	info.FunctionCount = len(info.Functions)
	pluginDescriptors, err := descriptorsFromServiceManifest(item.Manifest)
	if err != nil {
		info.Diagnostics = append(info.Diagnostics, err.Error())
		return info
	}
	if err := validateServicePluginDescriptors(pluginDescriptors, item.Manifest); err != nil {
		info.Diagnostics = append(info.Diagnostics, err.Error())
		return info
	}
	info.SubjectPrefix = item.Manifest.Service.SubjectPrefix
	info.Status = clientcontract.ServiceStatusReady
	if count := descriptorFunctionCount(pluginDescriptors); count > 0 {
		info.FunctionCount = count
	}
	return info
}

func serviceFunctionsForContract(functions []plugin.ServiceFunctionConfig) []clientcontract.ServiceFunctionInfo {
	out := make([]clientcontract.ServiceFunctionInfo, 0, len(functions))
	for _, function := range functions {
		out = append(out, clientcontract.ServiceFunctionInfo{
			Name:        function.Name,
			Subject:     function.Subject,
			Service:     function.Service,
			Method:      function.Method,
			Request:     function.Request,
			Response:    function.Response,
			Description: function.Description,
			RiskLevel:   function.RiskLevel,
			Approval:    function.ApprovalRequired,
			Idempotent:  function.Idempotent,
		})
	}
	return out
}

func descriptorFunctionCount(descriptors []*servicev1.ServiceDescriptor) int {
	total := 0
	for _, desc := range descriptors {
		total += len(desc.GetRpcs())
	}
	return total
}
