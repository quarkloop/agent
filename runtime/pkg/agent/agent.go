// Package agent provides the core agent with typed message loop.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/quarkloop/pkg/boundary"
	event "github.com/quarkloop/pkg/event"
	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/runtime/pkg/activity"
	"github.com/quarkloop/runtime/pkg/channel"
	"github.com/quarkloop/runtime/pkg/coreevents"
	"github.com/quarkloop/runtime/pkg/execution"
	"github.com/quarkloop/runtime/pkg/guard"
	"github.com/quarkloop/runtime/pkg/handoff"
	"github.com/quarkloop/runtime/pkg/hierarchy"
	"github.com/quarkloop/runtime/pkg/llm"
	"github.com/quarkloop/runtime/pkg/loop"
	"github.com/quarkloop/runtime/pkg/message"
	"github.com/quarkloop/runtime/pkg/modelservice"
	"github.com/quarkloop/runtime/pkg/modelusage"
	"github.com/quarkloop/runtime/pkg/permissions"
	"github.com/quarkloop/runtime/pkg/plan"
	"github.com/quarkloop/runtime/pkg/pluginmanager"
	"github.com/quarkloop/runtime/pkg/prompt"
	"github.com/quarkloop/runtime/pkg/session"
	"github.com/quarkloop/runtime/pkg/toolpolicy"
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

// sendInitMessages queues initialization messages at startup.
func (a *Agent) sendInitMessages() {
	// Get providers loaded from plugins
	providers := a.Plugins.GetProviders()

	// Log loaded providers
	if len(providers) == 0 {
		slog.Warn("no providers loaded from plugins")
	}
	for id := range providers {
		slog.Info("provider available", "id", id)
	}

	fallback := []plugin.ModelEntry{}
	if a.config.Model != "" {
		fallback = append(fallback, plugin.ModelEntry{
			ID:       a.config.Model,
			Provider: a.config.ModelProvider,
			Name:     a.config.Model,
			Default:  true,
		})
	}

	msg := NewInitLLMMsg()
	msg.ModelListURL = a.config.ModelListURL
	msg.Providers = providers
	msg.Fallback = fallback

	a.loop.Send(msg)
}

// workStepTicker periodically triggers work step execution.
func (a *Agent) workStepTicker(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Check if there's work to do
			select {
			case <-a.Plan.NextStep():
				a.loop.Send(NewWorkStepMsg())
			default:
			}
		}
	}
}

