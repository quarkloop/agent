//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/quarkloop/e2e/utils"
)

// TestSupervisorSessionEventReachesAgent verifies the supervisor -> runtime
// NATS path: creating a session through supervisor-owned subjects should cause
// the runtime process to mirror it into its in-memory registry.
func TestSupervisorSessionEventReachesAgent(t *testing.T) {
	env := utils.StartE2E(t, false, utils.StartOptions{DisableKnowledgeServices: true})

	before := utils.AgentSessionsCount(t, env)

	sess := utils.CreateChatSession(t, env, "event-test")
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
