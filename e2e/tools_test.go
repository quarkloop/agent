//go:build e2e

package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/quarkloop/e2e/utils"
)

// TestIOExecute exercises io_Execute through the runtime service catalog.
func TestIOExecute(t *testing.T) {
	ioAddr := reserveLoopbackAddress(t)
	gatewayAddr := reserveLoopbackAddress(t)
	env := utils.StartE2E(t, true, utils.StartOptions{
		DisableKnowledgeServices: true,
		Agents:                   []string{"quark-main"},
		Services:                 append(localServicePlugins("io"), gatewayServicePlugin()),
		SupervisorEnv: map[string]string{
			"QUARK_IO_ADDR":              ioAddr,
			"QUARK_GATEWAY_SERVICE_ADDR": gatewayAddr,
		},
		AgentServicePermissions: map[string][]string{
			"quark-main": {"io_Execute"},
		},
		BeforeRuntime: func(t *testing.T, setup utils.RuntimeSetup, bins utils.BuiltBinaries) {
			t.Helper()
			startGatewayServiceAt(t, bins.Gateway, gatewayAddr, setup.NATS.ClientURL)
			startIOServiceAt(t, bins.IO, ioAddr, setup.NATS)
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	sess := utils.CreateChatSession(t, env, "io-execute-test")
	utils.WaitForAgentSession(t, env, sess.ID, 10*time.Second)

	trace := utils.PostMessageTrace(t, ctx, env, sess.ID,
		"Please run a shell command that prints the marker text quark-ok, then reply with only what the command printed.")
	assertToolStarted(t, trace, "io_Execute")
	utils.Logf(t, "reply: %q", trace.Text)
	if trace.Text == "" {
		t.Fatal("expected non-empty reply")
	}
	if !strings.Contains(trace.Text, "quark-ok") {
		t.Fatalf("expected reply to contain %q, got %q", "quark-ok", trace.Text)
	}
}
