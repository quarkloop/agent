//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/quarkloop/e2e/utils"
)

func TestAgentUsesDevOpsReleaseServiceFunction(t *testing.T) {
	workingDir := t.TempDir()
	writeDevOpsReleaseFixture(t, workingDir)
	initGitRepository(t, workingDir)

	env := utils.StartE2E(t, true, standardDevOpsServicesStartOptions(t, workingDir))

	ctx, cancel := context.WithTimeout(context.Background(), devOpsServiceFlowTimeout+time.Minute)
	defer cancel()

	prompt := buildReleaseDryRunPrompt(workingDir)
	trace := runChatPrompt(t, ctx, env, workingDir, chatPromptRun{
		Title:          "devops-release-test",
		Label:          "devops release",
		ArtifactPrefix: "devops-release-agent",
		Prompt:         prompt,
		TraceOptions:   devOpsServiceTraceOptions("devops release dry run through service function"),
	})

	assertToolStarted(t, trace, "repo_Status")
	assertToolStarted(t, trace, "build_DetectProject")
	assertToolStarted(t, trace, "policy_EvaluateChange")
	assertToolStarted(t, trace, "build_DryRunRelease")
	assertToolLatestResultsSucceeded(t, trace, "repo_Status", "build_DetectProject", "policy_EvaluateChange", "build_DryRunRelease")
	assertToolResultContains(t, trace, "build_DryRunRelease", "v9.9.9", "quark-devops-fixture")
	assertAnswerContains(t, trace.Text, "v9.9.9", "quark-devops-fixture")
}

func TestAgentUsesDevOpsServiceForTestFailureExplanation(t *testing.T) {
	workingDir := t.TempDir()
	writeFailingGoTestFixture(t, workingDir)
	initGitRepository(t, workingDir)

	env := utils.StartE2E(t, true, standardDevOpsOnlyServicesStartOptions(t, workingDir))

	ctx, cancel := context.WithTimeout(context.Background(), devOpsServiceFlowTimeout+time.Minute)
	defer cancel()

	prompt := devOpsTestFailurePrompt(workingDir)
	trace := runChatPrompt(t, ctx, env, workingDir, chatPromptRun{
		Title:          "devops-test-failure",
		Label:          "devops failure",
		ArtifactPrefix: "devops-test-failure",
		Prompt:         prompt,
		TraceOptions:   devOpsServiceTraceOptions("devops test failure explanation"),
	})

	assertToolStarted(t, trace, "repo_Status")
	assertToolStarted(t, trace, "test_RunTests")
	assertToolStarted(t, trace, "test_ExplainFailure")
	assertToolNotStarted(t, trace, "build_DryRunRelease")
	assertToolLatestResultsSucceeded(t, trace, "repo_Status", "test_RunTests", "test_ExplainFailure")
	assertToolResultContains(t, trace, "test_RunTests", "TestBroken", "expected stable behavior")
	assertAnswerContainsAny(t, trace.Text, "TestBroken", "expected stable behavior", "failure")
}

func writeDevOpsReleaseFixture(t *testing.T, dir string) {
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

func writeFailingGoTestFixture(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/quarkdevopsfailure\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc Stable() bool { return false }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main_test.go"), []byte(`package main

import "testing"

func TestBroken(t *testing.T) {
	if !Stable() {
		t.Fatalf("expected stable behavior")
	}
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
}

func initGitRepository(t *testing.T, dir string) {
	t.Helper()
	runGitFixtureCommand(t, dir, "init")
	runGitFixtureCommand(t, dir, "config", "user.email", "e2e@example.invalid")
	runGitFixtureCommand(t, dir, "config", "user.name", "Quark E2E")
	runGitFixtureCommand(t, dir, "add", ".")
	runGitFixtureCommand(t, dir, "commit", "-m", "initial fixture")
}

func runGitFixtureCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
}
