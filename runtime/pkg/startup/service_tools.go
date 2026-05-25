package startup

import (
	"context"
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

func ServicePromptMaterials(catalog *runtimeservices.Catalog) []harnessclient.Material {
	if catalog == nil || catalog.Empty() {
		return nil
	}
	var materials []harnessclient.Material
	for _, descriptor := range catalog.Descriptors() {
		for _, skill := range descriptor.GetSkills() {
			content := strings.TrimSpace(skill.GetMarkdown())
			if content == "" {
				continue
			}
			materials = append(materials, harnessclient.Material{
				SourceID:   "plugin.service." + descriptor.GetName() + ".skill",
				SourceKind: "service_skill",
				Content:    content,
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

func ServiceFunctionPendingRefs(catalog *runtimeservices.Catalog) func() []string {
	if catalog == nil || catalog.Empty() {
		return nil
	}
	return catalog.PendingEmbeddingRefs
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
