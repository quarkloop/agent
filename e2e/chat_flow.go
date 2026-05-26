//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/quarkloop/e2e/utils"
)

type chatPromptRun struct {
	Title          string
	Label          string
	ArtifactPrefix string
	Prompt         string
	TraceOptions   utils.MessageTraceOptions
}

func runChatPrompt(t *testing.T, ctx context.Context, env *utils.E2EEnv, artifactDir string, run chatPromptRun) utils.MessageTrace {
	t.Helper()
	session := utils.CreateChatSession(t, env, run.Title)

	trace := utils.PostMessageTraceWithOptions(t, ctx, env, session.ID, run.Prompt, run.TraceOptions)
	utils.Logf(t, "%s reply: %s", run.Label, trace.Text)
	if run.ArtifactPrefix != "" {
		writeAgentRunArtifacts(t, artifactDir, run.ArtifactPrefix, env, trace, run.Prompt)
	}
	return trace
}
