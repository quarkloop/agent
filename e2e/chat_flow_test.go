//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

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
	utils.WaitForAgentSession(t, env, session.ID, 10*time.Second)

	trace := utils.PostMessageTraceWithOptions(t, ctx, env, session.ID, run.Prompt, run.TraceOptions)
	utils.Logf(t, "%s reply: %s", run.Label, trace.Text)
	if run.ArtifactPrefix != "" {
		writeAgentRunArtifacts(t, artifactDir, run.ArtifactPrefix, env, trace, run.Prompt)
	}
	return trace
}
