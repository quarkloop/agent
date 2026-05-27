package startup

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	"github.com/quarkloop/runtime/pkg/agent"
	"github.com/quarkloop/runtime/pkg/coreevents"
	"github.com/quarkloop/runtime/pkg/gatewayclient"
	"github.com/quarkloop/runtime/pkg/harnessclient"
	"github.com/quarkloop/runtime/pkg/pluginmanager"
	runtimeservices "github.com/quarkloop/runtime/pkg/services"
)

func CoreEventRecorder(catalog *runtimeservices.Catalog) *coreevents.Recorder {
	if catalog == nil || catalog.Empty() {
		return nil
	}
	return coreevents.New(catalog, slog.Default())
}

func ModelProviderFromService(catalog *runtimeservices.Catalog, providerID string) plugin.Provider {
	return ModelProviderFromServiceWithConfig(catalog, providerID, gatewayclient.ConfigFromEnv())
}

func ModelProviderFromServiceWithConfig(catalog *runtimeservices.Catalog, providerID string, cfg gatewayclient.Config) plugin.Provider {
	if catalog == nil || catalog.Empty() || providerID == "" {
		return nil
	}
	for _, desc := range catalog.Descriptors() {
		if desc.GetName() == "gateway" || desc.GetType() == "gateway" {
			return gatewayclient.New(cfg, providerID)
		}
	}
	return nil
}

type gatewayModelCatalog interface {
	ListModels(context.Context) ([]plugin.ModelEntry, error)
}

func ResolveSelectedGatewayModel(ctx context.Context, provider plugin.Provider, modelID string) ([]plugin.ModelEntry, error) {
	catalog, ok := provider.(gatewayModelCatalog)
	if !ok {
		return nil, fmt.Errorf("Gateway model catalog is unavailable")
	}
	entries, err := catalog.ListModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("list Gateway models: %w", err)
	}
	for _, entry := range entries {
		if entry.ID != modelID {
			continue
		}
		if entry.ContextWindow <= 0 {
			return nil, fmt.Errorf("Gateway model %q does not declare a context window", modelID)
		}
		entry.Default = true
		return []plugin.ModelEntry{entry}, nil
	}
	return nil, fmt.Errorf("Gateway model %q is not available from the selected provider", modelID)
}

func ServicePromptMaterials(catalog *runtimeservices.Catalog) []harnessclient.Material {
	if catalog == nil || catalog.Empty() {
		return nil
	}
	var materials []harnessclient.Material
	for _, descriptor := range catalog.Descriptors() {
		functions := make([]string, 0, len(descriptor.GetRpcs()))
		for _, rpc := range descriptor.GetRpcs() {
			if name := strings.TrimSpace(runtimeservices.FunctionNameFor(descriptor.GetName(), rpc)); name != "" {
				functions = append(functions, name)
			}
		}
		for _, skill := range descriptor.GetSkills() {
			content := strings.TrimSpace(skill.GetMarkdown())
			if content == "" {
				continue
			}
			materials = append(materials, harnessclient.Material{
				SourceID:        "plugin.service." + descriptor.GetName() + ".skill",
				SourceKind:      "service_skill",
				Content:         content,
				ApplicableTools: append([]string(nil), functions...),
			})
		}
	}
	return materials
}

func AgentSkillMaterial(resolved pluginmanager.CatalogPlugin) harnessclient.Material {
	id := "resolved"
	if resolved.AgentProfile != nil && resolved.AgentProfile.ID != "" {
		id = resolved.AgentProfile.ID
	}
	return harnessclient.Material{
		SourceID:   "plugin.agent." + id + ".skill",
		SourceKind: "agent_skill",
		Content:    strings.TrimSpace(resolved.Skill),
	}
}

// SpecialistSkillMaterials gives a main coordinator the installed guidance for
// profiles it may delegate to without activating their identity/system prompt.
func SpecialistSkillMaterials(catalog *pluginmanager.Catalog, coordinator pluginmanager.CatalogPlugin) []harnessclient.Material {
	if catalog == nil || coordinator.AgentProfile == nil || !coordinator.AgentProfile.IsMain() {
		return nil
	}
	byID := make(map[string]pluginmanager.CatalogPlugin)
	for _, item := range catalog.Plugins {
		if item.Type != plugin.TypeAgent || item.AgentProfile == nil || strings.TrimSpace(item.Skill) == "" {
			continue
		}
		byID[item.AgentProfile.ID] = item
	}
	materials := make([]harnessclient.Material, 0, len(coordinator.AgentProfile.Handoff.CanDelegateTo))
	for _, target := range coordinator.AgentProfile.Handoff.CanDelegateTo {
		if item, ok := byID[target]; ok {
			materials = append(materials, AgentSkillMaterial(item))
		}
	}
	return materials
}

func HarnessComposer(credential clientcontract.NATSCredential) harnessclient.Composer {
	cfg := gatewayclient.ConfigFromEnv()
	return harnessclient.New(harnessclient.Config{
		URL:      FirstNonEmpty(credential.URL, cfg.URL),
		Username: FirstNonEmpty(credential.Username, cfg.Username),
		Password: FirstNonEmpty(credential.Password, cfg.Password),
		Name:     "quark-runtime-harness-" + SpaceToken(credential.SpaceID),
		SpaceID:  credential.SpaceID,
	})
}

func RegisterServiceFunctions(a *agent.Agent, catalog *runtimeservices.Catalog) {
	if catalog == nil || catalog.Empty() {
		return
	}
	for _, schema := range catalog.ToolSchemas() {
		schema := schema
		a.Plugins.RegisterRuntimeTool(pluginmanager.RuntimeTool{
			Schema: schema,
			Handler: func(ctx context.Context, arguments string) (string, error) {
				return catalog.Execute(ctx, schema.Name, arguments)
			},
		})
	}
}

func ServiceFunctionToolResultRef(catalog *runtimeservices.Catalog) func(name, arguments, result string) (string, error) {
	if catalog == nil || catalog.Empty() {
		return nil
	}
	return catalog.CaptureToolResult
}

func ServiceFunctionToolCallArgumentNormalizer(catalog *runtimeservices.Catalog) func(context.Context, string, string) (string, error) {
	if catalog == nil || catalog.Empty() {
		return nil
	}
	return catalog.NormalizeToolCallArguments
}
