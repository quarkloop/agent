package fsstore_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	spacemodel "github.com/quarkloop/pkg/space"
	"github.com/quarkloop/supervisor/pkg/space/fsstore"
)

func TestFSStoreCreatesLayoutAndStoresLatestQuarkfile(t *testing.T) {
	root := t.TempDir()
	store, err := fsstore.NewFSStore(root)
	if err != nil {
		t.Fatal(err)
	}
	workDir := t.TempDir()
	qf := spacemodel.DefaultQuarkfile("test-space")

	sp, err := store.Create("test-space", qf, workDir)
	if err != nil {
		t.Fatal(err)
	}
	if sp.Version != "0.1.0" {
		t.Fatalf("create version = %q, want 0.1.0", sp.Version)
	}
	if _, err := os.Stat(filepath.Join(workDir, spacemodel.QuarkfileName)); err != nil {
		t.Fatalf("working Quarkfile missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "test-space", spacemodel.QuarkfileName)); err != nil {
		t.Fatalf("space Quarkfile missing: %v", err)
	}

	sp, err = store.UpdateQuarkfile("test-space", qf)
	if err != nil {
		t.Fatal(err)
	}
	latest, err := store.Quarkfile("test-space")
	if err != nil {
		t.Fatal(err)
	}
	if string(latest) != string(qf) {
		t.Fatal("latest Quarkfile did not match update")
	}
}

func TestFSStoreAgentEnvironmentComesFromQuarkfileModel(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "secret")
	t.Setenv("UNDECLARED_SECRET", "must-not-leak")

	store, err := fsstore.NewFSStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("test-space", spacemodel.DefaultQuarkfile("test-space"), t.TempDir()); err != nil {
		t.Fatal(err)
	}

	env, err := store.AgentEnvironment("test-space")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"QUARK_MODEL_PROVIDER=anthropic",
		"QUARK_MODEL_NAME=claude-sonnet-4",
		"ANTHROPIC_API_KEY=secret",
	} {
		if !slices.Contains(env, want) {
			t.Fatalf("agent env missing %q: %v", want, env)
		}
	}
	for _, got := range env {
		if got == "UNDECLARED_SECRET=must-not-leak" {
			t.Fatalf("agent env leaked undeclared variable: %v", env)
		}
	}
}

func TestFSStoreAgentEnvironmentAllowsAgentProfileModelWithoutTopLevelModel(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "secret")
	store, err := fsstore.NewFSStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	qf := []byte(`quark: "1.0"
meta:
  name: test-space
  version: "0.1.0"
plugins:
  - ref: quark/agent-knowledge
agents:
  - profile: quark-knowledge
    model:
      provider: openrouter
      name: openai/gpt-5-mini
      env:
        - OPENROUTER_API_KEY
`)
	if _, err := store.Create("test-space", qf, t.TempDir()); err != nil {
		t.Fatal(err)
	}

	env, err := store.AgentEnvironment("test-space")
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(env, "QUARK_MODEL_PROVIDER=openrouter") {
		t.Fatalf("agent-specific model should be carried in the resolved profile catalog, got env: %v", env)
	}
	if !slices.Contains(env, "OPENROUTER_API_KEY=secret") {
		t.Fatalf("agent model credential missing: %v", env)
	}
}

func TestFSStoreSessionsUseDirectory(t *testing.T) {
	root := t.TempDir()
	store, err := fsstore.NewFSStore(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("test-space", spacemodel.DefaultQuarkfile("test-space"), t.TempDir()); err != nil {
		t.Fatal(err)
	}

	sessions, err := store.Sessions("test-space")
	if err != nil {
		t.Fatal(err)
	}
	session, err := sessions.Create("chat", "hello")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "test-space", "sessions", session.ID+".jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("session file missing: %v", err)
	}
}
