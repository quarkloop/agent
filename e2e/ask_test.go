//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/quarkloop/supervisor/pkg/api"

	"github.com/quarkloop/e2e/utils"
)

// TestAskMode drives a full supervisor → agent chat flow: supervisor creates
// a chat session (agent mirrors it via SSE), then the test POSTs a user
// message to the agent's SSE endpoint and asserts a non-empty streamed reply.
func TestAskMode(t *testing.T) {
	env := utils.StartE2E(t, true, utils.StartOptions{
		DisableKnowledgeServices: true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	sess, err := env.Sup.CreateSession(ctx, env.Space, api.CreateSessionRequest{
		Type:  api.SessionTypeChat,
		Title: "ask-test",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	utils.WaitForAgentSession(t, env, sess.ID, 10*time.Second)

	reply := utils.PostMessage(t, ctx, env, sess.ID, "What is 2+2? Reply with just the number.")
	utils.Logf(t, "reply: %q", reply)
	if reply == "" {
		t.Fatal("expected non-empty reply")
	}
}
