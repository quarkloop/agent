package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/quarkloop/pkg/boundary"
	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/runtime/pkg/channel"
	"github.com/quarkloop/runtime/pkg/execution"
	"github.com/quarkloop/runtime/pkg/harnessclient"
	"github.com/quarkloop/runtime/pkg/hierarchy"
	"github.com/quarkloop/runtime/pkg/message"
	"github.com/quarkloop/runtime/pkg/modelservice"
	"github.com/quarkloop/runtime/pkg/permissions"
	"github.com/quarkloop/runtime/pkg/pluginmanager"
)

func TestPromptMaterialsIncludeConfiguredPluginMaterial(t *testing.T) {
	a := newTestAgent(t)
	a.config.PromptMaterials = []harnessclient.Material{{SourceID: "plugin.service.indexer.skill", Content: "Use service functions for indexing."}}

	got := a.promptMaterials()
	if len(got) != 1 || got[0].Content != "Use service functions for indexing." {
		t.Fatalf("prompt materials = %+v", got)
	}
}

func TestPromptMaterialsUseResolvedAgentProfilePrompt(t *testing.T) {
	a := newTestAgentWithConfig(t, Config{
		ID: "test-agent",
		Profile: Profile{
			ID:           "quark-knowledge",
			Name:         "Quark Knowledge",
			SystemPrompt: "You are Quark Knowledge.",
		},
	})

	got := a.promptMaterials()
	if len(got) != 1 || got[0].Content != "You are Quark Knowledge." {
		t.Fatalf("prompt materials missing profile prompt: %+v", got)
	}
	if got[0].SourceID != "plugin.agent.quark-knowledge.system" {
		t.Fatalf("prompt material provenance = %+v", got[0])
	}
}

func TestDefaultToolsComesFromPluginManager(t *testing.T) {
	a := newTestAgent(t)
	a.Plugins.RegisterRuntimeTool(pluginmanager.RuntimeTool{
		Schema: plugin.ToolSchema{Name: "runtime_echo", Description: "echo"},
		Handler: func(context.Context, string) (string, error) {
			return "ok", nil
		},
	})

	tools := a.defaultTools()
	if len(tools) != 1 || tools[0].Name != "runtime_echo" {
		t.Fatalf("default tools = %+v", tools)
	}
}

func TestDefaultToolsFiltersDeniedProfileFunctions(t *testing.T) {
	a := newTestAgentWithConfig(t, Config{
		ID:               "test-agent",
		PermissionPolicy: &permissions.Policy{RestrictTools: true, AllowedTools: []string{"gateway_Embed"}},
	})
	a.Plugins.RegisterRuntimeTool(pluginmanager.RuntimeTool{
		Schema: plugin.ToolSchema{Name: "gateway_Embed", Description: "embed"},
		Handler: func(context.Context, string) (string, error) {
			return "ok", nil
		},
	})
	a.Plugins.RegisterRuntimeTool(pluginmanager.RuntimeTool{
		Schema: plugin.ToolSchema{Name: "gateway_Embed", Description: "gateway embed"},
		Handler: func(context.Context, string) (string, error) {
			return "ok", nil
		},
	})

	tools := a.defaultTools()
	if len(tools) != 1 || tools[0].Name != "gateway_Embed" {
		t.Fatalf("filtered tools = %+v", tools)
	}
}

func TestDefaultToolsReturnsIndependentSchemaMaps(t *testing.T) {
	a := newTestAgent(t)
	a.Plugins.RegisterRuntimeTool(pluginmanager.RuntimeTool{
		Schema: plugin.ToolSchema{
			Name: "runtime_echo",
			Parameters: map[string]any{
				"properties": map[string]any{"value": map[string]any{"type": "string"}},
			},
		},
		Handler: func(context.Context, string) (string, error) {
			return "ok", nil
		},
	})

	first := a.defaultTools()
	first[0].Parameters["properties"].(map[string]any)["value"] = map[string]any{"type": "integer"}

	second := a.defaultTools()
	got := second[0].Parameters["properties"].(map[string]any)["value"].(map[string]any)["type"]
	if got != "string" {
		t.Fatalf("schema map mutation leaked into agent tool registry: %v", got)
	}
}

func TestAgentInitializesWorkflowStateStore(t *testing.T) {
	a := newTestAgent(t)
	if a.Workflows == nil {
		t.Fatal("workflow store was not initialized")
	}
}

