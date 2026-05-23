package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/runtime/pkg/agent"
	"github.com/quarkloop/runtime/pkg/catalogclient"
	natschannel "github.com/quarkloop/runtime/pkg/channel/nats"
	"github.com/quarkloop/runtime/pkg/channel/telegram"
	"github.com/quarkloop/runtime/pkg/channel/web"
	"github.com/quarkloop/runtime/pkg/coreevents"
	"github.com/quarkloop/runtime/pkg/gatewayclient"
	"github.com/quarkloop/runtime/pkg/permissions"
	"github.com/quarkloop/runtime/pkg/pluginmanager"
	"github.com/quarkloop/runtime/pkg/runtime"
	runtimeservices "github.com/quarkloop/runtime/pkg/services"
	"github.com/quarkloop/runtime/pkg/spacelease"
)

const CmdStartDefaultPort = 8765

// Start creates the "runtime start" command.
func Start() *cobra.Command {
	var port int
	var channelsFlag []string

	cmd := &cobra.Command{
		Use:           "start [channels...]",
		Short:         "start the runtime",
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			var channels []string

			// If flag was explicitly changed from default, or args are empty, use the flag
			if cmd.Flags().Changed("channel") || len(args) == 0 {
				channels = append(channels, channelsFlag...)
			}

			// Support positional arguments (e.g. `binary start channel web telegram`)
			for _, arg := range args {
				if arg != "channel" && arg != "channels" {
					channels = append(channels, arg)
				}
			}

			if len(channels) == 0 {
				channels = []string{"web"} // Fallback
			}

			return runStart(port, channels)
		},
	}

	cmd.Flags().IntVarP(&port, "port", "p", CmdStartDefaultPort, "HTTP listen port")
	cmd.Flags().StringSliceVarP(&channelsFlag, "channel", "c", []string{"web"}, "Channels to use (e.g., 'web', 'telegram', 'web,telegram', or 'all')")

	return cmd
}

