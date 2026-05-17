//go:build e2e

package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/quarkloop/supervisor/pkg/api"

	"github.com/quarkloop/e2e/utils"
)

// TestBashTool exercises the bash tool plugin end-to-end: supervisor creates
// a session, the agent receives it via SSE, a normal user message asks for a
// terminal check, and the final streamed reply must contain the expected
// stdout. Tools load in lib mode (plugin.so shipped alongside the binary).
func TestBashTool(t *testing.T) {
	runBashTool(t, false)
}

// TestBashToolBinaryMode runs the same flow as TestBashTool but with the
// tool plugin.so stripped from the installed space, forcing the agent's
// pluginmanager to fall back to api-mode (HTTP daemon) loading.
func TestBashToolBinaryMode(t *testing.T) {
	runBashTool(t, true)
}

func runBashTool(t *testing.T, forceBinary bool) {
	t.Helper()
	env := utils.StartE2E(t, true, utils.StartOptions{
		ForceBinaryTools:         forceBinary,
		DisableServiceDiscovery:  true,
		DisableKnowledgeServices: true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	sess, err := env.Sup.CreateSession(ctx, env.Space, api.CreateSessionRequest{
		Type:  api.SessionTypeChat,
		Title: "tools-test",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	utils.WaitForAgentSession(t, env, sess.ID, 10*time.Second)

	reply := utils.PostMessage(t, ctx, env, sess.ID,
		"Please use the terminal to print the marker text quark-ok, then reply with only what the terminal printed.")
	utils.Logf(t, "reply: %q", reply)
	if reply == "" {
		t.Fatal("expected non-empty reply")
	}
	if !strings.Contains(reply, "quark-ok") {
		t.Fatalf("expected reply to contain %q, got %q", "quark-ok", reply)
	}
}