func TestInitializationHandlersOwnModelAndChannelRegistration(t *testing.T) {
	a := newTestAgent(t)
	if err := a.handleInitLLM(context.Background(), InitLLMMsg{
		Fallback:  []plugin.ModelEntry{{ID: "test-model", Provider: "test", Name: "test-model", Default: true}},
		Providers: map[string]plugin.Provider{"test": staticProvider{reply: "ready"}},
	}); err != nil {
		t.Fatalf("initialize model: %v", err)
	}
	if a.Models.GetDefault() == nil {
		t.Fatal("initialization did not register default model")
	}
	bus := channel.NewChannelBus()
	if err := a.handleInitChannel(context.Background(), NewInitChannelMsg(bus)); err != nil {
		t.Fatalf("initialize channel: %v", err)
	}
	if a.Bus != bus {
		t.Fatal("initialization did not attach channel bus")
	}
}

func TestHandleUserMessageWithoutWorkflowDoesNotPanic(t *testing.T) {
	a := newTestAgent(t)
	a.Models.AddModel("test-model", staticProvider{reply: "hello from model"}, 0)
	a.Models.SetDefault("test-model")

	resp := make(chan message.StreamMessage, 8)
	err := a.handleUserMessage(context.Background(), NewUserMessage(context.Background(), message.PostRequest{
		SessionID: "session-1",
		Content:   "hello there",
	}, resp))
	if err != nil {
		t.Fatalf("handle user message: %v", err)
	}

	var reply string
	for msg := range resp {
		if msg.Type == "token" {
			reply += msg.Data.(string)
		}
	}
	if reply != "hello from model" {
		t.Fatalf("streamed reply = %q", reply)
	}
}

func TestExecuteToolRoutesThroughPluginManager(t *testing.T) {
	a := newTestAgent(t)
	a.Plugins.RegisterRuntimeTool(pluginmanager.RuntimeTool{
		Schema: plugin.ToolSchema{Name: "runtime_echo", Description: "echo"},
		Handler: func(ctx context.Context, arguments string) (string, error) {
			if arguments != `{"value":"hello"}` {
				t.Fatalf("arguments = %s", arguments)
			}
			return "hello", nil
		},
	})

	got, err := a.executeTool(context.Background(), "runtime_echo", `{"value":"hello"}`)
	if err != nil {
		t.Fatalf("execute tool: %v", err)
	}
	if got != "hello" {
		t.Fatalf("tool result = %q, want hello", got)
	}
}

func TestExecuteToolAppliesToolResultReferenceHook(t *testing.T) {
	a := newTestAgentWithConfig(t, Config{
		ID: "test-agent",
		ToolResultRef: func(name, arguments, result string) (string, error) {
			if name != "runtime_echo" || arguments != `{"value":"hello"}` || result != "hello" {
				t.Fatalf("unexpected hook input: %s %s %s", name, arguments, result)
			}
			return `{"contentRef":"content_1"}`, nil
		},
	})
	a.Plugins.RegisterRuntimeTool(pluginmanager.RuntimeTool{
		Schema: plugin.ToolSchema{Name: "runtime_echo", Description: "echo"},
		Handler: func(context.Context, string) (string, error) {
			return "hello", nil
		},
	})

	got, err := a.executeTool(context.Background(), "runtime_echo", `{"value":"hello"}`)
	if err != nil {
		t.Fatalf("execute tool: %v", err)
	}
	if got != `{"contentRef":"content_1"}` {
		t.Fatalf("tool result = %q", got)
	}
}