func runStart(port int, channels []string) error {
	if os.Getenv("QUARK_SUPERVISOR_URL") == "" {
		loadEnvFiles()
	}

	// 1. Deduplicate channels and handle "all"
	activeChannels := make(map[string]bool)
	for _, ch := range channels {
		if ch == "all" {
			activeChannels["web"] = true
			activeChannels["telegram"] = true
			activeChannels["nats"] = true
		} else {
			activeChannels[ch] = true
		}
	}

	if len(activeChannels) == 0 {
		return fmt.Errorf("no channels specified to start")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	spaces := runtimeSpacesFromEnv()
	if os.Getenv("QUARK_SPACE") == "" && len(spaces) > 0 {
		if err := os.Setenv("QUARK_SPACE", spaces[0]); err != nil {
			return fmt.Errorf("set primary runtime space: %w", err)
		}
	}
	leaseManager, leases, err := claimRuntimeSpaces(ctx, spaces)
	if err != nil {
		return err
	}
	defer releaseRuntimeSpaces(context.Background(), leases, leaseManager)

	// 2. Early validation: fail fast if any channel is invalid
	var validChannels []string
	for ch := range activeChannels {
		switch ch {
		case "web", "telegram", "nats":
			validChannels = append(validChannels, ch)
		default:
			return fmt.Errorf("unknown channel requested: %q", ch)
		}
	}

	slog.Info("starting runtime")
	slog.Info("enabled channels", "channels", fmt.Sprintf("%v", validChannels))

	catalogSnapshot, err := loadRuntimeCatalogSnapshot(ctx)
	if err != nil {
		return err
	}
	serviceCatalog, err := loadServiceCatalog(catalogSnapshot)
	if err != nil {
		return err
	}
	pluginCatalog, err := loadPluginCatalog(catalogSnapshot)
	if err != nil {
		return err
	}
	requestedAgent := os.Getenv("QUARK_AGENT_PROFILE")
	if catalogSnapshot != nil && strings.TrimSpace(catalogSnapshot.AgentProfile) != "" {
		requestedAgent = catalogSnapshot.AgentProfile
	}
	agentPlugin, err := resolveAgentPlugin(pluginCatalog, requestedAgent)
	if err != nil {
		return err
	}

	modelProvider, modelName := resolveModelSelection(agentPlugin.AgentProfile, os.Getenv("QUARK_MODEL_PROVIDER"), os.Getenv("QUARK_MODEL_NAME"))
	if modelProvider == "" || modelName == "" {
		return fmt.Errorf("model provider and name are required")
	}
	slog.Info("using model", "provider", modelProvider, "model", modelName)

	promptAddenda := servicePromptAddenda(serviceCatalog)
	coreRecorder := coreEventRecorder(serviceCatalog)
	var modelProviderAdapter plugin.Provider
	if adapter := modelProviderFromService(serviceCatalog, modelProvider); adapter != nil {
		modelProviderAdapter = adapter
	}
	if strings.TrimSpace(agentPlugin.Skill) != "" {
		promptAddenda = append(promptAddenda, strings.TrimSpace(agentPlugin.Skill))
	}
	agentName := "Main Agent"
	agentDescription := ""
	resolvedProfile := agent.Profile{}
	if agentPlugin.AgentProfile != nil {
		agentName = agentPlugin.AgentProfile.Name
		agentDescription = agentPlugin.AgentProfile.Description
		resolvedProfile = runtimeAgentProfile(agentPlugin)
		slog.Info("using agent profile", "id", agentPlugin.AgentProfile.ID, "name", agentPlugin.AgentProfile.Name)
	}

	// Create agent
	a, err := agent.NewAgent(agent.Config{
		ID:                   "main",
		Name:                 agentName,
		Description:          agentDescription,
		ModelProvider:        modelProvider,
		Model:                modelName,
		ModelListURL:         os.Getenv("MODEL_LIST_URL"),
		Profile:              resolvedProfile,
		SystemPrompt:         agentPlugin.SystemPrompt,
		PluginsDir:           os.Getenv("QUARK_PLUGINS_DIR"),
		PluginCatalog:        pluginCatalog,
		SupervisorURL:        os.Getenv("QUARK_SUPERVISOR_URL"),
		SpaceID:              os.Getenv("QUARK_SPACE"),
		PromptAddenda:        promptAddenda,
		PendingRefs:          serviceFunctionPendingRefs(serviceCatalog),
		ToolResultRef:        serviceFunctionToolResultRef(serviceCatalog),
		ToolCallArguments:    serviceFunctionToolCallArgumentNormalizer(serviceCatalog),
		CoreEvents:           coreRecorder,
		ModelProviderAdapter: modelProviderAdapter,
		PermissionPolicy:     runtimePermissionPolicy(agentPlugin.AgentProfile),
	})
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}
	registerServiceFunctions(a, serviceCatalog)

	// Create server with ChannelBus
	srv := runtime.NewServer()

	// Wire ChannelBus to agent via typed message
	a.Send(agent.NewInitChannelMsg(srv.Bus()))

	// 3. Instantiate and register the requested channels
	for _, ch := range validChannels {
		switch ch {
		case "web":
			listenAddr := fmt.Sprintf(":%d", port)
			slog.Info("registering web channel", "listen_addr", listenAddr)
			srv.Bus().Register(web.New(listenAddr, a))

		case "telegram":
			token := os.Getenv("TELEGRAM_BOT_TOKEN")
			if token == "" {
				return fmt.Errorf("TELEGRAM_BOT_TOKEN environment variable is required for the telegram channel")
			}

			slog.Info("registering telegram channel")
			srv.Bus().Register(telegram.New(
				telegram.Config{Token: token},
				a,
				func(id, chType, title string) { a.Sessions.GetOrCreate(id, chType, title) },
			))
		case "nats":
			slog.Info("registering nats channel")
			srv.Bus().Register(natschannel.New(
				natschannel.ConfigFromEnv(),
				a,
				a.Sessions,
				natschannel.WithPlan(a.Plan),
				natschannel.WithActivity(a.Activity),
			))
		}
	}

	slog.Info("starting agent background loop")
	go a.Run(ctx)

	slog.Info("runtime server is running, press Ctrl+C to exit")
	// Start all channels via ChannelBus and block
	return srv.Run(ctx)
}

func coreEventRecorder(catalog *runtimeservices.Catalog) *coreevents.Recorder {
	if catalog == nil || catalog.Empty() {
		return nil
	}
	return coreevents.New(catalog.Descriptors(), slog.Default())
}

