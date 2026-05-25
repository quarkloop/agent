// Package agent provides the core agent with typed message loop.
package agent

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/runtime/pkg/activity"
	"github.com/quarkloop/runtime/pkg/channel"
	"github.com/quarkloop/runtime/pkg/coreevents"
	"github.com/quarkloop/runtime/pkg/execution"
	"github.com/quarkloop/runtime/pkg/handoff"
	"github.com/quarkloop/runtime/pkg/hierarchy"
	"github.com/quarkloop/runtime/pkg/llm"
	"github.com/quarkloop/runtime/pkg/loop"
	"github.com/quarkloop/runtime/pkg/message"
	"github.com/quarkloop/runtime/pkg/permissions"
	"github.com/quarkloop/runtime/pkg/plan"
	"github.com/quarkloop/runtime/pkg/pluginmanager"
	"github.com/quarkloop/runtime/pkg/session"
	"github.com/quarkloop/runtime/pkg/workflow"
	supclient "github.com/quarkloop/supervisor/pkg/client"
)

// Config holds agent configuration.
type Config struct {
	ID                   string
	Name                 string
	Description          string
	ModelProvider        string
	Model                string
	ModelListURL         string
	Profile              Profile
	SystemPrompt         string
	PluginsDir           string
	PluginCatalog        *pluginmanager.Catalog
	PromptAddenda        []string
	PendingRefs          func() []string
	ToolResultRef        func(name, arguments, result string) (string, error)
	ToolCallArguments    llm.ToolCallArgumentNormalizer
	CoreEvents           *coreevents.Recorder
	ModelProviderAdapter plugin.Provider

	// Execution mode configuration
	ExecutionMode execution.Mode
	ExecutionCfg  execution.Config

	// Permission policy
	PermissionPolicy *permissions.Policy

	// Supervisor configuration (optional - for agents running under supervisor)
	SupervisorURL string
	SpaceID       string
}

// Agent is the main agent instance with a typed message loop.
type Agent struct {
	ID        string
	loop      *loop.Loop
	Sessions  *session.Registry
	Plan      *plan.Plan
	Models    *llm.Registry
	Plugins   *pluginmanager.Manager
	Bus       *channel.ChannelBus
	Activity  *activity.Store
	Workflows *workflow.Store
	core      *coreevents.Recorder
	config    Config

	// Hierarchy management
	identity  *hierarchy.Identity
	agents    *hierarchy.Registry
	delegator *hierarchy.Delegator
	profile   Profile
	handoff   handoff.Policy

	// Execution runtime
	execution *execution.ExecutionRuntime

	// Permission checker
	permissions *permissions.Checker

	// Supervisor client for all space-scoped data operations.
	// Nil only when the agent is running standalone (no supervisor URL).
	supervisorClient *supclient.Client
	// Space is the space name this agent serves; empty when standalone.
	Space string
}

