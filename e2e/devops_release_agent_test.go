//go:build e2e

package e2e

import (
	"context"
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