// handleUserMessage processes an incoming user message.
func (a *Agent) handleUserMessage(ctx context.Context, msg loop.Message) error {
	userMsg := msg.(UserMessageMsg)
	defer close(userMsg.Response)

	runID := newRunID()
	requestCtx, cancel := context.WithCancel(ctx)
	spaceID := userMsg.SpaceID
	if spaceID == "" {
		spaceID = a.Space
	}
	requestCtx = modelservice.WithSpaceID(requestCtx, spaceID)
	requestCtx = modelservice.WithSessionID(requestCtx, userMsg.SessionID)
	requestCtx = modelservice.WithRunID(requestCtx, runID)
	defer cancel()
	stopRequestCancel := context.AfterFunc(userMsg.Context, cancel)
	defer stopRequestCancel()
	response := userMsg.Response
	if a.Activity != nil {
		a.addActivity(userMsg.SessionID, "message.user", map[string]any{
			"content_length": len(userMsg.Content),
			"run_id":         runID,
		})
		instrumented, stopForwarding := a.instrumentResponse(requestCtx, userMsg.SessionID, userMsg.Response)
		response = instrumented
		defer stopForwarding()
	}
	slog.Info("agent message started", "session_id", userMsg.SessionID, "run_id", runID, "content_length", len(userMsg.Content))

	s := a.Sessions.Get(userMsg.SessionID)
	if s == nil {
		s = a.Sessions.GetOrCreate(userMsg.SessionID, "chat", "")
	}

	s.AddMessage("user", userMsg.Content)

	client := a.Models.GetDefault()
	if client == nil {
		return fmt.Errorf("no LLM client configured")
	}
	if a.config.ToolCallArguments != nil {
		requestCtx = llm.WithToolCallArgumentNormalizer(requestCtx, a.config.ToolCallArguments)
	}

	history := s.GetMessages()
	tools := a.defaultTools()
	workflowTracker := workflow.NewTracker(userMsg.SessionID, userMsg.Content, tools, a.Workflows, func(event workflow.Event) {
		a.addActivity(userMsg.SessionID, "workflow."+event.Type, event)
	})
	systemPrompt := a.systemPrompt()
	onTool := a.executeTool
	var workflowGuard llm.FinalGuard
	var workflowToolCallGate llm.ToolCallGate
	var workflowToolCallGuard llm.ToolCallGuard
	var workflowToolResultGate llm.ToolResultGate
	var workflowToolResultInstruction llm.ToolResultInstruction
	if workflowTracker != nil {
		if block := workflowTracker.PromptBlock(); block != "" {
			systemPrompt = systemPrompt + "\n\n" + block
		}
		onTool = workflowTracker.WrapToolHandler(onTool)
		workflowGuard = workflowTracker.FinalGuard
		workflowToolCallGate = workflowTracker.AcceptFinalBeforeToolCalls
		workflowToolCallGuard = workflowTracker.GuardToolCalls
		workflowToolResultGate = workflowTracker.AcceptFinalAfterToolCalls
		workflowToolResultInstruction = workflowTracker.ContinuationPrompt
	}
	fullResponse, err := message.HandleWithToolCallPolicyAndContinuation(
		requestCtx,
		history,
		client,
		systemPrompt,
		a.Plan.GetSummary(),
		tools,
		onTool,
		response,
		guard.CombineFinalGuards(a.finalGuard(), workflowGuard),
		workflowToolCallGate,
		workflowToolCallGuard,
		workflowToolResultGate,
		workflowToolResultInstruction,
	)
	if err != nil {
		a.emitMessageError(requestCtx, userMsg.SessionID, response, err)
		slog.Error("agent message failed", "session_id", userMsg.SessionID, "run_id", runID, "error", err)
		return err
	}
	if a.config.PendingRefs != nil {
		if refs := a.config.PendingRefs(); len(refs) > 0 {
			err := guard.UnconsumedPendingRefsError(refs)
			a.emitMessageError(requestCtx, userMsg.SessionID, response, err)
			slog.Error("agent message failed", "session_id", userMsg.SessionID, "run_id", runID, "error", err)
			return err
		}
	}

	s.AddMessage("assistant", fullResponse)
	if a.Activity != nil {
		a.addActivity(userMsg.SessionID, "message.assistant", map[string]any{
			"content_length": len(fullResponse),
			"run_id":         runID,
		})
	}
	slog.Info("agent message completed", "session_id", userMsg.SessionID, "run_id", runID, "content_length", len(fullResponse))
	return nil
}

func newRunID() string {
	return fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
}

func (a *Agent) instrumentResponse(ctx context.Context, sessionID string, downstream chan message.StreamMessage) (chan message.StreamMessage, func()) {
	upstream := make(chan message.StreamMessage, 64)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for msg := range upstream {
			a.recordStreamActivity(sessionID, msg)
			if !message.Emit(ctx, downstream, msg) {
				return
			}
		}
	}()
	return upstream, func() {
		close(upstream)
		<-done
	}
}

func (a *Agent) recordStreamActivity(sessionID string, msg message.StreamMessage) {
	if a.Activity == nil {
		return
	}
	switch msg.Type {
	case "tool_start", "tool_result":
		a.addActivity(sessionID, msg.Type, msg.Data)
	case "error":
		a.addActivity(sessionID, "message.error", map[string]any{"error": fmt.Sprint(msg.Data)})
	}
}

func (a *Agent) emitMessageError(ctx context.Context, sessionID string, response chan message.StreamMessage, err error) {
	payload := boundary.StreamPayload(err, boundary.Runtime, "message")
	if runID := modelservice.RunID(ctx); runID != "" {
		payload["run_id"] = runID
	}
	if sessionID != "" {
		payload["session_id"] = sessionID
	}
	if messageText, ok := payload["message"].(string); ok {
		payload["message"] = fmt.Sprintf("Agent Error: %s", messageText)
	}
	if a.Activity != nil {
		a.addActivity(sessionID, "message.error", payload)
	}
	message.Emit(ctx, response, message.StreamMessage{
		Type: "error",
		Data: payload,
	})
}

