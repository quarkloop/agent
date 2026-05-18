//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/quarkloop/e2e/utils"
	"github.com/quarkloop/supervisor/pkg/api"
)

func TestAgentUsesSystemServiceForReadOnlyInspection(t *testing.T) {
	workingDir := t.TempDir()
	env := utils.StartE2E(t, true, standardSystemServicesStartOptions(t, workingDir))

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	session, err := env.Sup.CreateSession(ctx, env.Space, api.CreateSessionRequest{
		Type:  api.SessionTypeChat,
		Title: "system-read-only-inspection",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	utils.WaitForAgentSession(t, env, session.ID, 10*time.Second)

	prompt := systemReadOnlyInspectionPrompt()
	trace := utils.PostMessageTraceWithOptions(t, ctx, env, session.ID, prompt, utils.MessageTraceOptions{
		Label:          "system read-only inspection",
		OverallTimeout: 3 * time.Minute,
		IdleTimeout:    90 * time.Second,
	})
	utils.Logf(t, "system inspection reply: %s", trace.Text)
	writeAgentRunArtifacts(t, workingDir, "system-read-only-inspection", env, trace, prompt)

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
