package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/quarkloop/pkg/plugin"
	"github.com/quarkloop/runtime/pkg/hierarchy"
	"github.com/quarkloop/runtime/pkg/message"
	"github.com/quarkloop/runtime/pkg/modelservice"
	"github.com/quarkloop/runtime/pkg/pluginmanager"
)

func TestSystemPromptIncludesConfiguredAddenda(t *testing.T) {
	a := newTestAgent(t)
	a.config.PromptAddenda = []string{"", "Use service functions for indexing."}

	got := a.systemPrompt()
	if !strings.Contains(got, "Use service functions for indexing.") {
		t.Fatalf("system prompt missing addendum:\n%s", got)
	}
}

func TestSystemPromptUsesResolvedAgentProfilePrompt(t *testing.T) {
	a := newTestAgentWithConfig(t, Config{
		ID:         "test-agent",
		PluginsDir: t.TempDir(),
		Profile: Profile{
			ID:           "quark-knowledge",
			Name:         "Quark Knowledge",
			SystemPrompt: "You are Quark Knowledge.",
		},
	})

	got := a.systemPrompt()
	if !strings.Contains(got, "You are Quark Knowledge.") {
		t.Fatalf("system prompt missing profile prompt:\n%s", got)
	}
	if strings.Contains(got, "Main Agent") {
		t.Fatalf("system prompt appears to use hardcoded main identity:\n%s", got)
	}
}

func TestSystemPromptIncludesResolvedHandoffPolicy(t *testing.T) {
	a := newTestAgentWithConfig(t, Config{
		ID:         "test-agent",
		PluginsDir: t.TempDir(),
		Profile: Profile{
			ID:             "quark-knowledge",
			SystemPrompt:   "You are Quark Knowledge.",
			HandoffTargets: []string{"quark-devops"},
		},
	})

	got := a.systemPrompt()
	if !strings.Contains(got, "Agent Handoffs") || !strings.Contains(got, "quark-devops") {
		t.Fatalf("system prompt missing handoff policy:\n%s", got)
	}
}

func TestSystemPromptIncludesRuntimeExtractionProfiles(t *testing.T) {
	a := newTestAgent(t)

	got := a.systemPrompt()
	for _, want := range []string{"Runtime Extraction Profiles", "`generic-open`", "IndexRequest.facts"} {
		if !strings.Contains(got, want) {
			t.Fatalf("system prompt missing extraction profile content %q:\n%s", want, got)
		}
	}
}

func TestSystemPromptIncludesWorkspaceSidecarPolicy(t *testing.T) {
	a := newTestAgent(t)

	got := a.systemPrompt()
	for _, want := range []string{"Workspace Sidecars", "explicit approval", "must not depend"} {
		if !strings.Contains(got, want) {
			t.Fatalf("system prompt missing workspace policy %q:\n%s", want, got)
		}
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

func TestSpawnSubAgentEnforcesResolvedHandoffPolicy(t *testing.T) {
	a := newTestAgentWithConfig(t, Config{
		ID:         "test-agent",
		PluginsDir: t.TempDir(),
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
	instrumented <- message.StreamMessage{Type: "tool_start", Data: map[string]any{"name": "indexer_GetContext"}}
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

func newTestAgent(t *testing.T) *Agent {
	t.Helper()
	return newTestAgentWithConfig(t, Config{ID: "test-agent", PluginsDir: t.TempDir()})
}

func newTestAgentWithConfig(t *testing.T, cfg Config) *Agent {
	t.Helper()
	a, err := NewAgent(cfg)
	if err != nil {
		t.Fatalf("new agent: %v", err)
	}
	return a
}
