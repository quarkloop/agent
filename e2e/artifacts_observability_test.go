//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/quarkloop/e2e/utils"
)

func TestAgentRunArtifactsAreRedactedAndStructured(t *testing.T) {
	secret := "sk-or-v1-super-secret-token"
	t.Setenv("OPENROUTER_API_KEY", secret)
	dir := t.TempDir()
	env := &utils.E2EEnv{
		Space:    "e2e-artifact-test",
		SupURL:   "http://127.0.0.1:7200",
		AgentURL: "http://127.0.0.1:7300",
		Provider: "openrouter",
		Model:    "test/model",
		Embedding: utils.EmbeddingOptions{
			Plugin:     "embedding",
			Mode:       "local",
			Provider:   "local",
			Model:      "local-hash-v1",
			Dimensions: 32,
		},
	}
	trace := utils.MessageTrace{
		Text:       "reply with " + secret,
		ToolStarts: []string{"embedding_Embed"},
		ToolStartEvents: []utils.ToolEvent{{
			CallID:    "call-1",
			Name:      "embedding_Embed",
			Arguments: `{"authorization":"Bearer ` + secret + `"}`,
		}},
		ToolResultEvents: []utils.ToolEvent{{
			CallID: "call-1",
			Name:   "embedding_Embed",
			Result: `{"api_key":"` + secret + `"}`,
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
		PromptSHA256    string           `json:"prompt_sha256"`
		Model           map[string]any   `json:"model"`
		Embedding       map[string]any   `json:"embedding"`
		ToolTimeline    []map[string]any `json:"tool_timeline"`
		ServiceTimeline []map[string]any `json:"service_timeline"`
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
	if payload.Model["provider"] != "openrouter" || payload.Embedding["provider"] != "local" {
		t.Fatalf("unexpected model/embedding snapshot: %+v %+v", payload.Model, payload.Embedding)
	}
	if len(payload.ToolTimeline) != 2 {
		t.Fatalf("tool timeline length = %d, want 2: %+v", len(payload.ToolTimeline), payload.ToolTimeline)
	}
	if len(payload.ServiceTimeline) != 2 || payload.ServiceTimeline[0]["service"] != "embedding" {
		t.Fatalf("unexpected service timeline: %+v", payload.ServiceTimeline)
	}
	if payload.Artifacts["reply"] == "" || payload.Artifacts["observability"] == "" {
		t.Fatalf("artifact paths missing from observability payload: %+v", payload.Artifacts)
	}
}
