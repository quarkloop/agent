//go:build e2e

package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestCompaction verifies that compaction fires when history exceeds 80% of the
// model's context window and that the agent keeps responding correctly after.
//
// Strategy:
//  1. Serve a local models.json with context_window=200 tokens → char budget = 200*4*0.8 = 640 chars.
//  2. Pass MODEL_LIST_URL pointing to that server so the agent loads it.
//  3. Send 5 messages each padded to ~200 chars — total ~1000 chars >> 640 char budget.
//  4. Verify the agent still responds after compaction trimmed the history.
func TestCompaction(t *testing.T) {
	if os.Getenv("OPENROUTER_API_KEY") == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}

	_ = exec.Command("pkill", "-f", "builtin").Run()
	time.Sleep(300 * time.Millisecond)

	// Serve a models.json with a tiny context_window to force compaction.
	model := os.Getenv("OPENROUTER_MODEL")
	if model == "" {
		model = "stepfun/step-3.5-flash:free"
	}
	modelList, _ := json.Marshal([]map[string]any{
		{
			"id":             model,
			"provider":       "openrouter",
			"name":           model,
			"default":        true,
			"context_window": 200, // tiny: budget = 200*4*0.8 = 640 chars
		},
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(modelList)
	}))
	defer srv.Close()

	agentBin := buildAgent(t)
	port := reservePort(t)

	// Inject MODEL_LIST_URL so the agent fetches our tiny context_window config.
	origEnv := os.Getenv("MODEL_LIST_URL")
	os.Setenv("MODEL_LIST_URL", srv.URL)
	defer os.Setenv("MODEL_LIST_URL", origEnv)

	baseURL := startAgent(t, agentBin, port)
	t.Logf("agent at %s, model list at %s", baseURL, srv.URL)

	sessionID := createSession(t, baseURL, "compaction-test", "Compaction Test")

	// Each message is padded to ~200 chars so 5 turns ≈ 1000 chars >> 640 char budget.
	padding := strings.Repeat("x", 180)
	send := func(prompt string) string {
		for attempt := 1; attempt <= 5; attempt++ {
			reply := sendMessage(t, baseURL, sessionID, prompt, 30*time.Second)
			isErr := reply == "" || reply == "RATE_LIMITED" || isProviderRateLimited(reply) || strings.Contains(reply, "Agent Error")
			if !isErr {
				return reply
			}
			t.Logf("attempt %d bad reply, retry in 4s...", attempt)
			time.Sleep(4 * time.Second)
		}
		t.Fatalf("no reply after retries for: %q", prompt)
		return ""
	}

	turns := []string{
		"What is the capital of France?",
		"What is the largest planet in the solar system?",
		"Who wrote the play Hamlet?",
		"What is the chemical symbol for water?",
		"What language is primarily spoken in Brazil?",
	}

	for i, question := range turns {
		prompt := fmt.Sprintf("%s (ignore this padding: %s)", question, padding)
		reply := send(prompt)
		t.Logf("turn %d: %q", i+1, reply)
	}

	// After compaction the agent must still answer a fresh question correctly.
	final := send("What is the boiling point of water in Celsius? Just the number.")
	t.Logf("final: %q", final)
	if !strings.Contains(final, "100") {
		t.Errorf("expected '100' in final reply, got %q", final)
	}

	// Verify compaction actually fired by checking the agent log.
	// The log line is emitted by llmcontext.CompactIndex when threshold is exceeded.
	agentLog := agentLogOf(t, baseURL)
	if !strings.Contains(agentLog, "llmcontext: compaction triggered") {
		t.Errorf("compaction was never triggered — log line missing.\nagent log:\n%s", agentLog)
	} else {
		// Print the compaction lines so CI shows exactly what happened.
		for _, line := range strings.Split(agentLog, "\n") {
			if strings.Contains(line, "llmcontext:") {
				t.Logf("compaction log: %s", line)
			}
		}
	}
}
