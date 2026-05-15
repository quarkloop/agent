//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/quarkloop/e2e/utils"
	"github.com/quarkloop/supervisor/pkg/api"
)

func TestAgentUsesBuildReleaseServiceFunction(t *testing.T) {
	buildReleaseAddr := fmt.Sprintf("127.0.0.1:%d", utils.ReservePort(t))
	workingDir := t.TempDir()
	writeBuildReleaseFixture(t, workingDir)

	env := utils.StartE2E(t, true, utils.StartOptions{
		WorkingDir:               workingDir,
		DisableKnowledgeServices: true,
		Services: []utils.ServicePlugin{{
			Name:       "build-release",
			Plugin:     "build-release",
			Mode:       "local",
			AddressEnv: "QUARK_BUILD_RELEASE_ADDR",
		}},
		SupervisorEnv: map[string]string{
			"QUARK_BUILD_RELEASE_ADDR": buildReleaseAddr,
		},
		BeforeRuntime: func(t *testing.T, setup utils.RuntimeSetup, bins utils.BuiltBinaries) {
			t.Helper()
			startBuildReleaseServiceAt(t, bins.BuildRelease, buildReleaseAddr)
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	session, err := env.Sup.CreateSession(ctx, env.Space, api.CreateSessionRequest{
		Type:  api.SessionTypeChat,
		Title: "build-release-devops-test",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	utils.WaitForAgentSession(t, env, session.ID, 10*time.Second)

	prompt := buildReleaseDryRunPrompt(workingDir)
	trace := utils.PostMessageTraceWithOptions(t, ctx, env, session.ID, prompt, utils.MessageTraceOptions{
		Label:          "build-release dry run through service function",
		OverallTimeout: 3 * time.Minute,
		IdleTimeout:    90 * time.Second,
	})
	utils.Logf(t, "build-release reply: %s", trace.Text)
	writeAgentRunArtifacts(t, workingDir, "build-release-agent", env, trace, prompt)

	assertToolStarted(t, trace, "build_release_DryRun")
	assertNoToolErrors(t, trace, "build_release_DryRun")
	assertToolResultContains(t, trace, "build_release_DryRun", "v9.9.9", "quark-devops-fixture")
	assertAnswerContains(t, trace.Text, "v9.9.9", "quark-devops-fixture")
}

func writeBuildReleaseFixture(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/quarkdevopsfixture\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := map[string]any{
		"package_name": "quark-devops-fixture",
		"binary_name":  "quark-devops-fixture",
		"release_dir":  "dist",
		"builds": []map[string]any{{
			"name":        "quark-devops-fixture",
			"source_path": ".",
			"binary_name": "quark-devops-fixture",
			"source_dir":  ".",
		}},
		"targets": []map[string]any{{
			"os":   "linux",
			"arch": "amd64",
		}},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "build_release.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