// handleInitLLM initializes or reinitializes LLM models.
func (a *Agent) handleInitLLM(ctx context.Context, msg loop.Message) error {
	payload := msg.(InitLLMMsg)
	slog.Info("initializing LLM models")

	providers := payload.Providers
	models := modelservice.New(providers, a.recordModelUsage)

	if payload.ModelListURL != "" {
		if err := a.Models.LoadFromURLWithGatewayService(payload.ModelListURL, models); err != nil {
			slog.Warn("remote model list failed, using fallback", "error", err)
		}
	}

	// Fallback: load from config if registry is empty
	if a.Models.GetDefault() == nil && len(payload.Fallback) > 0 {
		if len(payload.Fallback) > 0 {
			if err := a.Models.LoadEntriesWithGatewayService(payload.Fallback, models); err != nil {
				slog.Error("fallback model init failed", "error", err)
			}
		}
	}

	if client := a.Models.GetDefault(); client != nil {
		slog.Info("LLM ready", "default_model", a.Models.Default)
	} else {
		slog.Warn("no LLM models available")
	}

	return nil
}

func (a *Agent) recordModelUsage(ctx context.Context, usage modelservice.Usage) {
	sessionID := usage.SessionID
	if sessionID == "" {
		sessionID = modelservice.SessionID(ctx)
		usage.SessionID = sessionID
	}
	if a.Activity == nil {
		a.persistModelUsage(usage)
		return
	}
	a.addActivity(sessionID, "model.usage", usage)
	a.persistModelUsage(usage)
}

func (a *Agent) addActivity(sessionID, typ string, data any) activity.Record {
	if a.Activity == nil {
		return activity.Record{}
	}
	record := a.Activity.Add(sessionID, typ, data)
	if a.core != nil {
		a.core.Record(record)
	}
	return record
}

func (a *Agent) persistModelUsage(usage modelservice.Usage) {
	if a.supervisorClient == nil || a.Space == "" {
		return
	}
	usage.FallbackChain = append([]string(nil), usage.FallbackChain...)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := modelusage.Persist(ctx, a.supervisorClient, a.Space, usage, time.Now().UTC()); err != nil {
			slog.Warn("persist model usage failed", "error", err)
		}
	}()
}

// handleInitChannel processes channel state changes.
func (a *Agent) handleInitChannel(ctx context.Context, msg loop.Message) error {
	payload := msg.(InitChannelMsg)
	if bus, ok := payload.Bus.(*channel.ChannelBus); ok {
		a.Bus = bus
		slog.Info("channel bus registered", "active_channels", len(a.Bus.ActiveChannels()))
	}
	return nil
}

// handleSetModel dynamically changes the active LLM model.
func (a *Agent) handleSetModel(ctx context.Context, msg loop.Message) error {
	payload := msg.(SetModelMsg)
	if a.Models.SetDefault(payload.ModelID) {
		slog.Info("switched default model", "model_id", payload.ModelID)
	} else {
		slog.Warn("model not found in registry", "model_id", payload.ModelID)
	}
	return nil
}

// handleWorkStep executes the next autonomous work step.
func (a *Agent) handleWorkStep(ctx context.Context, msg loop.Message) error {
	client := a.Models.GetDefault()
	if client == nil {
		return nil
	}

	infer := func(ctx context.Context, messages []plugin.Message, tools []plugin.ToolSchema, onTool plugin.ToolHandler, onMessage func(string, any)) (string, error) {
		return client.Infer(ctx, messages, tools, onTool, onMessage, a.finalGuard())
	}
	if err := a.Plan.ExecuteStep(ctx, infer, a.systemPrompt(), a.defaultTools(), a.executeTool); err != nil {
		slog.Error("work step error", "error", err)
		return err
	}
	return nil
}

// handleToolCall executes a tool call (with permission checking via middleware).
func (a *Agent) handleToolCall(ctx context.Context, msg loop.Message) error {
	toolMsg := msg.(ToolCallMsg)

	result, err := a.executeTool(ctx, toolMsg.Tool, toolMsg.Arguments)
	toolMsg.ResultChan <- AgentToolResult{Output: result, Error: err}
	return err
}