// NewAgent creates a new Agent from config.
func NewAgent(cfg Config) (*Agent, error) {
	// Create the message loop
	l := loop.New(
		loop.WithInboxSize(64),
		loop.WithWorkQueueSize(32),
		loop.WithWorkPriority(true),
		loop.WithUnhandledCallback(func(msg loop.Message) {
			slog.Info("unhandled message", "type", msg.Type())
		}),
	)

	// Create execution runtime
	execCfg := cfg.ExecutionCfg
	if execCfg.Mode == "" {
		execCfg.Mode = execution.ModeAutonomous
	}
	execRuntime, err := execution.NewExecutionRuntime(execCfg)
	if err != nil {
		return nil, fmt.Errorf("execution runtime: %w", err)
	}

	// Create hierarchy registry
	agentRegistry := hierarchy.NewRegistry()
	profile := cfg.Profile.normalize(cfg.ID, cfg.Name, cfg.Description, cfg.SystemPrompt)
	handoffPolicy := handoff.NewPolicy(profile.ID, profile.HandoffTargets)

	// Determine plugins directory
	pluginsDir := cfg.PluginsDir
	if pluginsDir == "" {
		pluginsDir = "plugins"
	}

	a := &Agent{
		ID:          cfg.ID,
		loop:        l,
		Sessions:    session.NewRegistry(),
		Plan:        plan.New(),
		Models:      llm.NewRegistry(),
		Plugins:     pluginmanager.NewManager(pluginsDir),
		Activity:    activity.NewStore(1000),
		Workflows:   workflow.NewStore(),
		core:        cfg.CoreEvents,
		config:      cfg,
		agents:      agentRegistry,
		delegator:   hierarchy.NewDelegator(agentRegistry),
		profile:     profile,
		handoff:     handoffPolicy,
		execution:   execRuntime,
		permissions: permissions.NewChecker(cfg.PermissionPolicy),
	}
	a.Plugins.SetCatalog(cfg.PluginCatalog)

	// Create supervisor client if URL is provided
	if cfg.SupervisorURL != "" {
		a.supervisorClient = supclient.New(supclient.WithBaseURL(cfg.SupervisorURL))
		a.Space = cfg.SpaceID
	}

	// Register as main agent
	name := profile.Name
	if name == "" {
		name = "Main Agent"
	}
	entry, err := agentRegistry.RegisterMainWithProfile(cfg.ID, name, profile.Description, profile.ID, hierarchy.DefaultPermissions())
	if err != nil {
		return nil, fmt.Errorf("register main agent: %w", err)
	}
	a.identity = entry.Identity

	// Register this agent's loop
	a.delegator.RegisterLoop(cfg.ID, l)

	// Register handlers
	a.registerHandlers()

	// Configure execution middleware
	execRuntime.ConfigureLoop(l)

	// Add permission middleware if policy is set
	if cfg.PermissionPolicy != nil {
		l.Use(permissions.ToolMiddleware(a.permissions))
	}

	// Add recovery middleware
	l.Use(loop.RecoveryMiddleware)

	// Add observer middleware for logging
	l.Use(loop.ObserverMiddleware(func(msgType string, err error) {
		if err != nil {
			slog.Error("handler error", "type", msgType, "error", err)
		}
	}))

	return a, nil
}

// registerHandlers registers all message handlers.
func (a *Agent) registerHandlers() {
	a.loop.Register(MsgTypeUserMessage, a.handleUserMessage)
	a.loop.Register(MsgTypeInitLLM, a.handleInitLLM)
	a.loop.Register(MsgTypeInitChannel, a.handleInitChannel)
	a.loop.Register(MsgTypeSetModel, a.handleSetModel)
	a.loop.Register(MsgTypeWorkStep, a.handleWorkStep)
	a.loop.Register(MsgTypeToolCall, a.handleToolCall)

	// Register work item handler for delegation
	a.loop.Register("work_item", hierarchy.WorkHandler(a.agents, a.processWork))
}

// Post sends a user message to the agent.
func (a *Agent) Post(ctx context.Context, request message.PostRequest, resp chan message.StreamMessage) {
	a.loop.Send(NewUserMessage(ctx, request, resp))
}

// Send sends a typed message to the agent loop.
func (a *Agent) Send(msg loop.Message) {
	a.loop.Send(msg)
}

// Run starts the agent's main loop.
func (a *Agent) Run(ctx context.Context) error {
	slog.Info("main loop started", "agent_id", a.ID)
	defer a.core.Close()

	// Initialize loads supervisor-resolved tool plugins. Model providers are
	// registered separately through Gateway-backed runtime adapters.
	if err := a.Plugins.Initialize(ctx); err != nil {
		slog.Error("failed to initialize plugins", "error", err)
	}
	if a.config.ModelProviderAdapter != nil {
		a.Plugins.RegisterRuntimeProvider(a.config.ModelProvider, a.config.ModelProviderAdapter)
	}
	defer a.Plugins.Shutdown()

	// Update agent status
	a.agents.SetStatus(a.ID, hierarchy.StatusRunning)
	defer a.agents.SetStatus(a.ID, hierarchy.StatusComplete)

	// Send initialization messages
	a.sendInitMessages()

	// Start work step ticker for plan execution
	go a.workStepTicker(ctx)

	// Subscribe to supervisor events (session lifecycle, shutdown, etc).
	// This is the agent's only mechanism for learning about sessions — the
	// agent no longer exposes its own session CRUD API.
	go a.subscribeSupervisorEvents(ctx)

	// Run the loop
	return a.loop.Run(ctx)
}
