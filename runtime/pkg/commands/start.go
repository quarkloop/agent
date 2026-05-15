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
	"github.com/quarkloop/runtime/pkg/agent"
	"github.com/quarkloop/runtime/pkg/channel/telegram"
	"github.com/quarkloop/runtime/pkg/channel/web"
	"github.com/quarkloop/runtime/pkg/pluginmanager"
	"github.com/quarkloop/runtime/pkg/runtime"
	runtimeservices "github.com/quarkloop/runtime/pkg/services"
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
		} else {
			activeChannels[ch] = true
		}
	}

	if len(activeChannels) == 0 {
		return fmt.Errorf("no channels specified to start")
	}

	// 2. Early validation: fail fast if any channel is invalid
	var validChannels []string
	for ch := range activeChannels {
		switch ch {
		case "web", "telegram":
			validChannels = append(validChannels, ch)
		default:
			return fmt.Errorf("unknown channel requested: %q", ch)
		}
	}

	slog.Info("starting runtime")
	slog.Info("enabled channels", "channels", fmt.Sprintf("%v", validChannels))

	serviceCatalog, err := loadServiceCatalog()
	if err != nil {
		return err
	}
	pluginCatalog, err := loadPluginCatalog()
	if err != nil {
		return err
	}
	agentPlugin, err := resolveAgentPlugin(pluginCatalog, os.Getenv("QUARK_AGENT_PROFILE"))
	if err != nil {
		return err
	}

	modelProvider, modelName := resolveModelSelection(agentPlugin.AgentProfile, os.Getenv("QUARK_MODEL_PROVIDER"), os.Getenv("QUARK_MODEL_NAME"))
	if modelProvider == "" || modelName == "" {
		return fmt.Errorf("model provider and name are required")
	}
	slog.Info("using model", "provider", modelProvider, "model", modelName)

	promptAddenda := servicePromptAddenda(serviceCatalog)
	if strings.TrimSpace(agentPlugin.Skill) != "" {
		promptAddenda = append(promptAddenda, strings.TrimSpace(agentPlugin.Skill))
	}
	agentName := "Main Agent"
	agentDescription := ""
	if agentPlugin.AgentProfile != nil {
		agentName = agentPlugin.AgentProfile.Name
		agentDescription = agentPlugin.AgentProfile.Description
		slog.Info("using agent profile", "id", agentPlugin.AgentProfile.ID, "name", agentPlugin.AgentProfile.Name)
	}

	// Create agent
	a, err := agent.NewAgent(agent.Config{
		ID:            "main",
		Name:          agentName,
		Description:   agentDescription,
		ModelProvider: modelProvider,
		Model:         modelName,
		ModelListURL:  os.Getenv("MODEL_LIST_URL"),
		SystemPrompt:  agentPlugin.SystemPrompt,
		PluginsDir:    os.Getenv("QUARK_PLUGINS_DIR"),
		PluginCatalog: pluginCatalog,
		SupervisorURL: os.Getenv("QUARK_SUPERVISOR_URL"),
		SpaceID:       os.Getenv("QUARK_SPACE"),
		PromptAddenda: promptAddenda,
		PendingRefs:   serviceFunctionPendingRefs(serviceCatalog),
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
		}
	}

	// Start agent loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	slog.Info("starting agent background loop")
	go a.Run(ctx)

	slog.Info("runtime server is running, press Ctrl+C to exit")
	// Start all channels via ChannelBus and block
	return srv.Run(ctx)
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

func loadServiceCatalog() (*runtimeservices.Catalog, error) {
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

func loadPluginCatalog() (*pluginmanager.Catalog, error) {
	raw := strings.TrimSpace(os.Getenv("QUARK_RUNTIME_PLUGIN_CATALOG"))
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
