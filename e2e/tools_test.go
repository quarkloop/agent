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
	env := utils.StartE2E(t, true, utils.StartOptions{
		DisableKnowledgeServices: true,
		Agents:                   []string{"quark-main"},
		Services:                 append(localServicePlugins("io"), gatewayServicePlugin()),
		AgentServicePermissions: map[string][]string{
			"quark-main": {"io_Execute"},
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	beforeSessions := utils.AgentSessionsCount(t, env)
	sess := utils.CreateChatSession(t, env, "io-execute-test")
	trace := utils.PostMessageTrace(t, ctx, env, sess.ID,
		"Please run a shell command that prints the marker text quark-ok, then reply with only what the command printed.")
	if afterSessions := utils.AgentSessionsCount(t, env); afterSessions <= beforeSessions {
		t.Fatalf("runtime did not admit the supervisor-created input session: sessions before=%d after=%d", beforeSessions, afterSessions)
	}
	assertToolStarted(t, trace, "io_Execute")
	utils.Logf(t, "reply: %q", trace.Text)
	if trace.Text == "" {
		t.Fatal("expected non-empty reply")
	}
	if !strings.Contains(trace.Text, "quark-ok") {
		t.Fatalf("expected reply to contain %q, got %q", "quark-ok", trace.Text)
	}
}
