//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/quarkloop/supervisor/pkg/api"

	"github.com/quarkloop/e2e/utils"
)

// TestSupervisorSessionEventReachesAgent verifies the supervisor → agent SSE
// pipeline: creating a session through the supervisor should cause the agent
// child process to mirror it into its in-memory registry. This proves the
// launcher forwards QUARK_SUPERVISOR_URL so the agent can subscribe.
func TestSupervisorSessionEventReachesAgent(t *testing.T) {
	env := utils.StartE2E(t, false, standardKnowledgeServicesStartOptions(t, utils.EmbeddingOptions{
		Plugin:     "embedding",
		Mode:       "local",
		Provider:   "local",
		Model:      "local-hash-v1",
		Dimensions: 32,
	}, ""))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	before := utils.AgentSessionsCount(t, env)

	sess, err := env.Sup.CreateSession(ctx, env.Space, api.CreateSessionRequest{
		Type:  api.SessionTypeChat,
		Title: "event-test",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	utils.Logf(t, "created session id=%s", sess.ID)

	deadline := time.Now().Add(10 * time.Second)
	attempts := 0
	for time.Now().Before(deadline) {
		n := utils.AgentSessionsCount(t, env)
		attempts++
		if attempts == 1 || attempts%10 == 0 {
			utils.Logf(t, "poll %d: agent sessions=%d (want > %d)", attempts, n, before)
		}
		if n > before {
			utils.Logf(t, "agent mirrored session after %d polls", attempts)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("agent never registered the session (polls=%d, before=%d)", attempts, before)
}
