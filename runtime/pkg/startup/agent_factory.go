package startup

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	"github.com/quarkloop/runtime/pkg/agent"
	natschannel "github.com/quarkloop/runtime/pkg/channel/nats"
	"github.com/quarkloop/runtime/pkg/runtime"
)

type AgentRegistrar struct {
	Environment Environment
}

func (r AgentRegistrar) Register(ctx context.Context, srv *runtime.Server, spaces []string) error {
	spaceConfigs, err := SpaceConfigsFromEnv(spaces)
	if err != nil {
		return err
	}
	if len(spaceConfigs) == 0 {
		return fmt.Errorf("at least one runtime space is required for the NATS-native runtime")
	}
	for _, spaceConfig := range spaceConfigs {
		a, err := r.NewAgent(ctx, spaceConfig)
		if err != nil {
			return err
		}
		a.Send(agent.NewInitChannelMsg(srv.Bus()))
		slog.Info("registering nats channel", "space", spaceConfig.SpaceID)
		srv.Bus().Register(natschannel.New(
			NATSChannelConfig(spaceConfig.Credential),
			a,
			a.Sessions,
			natschannel.WithPlan(a.Plan),
			natschannel.WithActivity(a.Activity),
		))
		slog.Info("starting agent background loop", "space", spaceConfig.SpaceID, "agent_id", a.ID)
		go a.Run(ctx)
	}
	return nil
}

func (r AgentRegistrar) NewAgent(ctx context.Context, spaceConfig SpaceConfig) (*agent.Agent, error) {
	env := r.Environment
	catalogSnapshot, err := LoadRuntimeCatalogSnapshotForSpace(ctx, CatalogConfig(spaceConfig.Credential))
	if err != nil {
		return nil, err
	}
	serviceCatalog, err := LoadServiceCatalogForSpace(catalogSnapshot, spaceConfig.Credential)
	if err != nil {
		return nil, err
	}
	pluginCatalog, err := LoadPluginCatalog(catalogSnapshot)
	if err != nil {
		return nil, err
	}
	requestedAgent := env.AgentProfile
	if catalogSnapshot != nil && strings.TrimSpace(catalogSnapshot.AgentProfile) != "" {
		requestedAgent = catalogSnapshot.AgentProfile
	}
	agentPlugin, err := ResolveAgentPlugin(pluginCatalog, requestedAgent)
	if err != nil {
		return nil, err
	}
	modelProvider, modelName := ResolveModelSelection(agentPlugin.AgentProfile, env.ModelProvider, env.ModelName)
	if modelProvider == "" || modelName == "" {
		return nil, fmt.Errorf("model provider and name are required")
	}
	slog.Info("using model", "space", spaceConfig.SpaceID, "provider", modelProvider, "model", modelName)

	promptMaterials := ServicePromptMaterials(serviceCatalog)
	coreRecorder := CoreEventRecorder(serviceCatalog)
	modelProviderAdapter := ModelProviderFromServiceWithConfig(serviceCatalog, modelProvider, GatewayConfig(spaceConfig.Credential))
	if strings.TrimSpace(agentPlugin.Skill) != "" {
		promptMaterials = append(promptMaterials, AgentSkillMaterial(agentPlugin))
	}
	agentName := "Main Agent"
	agentDescription := ""
	resolvedProfile := agent.Profile{}
	if agentPlugin.AgentProfile != nil {
		agentName = agentPlugin.AgentProfile.Name
		agentDescription = agentPlugin.AgentProfile.Description
		resolvedProfile = AgentProfile(agentPlugin)
		slog.Info("using agent profile", "space", spaceConfig.SpaceID, "id", agentPlugin.AgentProfile.ID, "name", agentPlugin.AgentProfile.Name)
	}
	a, err := agent.NewAgent(agent.Config{
		ID:                   "main-" + SpaceToken(spaceConfig.SpaceID),
		Name:                 agentName,
		Description:          agentDescription,
		ModelProvider:        modelProvider,
		Model:                modelName,
		ModelListURL:         env.ModelListURL,
		Profile:              resolvedProfile,
		SystemPrompt:         agentPlugin.SystemPrompt,
		PluginsDir:           env.PluginsDir,
		PluginCatalog:        pluginCatalog,
		SupervisorURL:        env.SupervisorURL,
		SpaceID:              spaceConfig.SpaceID,
		PromptMaterials:      promptMaterials,
		ContextComposer:      HarnessComposer(spaceConfig.Credential),
		PendingRefs:          ServiceFunctionPendingRefs(serviceCatalog),
		ToolResultRef:        ServiceFunctionToolResultRef(serviceCatalog),
		ToolCallArguments:    ServiceFunctionToolCallArgumentNormalizer(serviceCatalog),
		CoreEvents:           coreRecorder,
		ModelProviderAdapter: modelProviderAdapter,
		PermissionPolicy:     PermissionPolicy(agentPlugin.AgentProfile),
	})
	if err != nil {
		return nil, fmt.Errorf("create agent for space %q: %w", spaceConfig.SpaceID, err)
	}
	RegisterServiceFunctions(a, serviceCatalog)
	return a, nil
}

func NATSChannelConfig(credential clientcontract.NATSCredential) natschannel.Config {
	cfg := natschannel.ConfigFromEnv()
	cfg.URL = FirstNonEmpty(credential.URL, cfg.URL)
	cfg.Username = FirstNonEmpty(credential.Username, cfg.Username)
	cfg.Password = FirstNonEmpty(credential.Password, cfg.Password)
	if credential.SpaceID != "" {
		cfg.Name = "quark-runtime-" + SpaceToken(credential.SpaceID)
	}
	return cfg
}
