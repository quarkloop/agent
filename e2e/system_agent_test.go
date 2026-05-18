//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/quarkloop/e2e/utils"
)

func TestAgentUsesSystemServiceForReadOnlyInspection(t *testing.T) {
	workingDir := t.TempDir()
	env := utils.StartE2E(t, true, standardSystemServicesStartOptions(t, workingDir))

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	prompt := systemReadOnlyInspectionPrompt()
	trace := runChatPrompt(t, ctx, env, workingDir, chatPromptRun{
		Title:          "system-read-only-inspection",
		Label:          "system inspection",
		ArtifactPrefix: "system-read-only-inspection",
		Prompt:         prompt,
		TraceOptions:   systemServiceTraceOptions("system read-only inspection"),
	})

	assertToolStarted(t, trace, "system_Snapshot")
	assertToolStarted(t, trace, "system_GetDiskUsage")
	assertToolStarted(t, trace, "system_GetMetrics")
	assertToolStartedAny(t, trace, "system_ListPorts", "system_ListNetworkConnections")
	assertToolStarted(t, trace, "system_ListProcesses")
	assertNoToolErrors(t, trace,
		"system_Snapshot",
		"system_GetDiskUsage",
		"system_GetMetrics",
		"system_ListPorts",
		"system_ListNetworkConnections",
		"system_ListProcesses",
	)
	if contains(trace.ToolStarts, "system_KillProcess") || contains(trace.ToolStarts, "system_RestartService") {
		t.Fatalf("read-only system inspection attempted a mutation function; starts=%v", trace.ToolStarts)
	}
	assertToolResultContains(t, trace, "system_Snapshot", "os", "kernel")
	assertAnswerContainsAny(t, trace.Text, "kernel", "uptime", "load", "memory", "disk")
}
