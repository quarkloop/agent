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
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	spacemodel "github.com/quarkloop/pkg/space"
	"github.com/quarkloop/supervisor/pkg/natshub"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/types/known/emptypb"
)

const runtimeServiceCatalogEnv = "QUARK_RUNTIME_SERVICE_CATALOG"
const runtimePluginCatalogEnv = "QUARK_RUNTIME_PLUGIN_CATALOG"
const defaultServiceFunctionTimeout = 30 * time.Second

type runtimePluginCatalogEntry = plugin.RuntimeCatalogPlugin

func (s *Server) runtimePluginCatalogEnv(ctx context.Context, space string) ([]string, error) {
	catalog, selectedAgent, err := s.resolveRuntimePluginCatalog(ctx, space)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(catalog)
	if err != nil {
		return nil, fmt.Errorf("marshal runtime plugin catalog: %w", err)
	}
	env := []string{runtimePluginCatalogEnv + "=" + string(payload)}
	if selectedAgent != "" {
		env = append(env, runtimeAgentProfileEnv+"="+selectedAgent)
	}
	return env, nil
}

func (s *Server) resolveRuntimePluginCatalog(ctx context.Context, space string) (plugin.RuntimeCatalog, string, error) {
	_ = ctx
	qfBytes, err := s.store.Quarkfile(space)
	if err != nil {
		return plugin.RuntimeCatalog{}, "", fmt.Errorf("read quarkfile: %w", err)
	}
	qf, err := spacemodel.ParseAndValidateQuarkfileForSpace(qfBytes, space)
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
		case plugin.TypeTool, plugin.TypeProvider, plugin.TypeAgent:
			entry, err := runtimePluginCatalogEntryFromInstalled(item)
			if err != nil {
				return plugin.RuntimeCatalog{}, "", fmt.Errorf("build runtime plugin catalog entry %s: %w", item.Manifest.Name, err)
			}
			catalog.Plugins = append(catalog.Plugins, entry)
		}
	}
	plugins, selectedAgent, err := newAgentProfileOverrideResolver(qf).apply(catalog.Plugins)
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

