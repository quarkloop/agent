package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/quarkloop/pkg/plugin"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	spacemodel "github.com/quarkloop/pkg/space"
	"github.com/quarkloop/supervisor/pkg/api"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
	"github.com/quarkloop/supervisor/pkg/serviceprocess"
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
	content, state, ok, err := s.services.Logs(space, name, 64*1024)
	if err != nil {
		return writeError(c, fiber.StatusInternalServerError, err.Error())
	}
	if !ok {
		return writeError(c, fiber.StatusNotFound, fmt.Sprintf("service %q is not supervisor-managed", name))
	}
	return writeJSON(c, fiber.StatusOK, api.ServiceLogsResponse{Name: name, LogPath: state.LogPath, Content: content})
}

func (s *Server) handleStartService(c *fiber.Ctx) error {
	space := c.Params("name")
	name := c.Params("service")
	info, err := s.startManagedService(c.Context(), space, name)
	if err != nil {
		return s.writeServiceLifecycleError(c, space, err)
	}
	return writeJSON(c, fiber.StatusCreated, api.ServiceLifecycleResponse{Service: info, Message: "service start requested"})
}

func (s *Server) handleStopService(c *fiber.Ctx) error {
	space := c.Params("name")
	name := c.Params("service")
	if _, err := s.store.Get(space); err != nil {
		return s.writeSpaceError(c, space, err)
	}
	if _, err := s.services.Stop(c.Context(), space, name); err != nil {
		return writeError(c, fiber.StatusNotFound, err.Error())
	}
	info, err := s.inspectService(c.Context(), space, name)
	if err != nil {
		return s.writeServiceLifecycleError(c, space, err)
	}
	return writeJSON(c, fiber.StatusOK, api.ServiceLifecycleResponse{Service: info, Message: "service stop requested"})
}

func (s *Server) handleRestartService(c *fiber.Ctx) error {
	space := c.Params("name")
	name := c.Params("service")
	info, err := s.restartManagedService(c.Context(), space, name)
	if err != nil {
		return s.writeServiceLifecycleError(c, space, err)
	}
	return writeJSON(c, fiber.StatusOK, api.ServiceRestartResponse{Service: info, Message: "service restart requested"})
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

func (s *Server) inspectService(ctx context.Context, space, serviceName string) (api.ServiceInfo, error) {
	services, err := s.inspectServices(ctx, space)
	if err != nil {
		return api.ServiceInfo{}, err
	}
	for _, service := range services {
		if service.Name == serviceName {
			return service, nil
		}
	}
	return api.ServiceInfo{}, fmt.Errorf("service %q not found", serviceName)
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
		out = append(out, s.inspectInstalledService(ctx, space, item, configured))
	}
	return out, nil
}

