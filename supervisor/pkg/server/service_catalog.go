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
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	spacemodel "github.com/quarkloop/pkg/space"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/emptypb"
)

const runtimeServiceCatalogEnv = "QUARK_RUNTIME_SERVICE_CATALOG"
const runtimePluginCatalogEnv = "QUARK_RUNTIME_PLUGIN_CATALOG"

type runtimePluginCatalog struct {
	Plugins []runtimePluginCatalogEntry `json:"plugins"`
}

type runtimePluginCatalogEntry struct {
	Name   string             `json:"name"`
	Type   string             `json:"type"`
	Path   string             `json:"path"`
	Schema *plugin.ToolSchema `json:"schema,omitempty"`
	Skill  string             `json:"skill,omitempty"`
}

func (s *Server) runtimePluginCatalogEnv(ctx context.Context, space string) ([]string, error) {
	_ = ctx
	mgr, err := s.store.Plugins(space)
	if err != nil {
		return nil, fmt.Errorf("open plugin store: %w", err)
	}
	installed, err := mgr.List()
	if err != nil {
		return nil, fmt.Errorf("list plugins: %w", err)
	}
	catalog := runtimePluginCatalog{Plugins: make([]runtimePluginCatalogEntry, 0, len(installed))}
	for _, item := range installed {
		switch item.Manifest.Type {
		case plugin.TypeTool, plugin.TypeProvider:
			catalog.Plugins = append(catalog.Plugins, runtimePluginCatalogEntryFromInstalled(item))
		}
	}
	payload, err := json.Marshal(catalog)
	if err != nil {
		return nil, fmt.Errorf("marshal runtime plugin catalog: %w", err)
	}
	return []string{runtimePluginCatalogEnv + "=" + string(payload)}, nil
}

func runtimePluginCatalogEntryFromInstalled(item pluginmanager.InstalledPlugin) runtimePluginCatalogEntry {
	entry := runtimePluginCatalogEntry{
		Name:  item.Manifest.Name,
		Type:  string(item.Manifest.Type),
		Path:  item.Path,
		Skill: readPluginSkill(item.Path),
	}
	if item.Manifest.Tool != nil {
		schema := item.Manifest.Tool.Schema
		entry.Schema = &schema
	}
	return entry
}

func readPluginSkill(pluginDir string) string {
	data, err := os.ReadFile(filepath.Join(pluginDir, "SKILL.md"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (s *Server) runtimeServiceCatalogEnv(ctx context.Context, space string) ([]string, error) {
	descriptors, err := s.resolveServicePluginCatalog(ctx, space)
	if err != nil {
		return nil, err
	}
	if len(descriptors) == 0 {
		return nil, nil
	}
	payload, err := protojson.Marshal(&servicev1.ListServicesResponse{Services: descriptors})
	if err != nil {
		return nil, fmt.Errorf("marshal runtime service catalog: %w", err)
	}
	return []string{runtimeServiceCatalogEnv + "=" + string(payload)}, nil
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
		address := servicePluginAddress(item.Manifest, serviceConfig[item.Manifest.Name])
		if address == "" {
			continue
		}
		discovered, err := discoverServicePlugin(ctx, address)
		if err != nil {
			return nil, fmt.Errorf("discover service plugin %s: %w", item.Manifest.Name, err)
		}
		skill := loadServicePluginSkill(item)
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
			descriptors = append(descriptors, desc)
		}
	}
	return descriptors, nil
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
	}
	return nil
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