func (s *Server) runtimeServiceCatalogEnv(ctx context.Context, space string) ([]string, error) {
	payload, err := s.runtimeServiceCatalogPayload(ctx, space)
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 {
		return nil, nil
	}
	return []string{runtimeServiceCatalogEnv + "=" + string(payload)}, nil
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

func (s *Server) resolveServicePluginCatalog(ctx context.Context, space string) ([]*servicev1.ServiceDescriptor, error) {
	mgr, err := s.store.Plugins(space)
	if err != nil {
		return nil, fmt.Errorf("open plugin store: %w", err)
	}
	installed, err := mgr.ListByType(plugin.TypeService)
	if err != nil {
		return nil, fmt.Errorf("list service plugins: %w", err)
	}
	serviceConfig, err := s.serviceConfigByPluginName(space)
	if err != nil {
		return nil, err
	}

	descriptors := make([]*servicev1.ServiceDescriptor, 0, len(installed))
	for _, item := range installed {
		configured, selected := servicePluginConfig(serviceConfig, item.Manifest)
		if !selected {
			continue
		}
		address := s.serviceCatalogAddress(space, item.Manifest, configured)
		if address == "" {
			if servicePluginReadinessRequired(item.Manifest, configured) {
				return nil, fmt.Errorf("service plugin %s missing endpoint: set %s or configure services[].address", item.Manifest.Name, item.Manifest.Service.AddressEnv)
			}
			continue
		}
		if err := checkServicePluginReadiness(ctx, address, item.Manifest); err != nil {
			return nil, fmt.Errorf("service plugin %s readiness at %s: %w", item.Manifest.Name, address, err)
		}
		discovered, err := discoverServicePlugin(ctx, address)
		if err != nil {
			return nil, fmt.Errorf("discover service plugin %s: %w", item.Manifest.Name, err)
		}
		skill := loadServicePluginSkill(item)
		pluginDescriptors := make([]*servicev1.ServiceDescriptor, 0, len(discovered))
		for _, desc := range discovered {
			if desc.GetAddress() == "" {
				desc.Address = address
			}
			if desc.GetName() == "" {
				desc.Name = item.Manifest.Name
			}
			if err := applyServiceFunctionMetadata(desc, item.Manifest); err != nil {
				return nil, fmt.Errorf("service plugin %s metadata: %w", item.Manifest.Name, err)
			}
			if skill != nil {
				desc.Skills = replaceSkill(desc.GetSkills(), skill)
			}
			pluginDescriptors = append(pluginDescriptors, desc)
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

func (s *Server) importServiceFunctionRoutes(space string, descriptors []*servicev1.ServiceDescriptor) error {
	if s == nil || s.natsHub == nil || len(descriptors) == 0 {
		return nil
	}
	routes := make([]natshub.ServiceFunctionRoute, 0)
	for _, desc := range descriptors {
		for _, rpc := range desc.GetRpcs() {
			owner := strings.TrimSpace(rpc.GetOwner())
			if owner == "" && desc != nil {
				owner = desc.GetName()
			}
			method := strings.TrimSpace(rpc.GetMethod())
			if owner == "" || method == "" {
				continue
			}
			route, err := natshub.NewServiceFunctionRoute(owner, "v1", method)
			if err != nil {
				return err
			}
			routes = append(routes, route)
		}
	}
	if len(routes) == 0 {
		return nil
	}
	return s.natsHub.ImportServiceFunctions(space, routes)
}

func applyServiceFunctionMetadata(desc *servicev1.ServiceDescriptor, manifest *plugin.Manifest) error {
	if desc == nil || manifest == nil || manifest.Service == nil {
		return nil
	}
	functions := manifest.Service.Functions
	if len(functions) == 0 {
		return nil
	}
	byRPC := make(map[string]plugin.ServiceFunctionConfig, len(functions))
	for _, function := range functions {
		key := serviceFunctionKey(function.Service, function.Method)
		byRPC[key] = function
	}
	for _, rpc := range desc.GetRpcs() {
		function, ok := byRPC[serviceFunctionKey(rpc.GetService(), rpc.GetMethod())]
		if !ok {
			return fmt.Errorf("missing manifest function for %s/%s", rpc.GetService(), rpc.GetMethod())
		}
		rpc.Request = function.Request
		rpc.Response = function.Response
		rpc.Description = function.Description
		rpc.Owner = serviceFunctionOwner(manifest.Name, function)
		rpc.FunctionName = function.Name
		rpc.RiskLevel = serviceFunctionRisk(function)
		rpc.ApprovalRequired = function.ApprovalRequired
		rpc.ApprovalRequirements = append([]string(nil), function.ApprovalRequirements...)
		rpc.Streaming = function.Streaming
		rpc.Idempotent = function.Idempotent
		rpc.TimeoutMillis = serviceFunctionTimeoutMillis(function)
		rpc.RetryPolicy = serviceFunctionRetryPolicy(function)
		rpc.Examples = serviceFunctionExamples(function)
	}
	return nil
}

func serviceFunctionOwner(pluginName string, function plugin.ServiceFunctionConfig) string {
	if strings.TrimSpace(function.Owner) != "" {
		return strings.TrimSpace(function.Owner)
	}
	return strings.TrimSpace(pluginName)
}

func serviceFunctionRisk(function plugin.ServiceFunctionConfig) string {
	if strings.TrimSpace(function.RiskLevel) == "" {
		return "read"
	}
	return strings.TrimSpace(function.RiskLevel)
}

func serviceFunctionTimeoutMillis(function plugin.ServiceFunctionConfig) int32 {
	timeout := defaultServiceFunctionTimeout
	if strings.TrimSpace(function.Timeout) != "" {
		parsed, err := time.ParseDuration(function.Timeout)
		if err == nil {
			timeout = parsed
		}
	}
	return int32(timeout / time.Millisecond)
}

func serviceFunctionRetryPolicy(function plugin.ServiceFunctionConfig) *servicev1.RetryPolicy {
	policy := function.RetryPolicy
	if policy.MaxAttempts == 0 && len(policy.RetryableCodes) == 0 && policy.InitialBackoffMillis == 0 && policy.MaxBackoffMillis == 0 {
		return nil
	}
	return &servicev1.RetryPolicy{
		MaxAttempts:          int32(policy.MaxAttempts),
		RetryableCodes:       append([]string(nil), policy.RetryableCodes...),
		InitialBackoffMillis: int32(policy.InitialBackoffMillis),
		MaxBackoffMillis:     int32(policy.MaxBackoffMillis),
	}
}

func serviceFunctionExamples(function plugin.ServiceFunctionConfig) []*servicev1.ServiceFunctionExample {
	out := make([]*servicev1.ServiceFunctionExample, 0, len(function.Examples))
	for _, example := range function.Examples {
		out = append(out, &servicev1.ServiceFunctionExample{
			Name:        example.Name,
			Description: example.Description,
			RequestJson: example.RequestJSON,
		})
	}
	return out
}

func serviceFunctionKey(service, method string) string {
	return strings.TrimSpace(service) + "/" + strings.TrimSpace(method)
}

func (s *Server) serviceConfigByPluginName(space string) (map[string]spacemodel.ServiceRef, error) {
	data, err := s.store.Quarkfile(space)
	if err != nil {
		return nil, fmt.Errorf("read quarkfile for service config: %w", err)
	}
	qf, err := spacemodel.ParseAndValidateQuarkfileForSpace(data, space)
	if err != nil {
		return nil, fmt.Errorf("parse quarkfile for service config: %w", err)
	}
	out := make(map[string]spacemodel.ServiceRef, len(qf.Services))
	for _, service := range qf.Services {
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

func servicePluginAddress(manifest *plugin.Manifest, configured spacemodel.ServiceRef) string {
	if manifest == nil || manifest.Service == nil {
		return ""
	}
	if configured.AddressEnv != "" {
		if value := strings.TrimSpace(os.Getenv(configured.AddressEnv)); value != "" {
			return value
		}
	}
	if configured.Address != "" {
		return strings.TrimSpace(configured.Address)
	}
	if manifest.Service.AddressEnv != "" {
		if value := strings.TrimSpace(os.Getenv(manifest.Service.AddressEnv)); value != "" {
			return value
		}
	}
	return strings.TrimSpace(manifest.Service.DefaultAddress)
}

func (s *Server) serviceCatalogAddress(space string, manifest *plugin.Manifest, configured spacemodel.ServiceRef) string {
	return servicePluginAddress(manifest, configured)
}

func servicePluginReadinessRequired(manifest *plugin.Manifest, configured spacemodel.ServiceRef) bool {
	if configured.Name != "" {
		return true
	}
	return manifest != nil && manifest.Service != nil && manifest.Service.Readiness.Required
}

func checkServicePluginReadiness(ctx context.Context, address string, manifest *plugin.Manifest) error {
	if manifest == nil || manifest.Service == nil {
		return nil
	}
	timeout, err := time.ParseDuration(manifest.Service.Health.Timeout)
	if err != nil {
		return fmt.Errorf("invalid health timeout: %w", err)
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := servicekit.Dial(callCtx, address)
	if err != nil {
		return fmt.Errorf("dial health endpoint: %w", err)
	}
	defer conn.Close()
	resp, err := healthpb.NewHealthClient(conn).Check(callCtx, &healthpb.HealthCheckRequest{Service: healthServiceName(manifest)})
	if err != nil {
		return fmt.Errorf("grpc health check: %w", err)
	}
	if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		return fmt.Errorf("health status is %s", resp.GetStatus().String())
	}
	return nil
}

func healthServiceName(manifest *plugin.Manifest) string {
	if manifest == nil || manifest.Service == nil {
		return ""
	}
	if manifest.Service.Health.Service != "" {
		return manifest.Service.Health.Service
	}
	if len(manifest.Service.ProtoServices) > 0 {
		return manifest.Service.ProtoServices[0]
	}
	return ""
}

func validateServicePluginDescriptors(descriptors []*servicev1.ServiceDescriptor, manifest *plugin.Manifest) error {
	if manifest == nil || manifest.Service == nil {
		return nil
	}
	if len(descriptors) == 0 {
		return fmt.Errorf("no descriptors returned")
	}
	minVersion := strings.TrimSpace(manifest.Service.Readiness.MinVersion)
	seenRPC := make(map[string]bool)
	seenFunction := make(map[string]bool)
	for _, desc := range descriptors {
		if desc.GetName() == "" {
			return fmt.Errorf("missing descriptor name")
		}
		if minVersion != "" && desc.GetVersion() != minVersion {
			return fmt.Errorf("unsupported version %q for %s (required: %s)", desc.GetVersion(), desc.GetName(), minVersion)
		}
		if desc.GetAddress() == "" {
			return fmt.Errorf("descriptor %s missing endpoint address", desc.GetName())
		}
		for _, rpc := range desc.GetRpcs() {
			seenRPC[serviceFunctionKey(rpc.GetService(), rpc.GetMethod())] = true
			if rpc.GetFunctionName() == "" {
				return fmt.Errorf("missing function name for %s/%s", rpc.GetService(), rpc.GetMethod())
			}
			if seenFunction[rpc.GetFunctionName()] {
				return fmt.Errorf("duplicate function name %s", rpc.GetFunctionName())
			}
			seenFunction[rpc.GetFunctionName()] = true
		}
	}
	for _, function := range manifest.Service.Functions {
		if !seenRPC[serviceFunctionKey(function.Service, function.Method)] {
			return fmt.Errorf("missing RPC descriptor for %s/%s", function.Service, function.Method)
		}
		if !seenFunction[function.Name] {
			return fmt.Errorf("missing function descriptor for %s", function.Name)
		}
	}
	return nil
}

func discoverServicePlugin(ctx context.Context, address string) ([]*servicev1.ServiceDescriptor, error) {
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	conn, err := servicekit.Dial(callCtx, address)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	resp, err := servicev1.NewServiceRegistryClient(conn).ListServices(callCtx, &emptypb.Empty{})
	if err != nil {
		return nil, err
	}
	out := make([]*servicev1.ServiceDescriptor, 0, len(resp.GetServices()))
	for _, desc := range resp.GetServices() {
		out = append(out, servicekit.CloneDescriptor(desc))
	}
	return out, nil
}

func loadServicePluginSkill(item pluginmanager.InstalledPlugin) *servicev1.SkillDescriptor {
	if item.Manifest == nil || item.Manifest.Service == nil {
		return nil
	}
	skillPath := item.Manifest.Service.Skill
	if skillPath == "" {
		skillPath = "SKILL.md"
	}
	data, err := os.ReadFile(filepath.Join(item.Path, skillPath))
	if err != nil {
		return nil
	}
	return &servicev1.SkillDescriptor{
		Name:     "service-" + item.Manifest.Name,
		Version:  item.Manifest.Version,
		Markdown: string(data),
	}
}

func replaceSkill(skills []*servicev1.SkillDescriptor, skill *servicev1.SkillDescriptor) []*servicev1.SkillDescriptor {
	out := make([]*servicev1.SkillDescriptor, 0, len(skills)+1)
	for _, existing := range skills {
		if existing.GetName() == skill.GetName() {
			continue
		}
		out = append(out, existing)
	}
	return append(out, skill)
}