func modelProviderFromService(catalog *runtimeservices.Catalog, providerID string) plugin.Provider {
	if catalog == nil || catalog.Empty() || providerID == "" {
		return nil
	}
	for _, desc := range catalog.Descriptors() {
		if desc.GetName() == "gateway" || desc.GetType() == "gateway" {
			return gatewayclient.New(gatewayclient.ConfigFromEnv(), providerID)
		}
	}
	return nil
}

func runtimePermissionPolicy(profile *plugin.AgentProfile) *permissions.Policy {
	if profile == nil {
		return nil
	}
	allowed := make([]string, 0, len(profile.Permissions.Tools)+len(profile.Permissions.Services))
	seen := make(map[string]struct{}, cap(allowed))
	add := func(values []string) {
		for _, value := range values {
			name := strings.TrimSpace(value)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			allowed = append(allowed, name)
		}
	}
	add(profile.Permissions.Tools)
	add(profile.Permissions.Services)
	return &permissions.Policy{RestrictTools: true, AllowedTools: allowed}
}

func runtimeAgentProfile(item pluginmanager.CatalogPlugin) agent.Profile {
	if item.AgentProfile == nil {
		return agent.Profile{SystemPrompt: item.SystemPrompt}
	}
	return agent.Profile{
		ID:             item.AgentProfile.ID,
		Name:           item.AgentProfile.Name,
		Description:    item.AgentProfile.Description,
		SystemPrompt:   item.SystemPrompt,
		HandoffTargets: append([]string(nil), item.AgentProfile.Handoff.CanDelegateTo...),
	}
}

func resolveAgentPlugin(catalog *pluginmanager.Catalog, requested string) (pluginmanager.CatalogPlugin, error) {
	if catalog == nil || catalog.Empty() {
		return pluginmanager.CatalogPlugin{}, nil
	}
	agents := make([]pluginmanager.CatalogPlugin, 0)
	for _, item := range catalog.Plugins {
		if item.Type == plugin.TypeAgent && item.AgentProfile != nil {
			agents = append(agents, item)
		}
	}
	if len(agents) == 0 {
		return pluginmanager.CatalogPlugin{}, nil
	}
	requested = strings.TrimSpace(requested)
	if requested != "" {
		for _, item := range agents {
			if item.Name == requested || item.AgentProfile.ID == requested {
				return item, nil
			}
		}
		return pluginmanager.CatalogPlugin{}, fmt.Errorf("agent profile %q not found in supervisor-resolved catalog", requested)
	}
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].AgentProfile.ID < agents[j].AgentProfile.ID
	})
	return agents[0], nil
}

func resolveModelSelection(profile *plugin.AgentProfile, envProvider, envModel string) (string, string) {
	if profile != nil && profile.Model.Provider != "" && profile.Model.Model != "" {
		return profile.Model.Provider, profile.Model.Model
	}
	return envProvider, envModel
}

func servicePromptAddenda(catalog *runtimeservices.Catalog) []string {
	if catalog == nil || catalog.Empty() {
		return nil
	}
	return []string{catalog.Prompt()}
}

