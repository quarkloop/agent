//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/quarkloop/e2e/utils"
)

// TestRuntimeAcceptsSupervisorSessionInput verifies the NATS boundary: the
// supervisor provisions session identity and runtime admits it when the user
// first sends input. Runtime no longer mirrors HTTP-created state.
func TestRuntimeAcceptsSupervisorSessionInput(t *testing.T) {
	env := utils.StartE2E(t, true, utils.StartOptions{
		DisableKnowledgeServices: true,
		Services:                 []utils.ServicePlugin{gatewayServicePlugin()},
	})

	before := utils.AgentSessionsCount(t, env)
	sess := utils.CreateChatSession(t, env, "event-test")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	reply := utils.PostMessage(t, ctx, env, sess.ID, "Reply briefly to confirm this session is available.")
	if reply == "" {
		t.Fatal("expected a response after sending session input")
	}
	after := utils.AgentSessionsCount(t, env)
	if after <= before {
		t.Fatalf("runtime did not admit input session: sessions before=%d after=%d", before, after)
	}
}