// processWork processes delegated work from a sub-agent.
func (a *Agent) processWork(ctx context.Context, agentID, task string) (string, error) {
	client := a.Models.GetDefault()
	if client == nil {
		return "", fmt.Errorf("no LLM client configured")
	}

	// Simple inference for sub-agent work
	msgs := []plugin.Message{
		{Role: "system", Content: a.systemPrompt()},
		{Role: "user", Content: task},
	}

	return client.Infer(ctx, msgs, a.defaultTools(), a.executeTool, nil, nil)
}

// defaultTools returns the available tools.
func (a *Agent) defaultTools() []plugin.ToolSchema {
	tools := a.Plugins.GetTools()
	if len(tools) == 0 {
		return nil
	}
	filtered := make([]plugin.ToolSchema, 0, len(tools))
	for _, tool := range tools {
		if a.permissions != nil && !a.permissions.CanUseTool(tool.Name) {
			continue
		}
		filtered = append(filtered, cloneToolSchema(tool))
	}
	return filtered
}

func cloneToolSchema(schema plugin.ToolSchema) plugin.ToolSchema {
	schema.Parameters = cloneSchemaMap(schema.Parameters)
	return schema
}

func cloneSchemaMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = cloneSchemaValue(value)
	}
	return out
}

func cloneSchemaValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneSchemaMap(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = cloneSchemaValue(item)
		}
		return out
	default:
		return value
	}
}

func (a *Agent) systemPrompt() string {
	addenda := make([]string, 0, len(a.config.PromptAddenda)+1)
	addenda = append(addenda, a.config.PromptAddenda...)
	if block := a.handoff.PromptBlock(); block != "" {
		addenda = append(addenda, block)
	}
	return prompt.BuildRuntimeSystemPrompt(a.profile.SystemPrompt, addenda)
}

func (a *Agent) finalGuard() llm.FinalGuard {
	return guard.PendingEmbeddingRefs(a.config.PendingRefs, 8)
}

// executeTool executes a requested tool through the runtime tool manager.
func (a *Agent) executeTool(ctx context.Context, name, arguments string) (string, error) {
	// Check permissions
	if err := a.permissions.ValidateTool(name); err != nil {
		return "", a.toolPolicyDeniedError(ctx, name, arguments)
	}
	runtimeApproved, err := a.requireToolApproval(ctx, name, arguments)
	if err != nil {
		return "", err
	}
	if err := toolpolicy.Validate(toolpolicy.Invocation{
		Name:            name,
		Arguments:       arguments,
		RuntimeApproved: runtimeApproved,
	}); err != nil {
		return "", err
	}
	result, err := a.Plugins.ExecuteTool(ctx, name, arguments)
	if err != nil {
		return "", err
	}
	if a.config.ToolResultRef == nil {
		return result, nil
	}
	return a.config.ToolResultRef(name, arguments, result)
}

func (a *Agent) toolPolicyDeniedError(ctx context.Context, name, arguments string) error {
	sessionID := modelservice.SessionID(ctx)
	runID := modelservice.RunID(ctx)
	if a.Activity != nil {
		a.addActivity(sessionID, "policy.denied", map[string]any{
			"tool":             name,
			"reason":           "tool_not_allowed",
			"arguments_length": len(arguments),
			"run_id":           runID,
		})
	}
	return boundary.New(
		boundary.Runtime,
		boundary.PolicyDenied,
		"tool."+name,
		fmt.Sprintf("tool %q is not allowed by the active agent policy", name),
	)
}

