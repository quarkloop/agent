//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/quarkloop/e2e/utils"
)

// TestAskMode drives a full supervisor -> runtime chat flow over NATS.
func TestAskMode(t *testing.T) {
	gatewayAddr := reserveLoopbackAddress(t)
	env := utils.StartE2E(t, true, utils.StartOptions{
		DisableKnowledgeServices: true,
		Services:                 []utils.ServicePlugin{gatewayServicePlugin()},
		SupervisorEnv: map[string]string{
			"QUARK_GATEWAY_SERVICE_ADDR": gatewayAddr,
		},
		BeforeRuntime: func(t *testing.T, setup utils.RuntimeSetup, bins utils.BuiltBinaries) {
			t.Helper()
			startGatewayServiceAt(t, bins.Model, gatewayAddr, setup.NATS.ClientURL)
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	sess := utils.CreateChatSession(t, env, "ask-test")

	utils.WaitForAgentSession(t, env, sess.ID, 10*time.Second)

	reply := utils.PostMessage(t, ctx, env, sess.ID, "What is 2+2? Reply with just the number.")
	utils.Logf(t, "reply: %q", reply)
	if reply == "" {
		t.Fatal("expected non-empty reply")
	}
}
