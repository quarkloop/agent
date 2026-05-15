package server

import (
	"context"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/quarkloop/pkg/plugin"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	spacemodel "github.com/quarkloop/pkg/space"
	"github.com/quarkloop/supervisor/pkg/api"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
)

func (s *Server) handleListServices(c *fiber.Ctx) error {
	name := c.Params("name")
	services, err := s.inspectServices(c.Context(), name)
	if err != nil {
		return s.writeSpaceError(c, name, err)
	}
	return writeJSON(c, fiber.StatusOK, api.ListServicesResponse{Services: services})
}

func (s *Server) handleInspectService(c *fiber.Ctx) error {
	name := c.Params("name")
	serviceName := c.Params("service")
	services, err := s.inspectServices(c.Context(), name)
	if err != nil {
		return s.writeSpaceError(c, name, err)
	}
	for _, service := range services {
		if service.Name == serviceName {
			return writeJSON(c, fiber.StatusOK, service)
		}
	}
	return writeError(c, fiber.StatusNotFound, fmt.Sprintf("service %q not found", serviceName))
}

func (s *Server) handleServiceLogs(c *fiber.Ctx) error {
	space := c.Params("name")
	if _, err := s.store.Get(space); err != nil {
		return s.writeSpaceError(c, space, err)
	}
	name := c.Params("service")
	return writeJSON(c, fiber.StatusOK, api.ServiceLogsResponse{
		Name:      name,
		Supported: false,
		Message:   "service log streaming is not available until supervisor owns service process lifecycle",
	})
}

func (s *Server) handleRestartService(c *fiber.Ctx) error {
	space := c.Params("name")
	if _, err := s.store.Get(space); err != nil {
		return s.writeSpaceError(c, space, err)
	}
	name := c.Params("service")
	return writeJSON(c, fiber.StatusOK, api.ServiceRestartResponse{
		Name:      name,
		Supported: false,
		Message:   "service restart is not available until supervisor owns service process lifecycle",
	})
}

func (s *Server) handleServiceDoctor(c *fiber.Ctx) error {
	name := c.Params("name")
	services, err := s.inspectServices(c.Context(), name)
	if err != nil {
		return s.writeSpaceError(c, name, err)
	}
	issues := make([]string, 0)
	for _, service := range services {
		if service.Status == api.ServiceStatusReady {
			continue
		}
		for _, diagnostic := range service.Diagnostics {
			issues = append(issues, fmt.Sprintf("%s: %s", service.Name, diagnostic))
		}
		if len(service.Diagnostics) == 0 {
			issues = append(issues, fmt.Sprintf("%s: status is %s", service.Name, service.Status))
		}
	}
	return writeJSON(c, fiber.StatusOK, api.ServiceDoctorResponse{Services: services, Issues: issues})
}

func (s *Server) inspectServices(ctx context.Context, space string) ([]api.ServiceInfo, error) {
	mgr, err := s.store.Plugins(space)
	if err != nil {
		return nil, err
	}
	installed, err := mgr.ListByType(plugin.TypeService)
	if err != nil {
		return nil, err
	}
	serviceConfig, err := s.serviceConfigByPluginName(space)
	if err != nil {
		return nil, err
	}
	out := make([]api.ServiceInfo, 0, len(installed))
	for _, item := range installed {
		configured := serviceConfig[item.Manifest.Name]
		out = append(out, s.inspectInstalledService(ctx, item, configured))
	}
	return out, nil
}

func (s *Server) inspectInstalledService(ctx context.Context, item pluginmanager.InstalledPlugin, configured spacemodel.ServiceRef) api.ServiceInfo {
	info := api.ServiceInfo{
		Name:        item.Manifest.Name,
		Type:        string(item.Manifest.Type),
		Version:     item.Manifest.Version,
		Mode:        string(item.Manifest.Mode),
		Description: item.Manifest.Description,
		Status:      api.ServiceStatusUnavailable,
		Functions:   nil,
	}
	if item.Manifest.Service == nil {
		info.Diagnostics = append(info.Diagnostics, "service manifest is missing service config")
		return info
	}
	info.AddressEnv = item.Manifest.Service.AddressEnv
	if configured.AddressEnv != "" {
		info.AddressEnv = configured.AddressEnv
	}
	info.HealthService = healthServiceName(item.Manifest)
	info.MinVersion = item.Manifest.Service.Readiness.MinVersion
	info.Functions = serviceFunctionsForAPI(item.Manifest.Service.Functions)
	info.FunctionCount = len(info.Functions)

	address := servicePluginAddress(item.Manifest, configured)
	if address == "" {
		if servicePluginReadinessRequired(item.Manifest, configured) {
			info.Status = api.ServiceStatusMissing
			info.Diagnostics = append(info.Diagnostics, fmt.Sprintf("missing endpoint: set %s or configure services[].address", item.Manifest.Service.AddressEnv))
			return info
		}
		info.Status = api.ServiceStatusUnconfigured
		info.Diagnostics = append(info.Diagnostics, "service endpoint is not configured")
		return info
	}
	info.Endpoint = address
	if err := checkServicePluginReadiness(ctx, address, item.Manifest); err != nil {
		info.Diagnostics = append(info.Diagnostics, err.Error())
		return info
	}
	discovered, err := discoverServicePlugin(ctx, address)
	if err != nil {
		info.Diagnostics = append(info.Diagnostics, err.Error())
		return info
	}
	pluginDescriptors := make([]*servicev1.ServiceDescriptor, 0, len(discovered))
	for _, desc := range discovered {
		if desc.GetAddress() == "" {
			desc.Address = address
		}
		if desc.GetName() == "" {
			desc.Name = item.Manifest.Name
		}
		if err := applyServiceFunctionMetadata(desc, item.Manifest); err != nil {
			info.Diagnostics = append(info.Diagnostics, err.Error())
			return info
		}
		pluginDescriptors = append(pluginDescriptors, desc)
	}
	if err := validateServicePluginDescriptors(pluginDescriptors, item.Manifest); err != nil {
		info.Diagnostics = append(info.Diagnostics, err.Error())
		return info
	}
	info.Status = api.ServiceStatusReady
	if count := descriptorFunctionCount(pluginDescriptors); count > 0 {
		info.FunctionCount = count
	}
	return info
}

func serviceFunctionsForAPI(functions []plugin.ServiceFunctionConfig) []api.ServiceFunctionInfo {
	out := make([]api.ServiceFunctionInfo, 0, len(functions))
	for _, function := range functions {
		out = append(out, api.ServiceFunctionInfo{
			Name:        function.Name,
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