func TestExecuteToolUsesAssistiveApprovalGate(t *testing.T) {
	a := newTestAgentWithConfig(t, Config{
		ID: "test-agent",
		ExecutionCfg: execution.Config{
			Mode:            execution.ModeAssistive,
			ApprovalTimeout: time.Second,
		},
	})
	executed := make(chan struct{}, 1)
	a.Plugins.RegisterRuntimeTool(pluginmanager.RuntimeTool{
		Schema: plugin.ToolSchema{Name: "runtime_echo", Description: "echo"},
		Handler: func(context.Context, string) (string, error) {
			executed <- struct{}{}
			return "approved", nil
		},
	})

	type toolResult struct {
		output string
		err    error
	}
	resultCh := make(chan toolResult, 1)
	ctx := modelservice.WithSessionID(context.Background(), "session-1")
	go func() {
		output, err := a.executeTool(ctx, "runtime_echo", `{"value":"hello"}`)
		resultCh <- toolResult{output: output, err: err}
	}()

	var requestID string
	deadline := time.After(time.Second)
	for requestID == "" {
		select {
		case <-deadline:
			t.Fatal("approval request was not created")
		default:
			if pending := a.execution.PendingApprovals(); len(pending) > 0 {
				requestID = pending[0].ID
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	if !a.execution.Approve(requestID, "test approval") {
		t.Fatalf("approval %s was not accepted", requestID)
	}

	select {
	case got := <-resultCh:
		if got.err != nil || got.output != "approved" {
			t.Fatalf("execute tool = %q, %v", got.output, got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("tool execution did not resume after approval")
	}
	select {
	case <-executed:
	default:
		t.Fatal("tool handler did not run")
	}
}

func TestExecuteToolRequiresRuntimeApprovalForIOMutations(t *testing.T) {
	a := newTestAgent(t)
	executed := false
	a.Plugins.RegisterRuntimeTool(pluginmanager.RuntimeTool{
		Schema: plugin.ToolSchema{Name: "io_Remove", Description: "remove path"},
		Handler: func(context.Context, string) (string, error) {
			executed = true
			return "removed", nil
		},
	})

	_, err := a.executeTool(context.Background(), "io_Remove", `{"path":"/tmp/quark-e2e-space","approved":true}`)
	if !boundary.IsCategory(err, boundary.ApprovalRequired) {
		t.Fatalf("expected approval-required boundary error, got %v", err)
	}
	if executed {
		t.Fatal("io_Remove executed without runtime approval")
	}
}

func TestExecuteToolDeniesUnpermittedServiceFunctionAndRecordsPolicyEvent(t *testing.T) {
	a := newTestAgentWithConfig(t, Config{
		ID:               "test-agent",
		PermissionPolicy: &permissions.Policy{AllowedTools: []string{"io_Read"}},
	})
	executed := false
	a.Plugins.RegisterRuntimeTool(pluginmanager.RuntimeTool{
		Schema: plugin.ToolSchema{Name: "indexer_QueryContext", Description: "query context"},
		Handler: func(context.Context, string) (string, error) {
			executed = true
			return "should-not-run", nil
		},
	})

	ctx := modelservice.WithSessionID(context.Background(), "session-1")
	_, err := a.executeTool(ctx, "indexer_QueryContext", `{"authorization":"Bearer secret-value"}`)
	if err == nil {
		t.Fatal("expected permission denial")
	}
	if !boundary.IsCategory(err, boundary.PolicyDenied) {
		t.Fatalf("expected policy denied boundary error, got %v", err)
	}
	if executed {
		t.Fatal("service function handler ran despite permission denial")
	}
	records := a.Activity.List(10)
	if len(records) != 1 || records[0].Type != "policy.denied" || records[0].SessionID != "session-1" {
		t.Fatalf("activity records = %+v", records)
	}
	data := string(records[0].Data)
	if !strings.Contains(data, "indexer_QueryContext") || strings.Contains(data, "secret-value") {
		t.Fatalf("policy activity did not record safe denial details: %s", data)
	}
}

func TestSpawnSubAgentEnforcesResolvedHandoffPolicy(t *testing.T) {
	a := newTestAgentWithConfig(t, Config{
		ID: "test-agent",
		Profile: Profile{
			ID:             "quark-knowledge",
			HandoffTargets: []string{"quark-devops"},
		},
	})

	if _, err := a.SpawnSubAgent(&hierarchy.SpawnConfig{Name: "DevOps Worker", Task: "inspect build", ProfileID: "quark-devops"}); err != nil {
		t.Fatalf("allowed handoff spawn: %v", err)
	}
	if _, err := a.SpawnSubAgent(&hierarchy.SpawnConfig{Name: "System Worker", Task: "inspect system", ProfileID: "quark-system"}); err == nil {
		t.Fatal("expected disallowed handoff spawn error")
	}
}

func TestInstrumentResponseRecordsToolActivity(t *testing.T) {
	a := newTestAgent(t)
	downstream := make(chan message.StreamMessage, 1)
	instrumented, stop := a.instrumentResponse(context.Background(), "s1", downstream)
	instrumented <- message.StreamMessage{Type: "tool_start", Data: map[string]any{"name": "indexer_QueryContext"}}
	stop()

	records := a.Activity.List(10)
	if len(records) != 1 || records[0].Type != "tool_start" {
		t.Fatalf("activity records = %+v", records)
	}
	select {
	case msg := <-downstream:
		if msg.Type != "tool_start" {
			t.Fatalf("downstream message = %+v", msg)
		}
	default:
		t.Fatal("expected downstream tool event")
	}
}

func TestRecordModelUsageStoresRedactedSessionActivity(t *testing.T) {
	a := newTestAgent(t)
	ctx := modelservice.WithSessionID(context.Background(), "session-1")
	a.recordModelUsage(ctx, modelservice.Usage{
		Provider:      "openrouter",
		Model:         "openai/gpt-test",
		InputTokens:   11,
		OutputTokens:  7,
		FinishReason:  "stop",
		FallbackChain: []string{"openrouter"},
	})

	records := a.Activity.List(10)
	if len(records) != 1 || records[0].Type != "model.usage" || records[0].SessionID != "session-1" {
		t.Fatalf("activity records = %+v", records)
	}
	var usage modelservice.Usage
	if err := json.Unmarshal(records[0].Data, &usage); err != nil {
		t.Fatalf("decode usage: %v", err)
	}
	if usage.Provider != "openrouter" || usage.Model != "openai/gpt-test" || usage.InputTokens != 11 {
		t.Fatalf("usage data = %+v", usage)
	}
	if strings.Contains(string(records[0].Data), "prompt") || strings.Contains(string(records[0].Data), "arguments") {
		t.Fatalf("usage data leaked non-accounting payload: %s", records[0].Data)
	}
}

func TestEmitMessageErrorPropagatesBoundaryCategory(t *testing.T) {
	a := newTestAgent(t)
	response := make(chan message.StreamMessage, 1)
	err := plugin.NewProviderError(plugin.ProviderErrorRateLimit, "openrouter", "model-a", 429, nil)

	ctx := modelservice.WithRunID(context.Background(), "run-1")
	a.emitMessageError(ctx, "session-1", response, err)

	select {
	case msg := <-response:
		if msg.Type != "error" {
			t.Fatalf("stream message = %+v", msg)
		}
		payload, ok := msg.Data.(map[string]any)
		if !ok {
			t.Fatalf("stream payload type = %T", msg.Data)
		}
		if payload["boundary"] != string(boundary.Provider) || payload["category"] != string(boundary.RateLimit) {
			t.Fatalf("stream payload = %+v", payload)
		}
		if payload["session_id"] != "session-1" || payload["run_id"] != "run-1" || payload["diagnostic"] == nil {
			t.Fatalf("stream payload missing observability fields = %+v", payload)
		}
	default:
		t.Fatal("expected stream error payload")
	}
	records := a.Activity.List(10)
	if len(records) != 1 || records[0].Type != "message.error" {
		t.Fatalf("activity records = %+v", records)
	}
}

func newTestAgent(t *testing.T) *Agent {
	t.Helper()
	return newTestAgentWithConfig(t, Config{ID: "test-agent"})
}

func newTestAgentWithConfig(t *testing.T, cfg Config) *Agent {
	t.Helper()
	if cfg.ContextComposer == nil {
		cfg.ContextComposer = testComposer{}
	}
	a, err := NewAgent(cfg)
	if err != nil {
		t.Fatalf("new agent: %v", err)
	}
	return a
}

type testComposer struct{}

func (testComposer) Compose(_ context.Context, input harnessclient.Input) ([]plugin.Message, error) {
	var out []plugin.Message
	for _, material := range input.Materials {
		if material.Content != "" {
			out = append(out, plugin.Message{Role: "system", Content: material.Content})
		}
	}
	out = append(out, input.History...)
	return out, nil
}

type staticProvider struct {
	reply string
}

func (p staticProvider) ChatCompletionStream(context.Context, *plugin.ChatRequest) (<-chan plugin.StreamEvent, error) {
	ch := make(chan plugin.StreamEvent, 2)
	ch <- plugin.StreamEvent{Delta: p.reply}
	ch <- plugin.StreamEvent{Done: true}
	close(ch)
	return ch, nil
}

func (p staticProvider) ParseToolCalls(content string) ([]plugin.ToolCall, string) {
	return nil, content
}