func (a *Agent) requireToolApproval(ctx context.Context, name, arguments string) (bool, error) {
	if a.execution == nil || a.execution.Mode() != execution.ModeAssistive || a.execution.Gate() == nil {
		return false, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	sessionID := modelservice.SessionID(ctx)
	if err := a.execution.Gate().RequestApproval(ctx, name, arguments, sessionID); err != nil {
		return false, fmt.Errorf("tool call approval failed for %s: %w", name, err)
	}
	return true, nil
}

// Identity returns the agent's hierarchy identity.
func (a *Agent) Identity() *hierarchy.Identity {
	return a.identity
}

// Agents returns the hierarchy registry.
func (a *Agent) Agents() *hierarchy.Registry {
	return a.agents
}

// Delegator returns the work delegator.
func (a *Agent) Delegator() *hierarchy.Delegator {
	return a.delegator
}

// Execution returns the execution runtime.
func (a *Agent) Execution() *execution.ExecutionRuntime {
	return a.execution
}

// Permissions returns the permission checker.
func (a *Agent) Permissions() *permissions.Checker {
	return a.permissions
}

// SpawnSubAgent spawns a new sub-agent with the given configuration.
func (a *Agent) SpawnSubAgent(config *hierarchy.SpawnConfig) (*hierarchy.AgentEntry, error) {
	if config != nil && config.ProfileID != "" {
		if err := a.handoff.ValidateTarget(config.ProfileID); err != nil {
			return nil, err
		}
	}
	return a.agents.Spawn(a.ID, config)
}

// DelegateWork delegates work to a sub-agent.
func (a *Agent) DelegateWork(ctx context.Context, agentID, task string, timeout time.Duration) (hierarchy.WorkResult, error) {
	work := hierarchy.WorkItem{
		BaseMessage: loop.NewPriorityMessage("work_item", 5),
		AgentID:     agentID,
		Task:        task,
		Timeout:     timeout,
	}
	return a.delegator.DelegateAndWait(ctx, a.ID, work)
}

// Supervisor returns the supervisor client, or nil if the agent is running
// standalone.
func (a *Agent) Supervisor() *supclient.Client {
	return a.supervisorClient
}

// HasSupervisor returns true if the agent is running under a supervisor.
func (a *Agent) HasSupervisor() bool {
	return a.supervisorClient != nil
}

// subscribeSupervisorEvents consumes the supervisor's space event stream and
// mirrors session lifecycle events into the local in-memory registry so the
// agent can serve messages for sessions the supervisor has created. The call
// returns when ctx is cancelled or the stream terminates; callers should
// reconnect with backoff.
func (a *Agent) subscribeSupervisorEvents(ctx context.Context) {
	if a.supervisorClient == nil || a.Space == "" {
		slog.Info("supervisor event stream disabled", "client", a.supervisorClient != nil, "space", a.Space)
		return
	}
	slog.Info("subscribing to supervisor events", "space", a.Space)
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		err := a.supervisorClient.StreamEventsWithReady(ctx, a.Space,
			func() {
				slog.Info("supervisor event stream ready", "space", a.Space)
				a.syncSupervisorSessions(ctx)
			},
			func(ev event.Event) { a.applyEvent(ev) },
		)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			slog.Error("supervisor event stream error, retrying", "error", err, "retry_in", backoff)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (a *Agent) syncSupervisorSessions(ctx context.Context) {
	if a.supervisorClient == nil || a.Space == "" {
		return
	}
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	sessions, err := a.supervisorClient.ListSessions(callCtx, a.Space)
	if err != nil {
		slog.Warn("sync supervisor sessions failed", "space", a.Space, "error", err)
		return
	}
	for _, sess := range sessions {
		if sess.ID == "" {
			continue
		}
		a.Sessions.GetOrCreate(sess.ID, string(sess.Type), sess.Title)
	}
	slog.Info("synced supervisor sessions", "space", a.Space, "count", len(sessions))
}

// applyEvent updates agent runtime state in response to a supervisor event.
func (a *Agent) applyEvent(ev event.Event) {
	switch ev.Kind {
	case event.SessionCreated:
		var p struct {
			ID    string `json:"id"`
			Type  string `json:"type"`
			Title string `json:"title"`
		}
		if err := json.Unmarshal(ev.Payload, &p); err != nil || p.ID == "" {
			return
		}
		a.Sessions.GetOrCreate(p.ID, p.Type, p.Title)
		slog.Info("session created", "id", p.ID, "type", p.Type)
	case event.SessionDeleted:
		var p struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(ev.Payload, &p); err != nil || p.ID == "" {
			return
		}
		a.Sessions.Delete(p.ID)
		slog.Info("session deleted", "id", p.ID)
	}
}
