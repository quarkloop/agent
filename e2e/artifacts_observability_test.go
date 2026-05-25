//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/quarkloop/e2e/utils"
	"github.com/quarkloop/supervisor/pkg/api"
)

func TestAgentRunArtifactsAreRedactedAndStructured(t *testing.T) {
	secret := "sk-or-v1-super-secret-token"
	t.Setenv("OPENROUTER_API_KEY", secret)
	dir := t.TempDir()
	env := &utils.E2EEnv{
		Space:  "e2e-artifact-test",
		SupURL: "http://127.0.0.1:7200",
		Agent: api.RuntimeInfo{
			ID: "runtime-1",
		},
		Provider: "openrouter",
		Model:    "test/model",
		Embedding: utils.GatewayEmbeddingOptions{
			Provider:   "openrouter",
			Model:      "fixture/embed",
			Dimensions: 32,
		},
	}
	trace := utils.MessageTrace{
		Text:       "reply with " + secret,
		Space:      env.Space,
		SessionID:  "session-1",
		RunID:      "run-1",
		ToolStarts: []string{"gateway_Embed"},
		ToolStartEvents: []utils.ToolEvent{{
			CallID:        "call-1",
			ServiceCallID: "call-1",
			Name:          "gateway_Embed",
			Arguments:     `{"authorization":"Bearer ` + secret + `"}`,
			SessionID:     "session-1",
			RunID:         "run-1",
			ObservedAt:    "2026-05-18T10:00:00Z",
		}},
		ToolResultEvents: []utils.ToolEvent{{
			CallID:         "call-1",
			ServiceCallID:  "call-1",
			Name:           "gateway_Embed",
			Subject:        "svc.gateway.v1.embed",
			Result:         `{"api_key":"` + secret + `"}`,
			SessionID:      "session-1",
			RunID:          "run-1",
			ObservedAt:     "2026-05-18T10:00:01Z",
			DurationMillis: 25,
		}},
	}

	artifacts := writeAgentRunArtifacts(t, dir, "agent-run", env, trace, "prompt "+secret)
	for _, path := range []string{artifacts.Reply, artifacts.Tools, artifacts.ToolEvents, artifacts.Observability} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read artifact %s: %v", path, err)
		}
		if containsText(string(data), secret) {
			t.Fatalf("artifact %s leaked secret:\n%s", filepath.Base(path), string(data))
		}
	}

	var payload struct {
		ArtifactID      string           `json:"artifact_id"`
		SessionID       string           `json:"session_id"`
		RunID           string           `json:"run_id"`
		PromptSHA256    string           `json:"prompt_sha256"`
		Model           map[string]any   `json:"model"`
		Embedding       map[string]any   `json:"embedding"`
		CatalogSnapshot map[string]any   `json:"catalog_snapshot"`
		ProfileSnapshot map[string]any   `json:"profile_snapshot"`
		ModelUsage      map[string]any   `json:"model_usage"`
		ModelTimeline   []map[string]any `json:"model_usage_timeline"`
		ToolTimeline    []map[string]any `json:"tool_timeline"`
		ServiceTimeline []map[string]any `json:"service_timeline"`
		Diagnostics     []map[string]any `json:"diagnostics"`
		Artifacts       map[string]any   `json:"artifacts"`
	}
	data, err := os.ReadFile(artifacts.Observability)
	if err != nil {
		t.Fatalf("read observability artifact: %v", err)
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode observability artifact: %v\n%s", err, string(data))
	}
	if len(payload.PromptSHA256) != 64 {
		t.Fatalf("prompt hash length = %d, want 64", len(payload.PromptSHA256))
	}
	if payload.ArtifactID == "" || payload.SessionID != "session-1" || payload.RunID != "run-1" {
		t.Fatalf("observability identity missing: %+v", payload)
	}
	if payload.Model["provider"] != "openrouter" || payload.Embedding["provider"] != "openrouter" {
		t.Fatalf("unexpected model/embedding snapshot: %+v %+v", payload.Model, payload.Embedding)
	}
	if payload.CatalogSnapshot["catalog_ref"] == "" || payload.ProfileSnapshot["model"] != "test/model" {
		t.Fatalf("catalog/profile snapshots missing: %+v %+v", payload.CatalogSnapshot, payload.ProfileSnapshot)
	}
	if payload.ModelUsage["provider"] != "openrouter" || payload.ModelUsage["reported_by_model"] != false {
		t.Fatalf("unexpected model usage snapshot: %+v", payload.ModelUsage)
	}
	if len(payload.ModelTimeline) != 1 || payload.ModelTimeline[0]["provider"] != "openrouter" {
		t.Fatalf("unexpected model usage timeline: %+v", payload.ModelTimeline)
	}
	if len(payload.ToolTimeline) != 2 {
		t.Fatalf("tool timeline length = %d, want 2: %+v", len(payload.ToolTimeline), payload.ToolTimeline)
	}
	if payload.ToolTimeline[0]["service_call_id"] != "call-1" || payload.ToolTimeline[0]["run_id"] != "run-1" {
		t.Fatalf("tool timeline missing correlation fields: %+v", payload.ToolTimeline)
	}
	if len(payload.ServiceTimeline) != 2 || payload.ServiceTimeline[0]["service"] != "gateway" {
		t.Fatalf("unexpected service timeline: %+v", payload.ServiceTimeline)
	}
	if payload.ServiceTimeline[0]["subject"] != "svc.gateway.v1.embed" {
		t.Fatalf("service timeline missing NATS subject: %+v", payload.ServiceTimeline[0])
	}
	if len(payload.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics for successful trace: %+v", payload.Diagnostics)
	}
	if payload.Artifacts["reply"] == "" || payload.Artifacts["observability"] == "" {
		t.Fatalf("artifact paths missing from observability payload: %+v", payload.Artifacts)
	}
}

func TestAgentRunArtifactsIncludeToolFailureDiagnostics(t *testing.T) {
	dir := t.TempDir()
	env := &utils.E2EEnv{
		Space:  "e2e-artifact-test",
		SupURL: "http://127.0.0.1:7200",
		Agent: api.RuntimeInfo{
			ID: "runtime-1",
		},
		Provider: "openrouter",
		Model:    "test/model",
		Embedding: utils.GatewayEmbeddingOptions{
			Provider:   "openrouter",
			Model:      "fixture/embed",
			Dimensions: 32,
		},
	}
	trace := utils.MessageTrace{
		Space:     env.Space,
		SessionID: "session-1",
		RunID:     "run-1",
		ToolResultEvents: []utils.ToolEvent{{
			CallID:        "call-1",
			ServiceCallID: "call-1",
			Name:          "indexer_QueryContext",
			Error:         true,
			SessionID:     "session-1",
			RunID:         "run-1",
		}},
	}

	artifacts := writeAgentRunArtifacts(t, dir, "agent-run", env, trace, "what did we index?")
	data, err := os.ReadFile(artifacts.Observability)
	if err != nil {
		t.Fatalf("read observability artifact: %v", err)
	}
	var payload struct {
		Diagnostics []map[string]any `json:"diagnostics"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode observability artifact: %v\n%s", err, string(data))
	}
	if len(payload.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %+v", payload.Diagnostics)
	}
	if payload.Diagnostics[0]["service"] != "indexer" || payload.Diagnostics[0]["service_call_id"] != "call-1" {
		t.Fatalf("diagnostic missing service correlation: %+v", payload.Diagnostics[0])
	}
}