func (s *Server) inspectInstalledService(ctx context.Context, space string, item pluginmanager.InstalledPlugin, configured spacemodel.ServiceRef) api.ServiceInfo {
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
	if managed, ok := s.managedServiceState(space, item.Manifest.Name); ok {
		applyManagedServiceState(&info, managed)
		if managed.Endpoint != "" {
			address = managed.Endpoint
		}
		if managed.Status == api.ServiceStatusStopped || managed.Status == api.ServiceStatusStopping || managed.Status == api.ServiceStatusStarting {
			return info
		}
	}
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
	if managed, ok := s.managedServiceState(space, item.Manifest.Name); ok {
		s.services.MarkReady(space, item.Manifest.Name)
		applyManagedServiceState(&info, managed)
		info.Status = api.ServiceStatusReady
	}
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

func (s *Server) managedServiceState(space, serviceName string) (serviceprocess.State, bool) {
	if s.services == nil {
		return serviceprocess.State{}, false
	}
	return s.services.Inspect(space, serviceName)
}

func (s *Server) startManagedService(ctx context.Context, space, serviceName string) (api.ServiceInfo, error) {
	item, configured, err := s.installedService(space, serviceName)
	if err != nil {
		return api.ServiceInfo{}, err
	}
	spec, err := s.serviceProcessSpec(space, item, configured)
	if err != nil {
		return api.ServiceInfo{}, err
	}
	if _, err := s.services.Start(ctx, spec); err != nil {
		return api.ServiceInfo{}, err
	}
	s.waitForManagedServiceReadiness(ctx, spec, item.Manifest)
	return s.inspectService(ctx, space, serviceName)
}

func (s *Server) restartManagedService(ctx context.Context, space, serviceName string) (api.ServiceInfo, error) {
	item, configured, err := s.installedService(space, serviceName)
	if err != nil {
		return api.ServiceInfo{}, err
	}
	spec, err := s.serviceProcessSpec(space, item, configured)
	if err != nil {
		return api.ServiceInfo{}, err
	}
	if _, err := s.services.Restart(ctx, spec); err != nil {
		return api.ServiceInfo{}, err
	}
	s.waitForManagedServiceReadiness(ctx, spec, item.Manifest)
	return s.inspectService(ctx, space, serviceName)
}

func (s *Server) installedService(space, serviceName string) (pluginmanager.InstalledPlugin, spacemodel.ServiceRef, error) {
	if _, err := s.store.Get(space); err != nil {
		return pluginmanager.InstalledPlugin{}, spacemodel.ServiceRef{}, err
	}
	mgr, err := s.store.Plugins(space)
	if err != nil {
		return pluginmanager.InstalledPlugin{}, spacemodel.ServiceRef{}, err
	}
	installed, err := mgr.ListByType(plugin.TypeService)
	if err != nil {
		return pluginmanager.InstalledPlugin{}, spacemodel.ServiceRef{}, err
	}
	config, err := s.serviceConfigByPluginName(space)
	if err != nil {
		return pluginmanager.InstalledPlugin{}, spacemodel.ServiceRef{}, err
	}
	for _, item := range installed {
		if item.Manifest.Name == serviceName {
			return item, config[item.Manifest.Name], nil
		}
	}
	return pluginmanager.InstalledPlugin{}, spacemodel.ServiceRef{}, fmt.Errorf("service %q not found", serviceName)
}

func (s *Server) serviceProcessSpec(space string, item pluginmanager.InstalledPlugin, configured spacemodel.ServiceRef) (serviceprocess.ProcessSpec, error) {
	if item.Manifest.Service == nil {
		return serviceprocess.ProcessSpec{}, fmt.Errorf("service %q has no service config", item.Manifest.Name)
	}
	address := servicePluginAddress(item.Manifest, configured)
	if address == "" {
		port, err := reservePort()
		if err != nil {
			return serviceprocess.ProcessSpec{}, err
		}
		address = fmt.Sprintf("127.0.0.1:%d", port)
	}
	workingDir, err := os.Getwd()
	if err != nil {
		return serviceprocess.ProcessSpec{}, err
	}
	stateDir, err := s.store.ServiceStateDir(space, item.Manifest.Name)
	if err != nil {
		return serviceprocess.ProcessSpec{}, err
	}
	env := append(os.Environ(), item.Manifest.Service.AddressEnv+"="+address)
	args := []string{"--addr", address, "--skill-dir", item.Path}
	switch item.Manifest.Name {
	case "core":
		args = append(args, "--root", stateDir)
	case "ingestion":
		args = append(args, "--root", stateDir)
	case "indexer":
		dgraphAddr := firstEnv("QUARK_DGRAPH_ADDR", "DGRAPH_TEST_ADDR", "DGRAPH_ADDR")
		if dgraphAddr != "" {
			args = append(args, "--dgraph", dgraphAddr)
		}
	case "embedding-openrouter":
		env = append(env, "QUARK_EMBEDDING_PROVIDER=openrouter")
	}
	return serviceprocess.ProcessSpec{
		Space:         space,
		Name:          item.Manifest.Name,
		Binary:        s.serviceBinaryPath(item.Manifest),
		Args:          args,
		Env:           env,
		WorkingDir:    workingDir,
		Endpoint:      address,
		HealthService: healthServiceName(item.Manifest),
		LogPath:       filepath.Join(stateDir, "logs", "service.log"),
	}, nil
}

func (s *Server) serviceBinaryPath(manifest *plugin.Manifest) string {
	target := manifest.Name + "-service"
	if manifest.Build != nil && manifest.Build.APITarget != "" {
		target = manifest.Build.APITarget
	}
	switch manifest.Name {
	case "embedding-openrouter":
		target = "embedding-service"
	case "build-release":
		target = "build-release-service"
	}
	return filepath.Join(s.cfg.ServiceBinDir, target)
}

func (s *Server) waitForManagedServiceReadiness(ctx context.Context, spec serviceprocess.ProcessSpec, manifest *plugin.Manifest) {
	deadline, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		if err := checkServicePluginReadiness(deadline, spec.Endpoint, manifest); err == nil {
			s.services.MarkReady(spec.Space, spec.Name)
			return
		} else {
			lastErr = err
		}
		select {
		case <-deadline.Done():
			if lastErr != nil {
				s.services.MarkUnavailable(spec.Space, spec.Name, lastErr.Error())
			}
			return
		case <-ticker.C:
		}
	}
}

func applyManagedServiceState(info *api.ServiceInfo, state serviceprocess.State) {
	info.Status = state.Status
	info.PID = state.PID
	info.Endpoint = state.Endpoint
	info.LogPath = state.LogPath
	info.Diagnostics = append([]string(nil), state.Diagnostics...)
	if !state.StartedAt.IsZero() {
		started := state.StartedAt
		info.StartedAt = &started
	}
}

func (s *Server) writeServiceLifecycleError(c *fiber.Ctx, space string, err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "not found") {
		return writeError(c, fiber.StatusNotFound, err.Error())
	}
	return s.writeSpaceError(c, space, err)
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