func registerServiceFunctions(a *agent.Agent, catalog *runtimeservices.Catalog) {
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

func serviceFunctionPendingRefs(catalog *runtimeservices.Catalog) func() []string {
	if catalog == nil || catalog.Empty() {
		return nil
	}
	return catalog.PendingEmbeddingRefs
}

func serviceFunctionToolResultRef(catalog *runtimeservices.Catalog) func(name, arguments, result string) (string, error) {
	if catalog == nil || catalog.Empty() {
		return nil
	}
	return catalog.CaptureToolResult
}

func serviceFunctionToolCallArgumentNormalizer(catalog *runtimeservices.Catalog) func(context.Context, string, string) (string, error) {
	if catalog == nil || catalog.Empty() {
		return nil
	}
	return catalog.NormalizeToolCallArguments
}

func loadRuntimeCatalogSnapshot(ctx context.Context) (*clientcontract.RuntimeCatalogResponse, error) {
	cfg := catalogclient.ConfigFromEnv()
	if !cfg.Available() {
		return nil, nil
	}
	snapshot, err := catalogclient.FetchRuntimeCatalog(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if len(snapshot.PluginCatalog) == 0 {
		return nil, fmt.Errorf("runtime catalog snapshot missing plugin catalog")
	}
	slog.Info("runtime catalog snapshot loaded", "space", snapshot.SpaceID, "generated_at", snapshot.GeneratedAt)
	return &snapshot, nil
}

func loadServiceCatalog(snapshot *clientcontract.RuntimeCatalogResponse) (*runtimeservices.Catalog, error) {
	if snapshot != nil && len(snapshot.ServiceCatalog) > 0 {
		descriptors, err := servicekit.UnmarshalRuntimeServiceCatalog(snapshot.ServiceCatalog)
		if err != nil {
			return nil, fmt.Errorf("parse nats runtime service catalog: %w", err)
		}
		return runtimeservices.NewCatalog(descriptors), nil
	}
	catalog, err := runtimeservices.CatalogFromEnv()
	if err != nil {
		return nil, err
	}
	if catalog == nil || catalog.Empty() {
		slog.Info("no supervisor-resolved service catalog provided")
		return nil, nil
	}
	for _, desc := range catalog.Descriptors() {
		slog.Info("service plugin loaded", "name", desc.GetName(), "type", desc.GetType(), "addr", desc.GetAddress())
	}
	return catalog, nil
}

func loadPluginCatalog(snapshot *clientcontract.RuntimeCatalogResponse) (*pluginmanager.Catalog, error) {
	raw := strings.TrimSpace(os.Getenv("QUARK_RUNTIME_PLUGIN_CATALOG"))
	if snapshot != nil && len(snapshot.PluginCatalog) > 0 {
		raw = string(snapshot.PluginCatalog)
	}
	if raw == "" {
		slog.Info("no supervisor-resolved plugin catalog provided; using empty catalog")
		catalog := plugin.NewRuntimeCatalog(nil)
		return &catalog, nil
	}
	var catalog pluginmanager.Catalog
	if err := json.Unmarshal([]byte(raw), &catalog); err != nil {
		return nil, fmt.Errorf("parse runtime plugin catalog: %w", err)
	}
	if err := catalog.Validate(); err != nil {
		return nil, fmt.Errorf("invalid runtime plugin catalog: %w", err)
	}
	if catalog.Empty() {
		slog.Info("supervisor-resolved plugin catalog is empty")
		return &catalog, nil
	}
	for _, item := range catalog.Plugins {
		slog.Info("plugin catalog entry loaded", "name", item.Name, "type", item.Type, "path", item.Path)
	}
	return &catalog, nil
}

func runtimeSpacesFromEnv() []string {
	values := make([]string, 0)
	add := func(value string) {
		for _, part := range strings.Split(value, ",") {
			space := strings.TrimSpace(part)
			if space != "" {
				values = append(values, space)
			}
		}
	}
	add(os.Getenv("QUARK_SPACES"))
	if len(values) == 0 {
		add(os.Getenv("QUARK_SPACE"))
	}
	seen := make(map[string]struct{}, len(values))
	out := values[:0]
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func claimRuntimeSpaces(ctx context.Context, spaces []string) (*spacelease.Manager, []*spacelease.Lease, error) {
	if len(spaces) == 0 {
		return nil, nil, nil
	}
	cfg := spacelease.ConfigFromEnv()
	if strings.TrimSpace(cfg.URL) == "" {
		slog.Warn("runtime space leases disabled because QUARK_NATS_URL is empty", "spaces", spaces)
		return nil, nil, nil
	}
	manager, err := spacelease.New(ctx, cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("runtime space lease manager: %w", err)
	}
	leases := make([]*spacelease.Lease, 0, len(spaces))
	for _, spaceID := range spaces {
		lease, err := manager.Claim(ctx, spaceID)
		if err != nil {
			releaseRuntimeSpaces(context.Background(), leases, manager)
			return nil, nil, err
		}
		lease.StartRenewal(ctx)
		leases = append(leases, lease)
		slog.Info("runtime space lease claimed", "space", spaceID, "runtime", lease.RuntimeID)
	}
	return manager, leases, nil
}

func releaseRuntimeSpaces(ctx context.Context, leases []*spacelease.Lease, manager *spacelease.Manager) {
	for _, lease := range leases {
		if err := lease.Release(ctx); err != nil {
			slog.Warn("release runtime space lease failed", "space", lease.SpaceID, "error", err)
		}
	}
	if manager != nil {
		manager.Close()
	}
}
