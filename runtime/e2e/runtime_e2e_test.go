//go:build e2e

package e2e_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// loadDotEnv loads key=value pairs from a file into the process environment.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k != "" && os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

func init() {
	_, thisFile, _, _ := runtime.Caller(0)
	quarkRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	loadDotEnv(filepath.Join(quarkRoot, "quark", ".env"))
	loadDotEnv(filepath.Join(quarkRoot, ".env"))
}

// reservePort finds a free TCP port.
func reservePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// buildAgent compiles the agent binary into a temp dir and returns the path.
func buildAgent(t *testing.T) string {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	// agent/e2e/ → agent/
	agentDir := filepath.Join(filepath.Dir(thisFile), "..")

	out := filepath.Join(t.TempDir(), "agent")
	cmd := exec.Command("go", "build", "-o", out, "./cmd/agent")
	cmd.Dir = agentDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build agent: %v\n%s", err, output)
	}
	return out
}

// agentLogs stores log buffers keyed by base URL so tests can inspect them.
var agentLogs = map[string]*bytes.Buffer{}

// agentLogOf returns the accumulated log output for an agent started at baseURL.
func agentLogOf(t *testing.T, baseURL string) string {
	t.Helper()
	if buf, ok := agentLogs[baseURL]; ok {
		return buf.String()
	}
	return ""
}

// startAgent launches the agent binary and returns its base URL.
// The process is killed on t.Cleanup.
func startAgent(t *testing.T, agentBin string, port int) string {
	t.Helper()

	_, thisFile, _, _ := runtime.Caller(0)
	// Run from agent source root so the plugin manager can find pkg/plugins/builtin
	agentSrcRoot := filepath.Join(filepath.Dir(thisFile), "..")

	env := os.Environ()
	// pass through API keys if set
	for _, key := range []string{"OPENROUTER_API_KEY", "ZHIPU_API_KEY", "OPENROUTER_MODEL", "MODEL_LIST_URL"} {
		if v := os.Getenv(key); v != "" {
			env = append(env, fmt.Sprintf("%s=%s", key, v))
		}
	}

	var logBuf bytes.Buffer
	cmd := exec.Command(agentBin, "start", "--port", fmt.Sprint(port))
	cmd.Dir = agentSrcRoot
	cmd.Env = env
	cmd.Stdout = &logBuf
	cmd.Stderr = &logBuf
	// Put agent in its own process group so we can kill it and all its
	// child processes (e.g. builtin tool) in one shot, preventing the
	// stdout pipe from staying open and hanging cmd.Wait().
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start agent: %v", err)
	}

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	agentLogs[baseURL] = &logBuf

	t.Cleanup(func() {
		// Kill the entire process group.
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		_ = cmd.Wait()
		delete(agentLogs, baseURL)
		if t.Failed() {
			t.Logf("agent logs:\n%s", logBuf.String())
		}
	})

	waitForHealth(t, baseURL, 15*time.Second)
	return baseURL
}

// waitForHealth polls GET /health until it responds 200 or times out.
func waitForHealth(t *testing.T, baseURL string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("agent at %s did not become healthy within %s", baseURL, timeout)
}

// createSession creates a chat session and returns its ID.
func createSession(t *testing.T, baseURL, id, title string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{
		"id":    id,
		"type":  "chat",
		"title": title,
	})
	resp, err := http.Post(baseURL+"/v1/sessions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("create session: status %d: %s", resp.StatusCode, data)
	}
	var s struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	return s.ID
}

// sendMessage posts a message to a session and collects the SSE stream.
// Returns the full concatenated token output.
func sendMessage(t *testing.T, baseURL, sessionID, content string, timeout time.Duration) string {
	t.Helper()

	body, _ := json.Marshal(map[string]string{"content": content})
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/v1/sessions/%s/messages", baseURL, sessionID),
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("send message: status %d: %s", resp.StatusCode, data)
	}

	var fullReply strings.Builder
	var agentErr string
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		var msg struct {
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal([]byte(data), &msg); err != nil {
			fullReply.WriteString(data)
			continue
		}
		switch msg.Type {
		case "token":
			var tok string
			if err := json.Unmarshal(msg.Data, &tok); err == nil {
				fullReply.WriteString(tok)
			}
		case "error":
			var errMsg string
			if err := json.Unmarshal(msg.Data, &errMsg); err == nil {
				agentErr = errMsg
			} else {
				agentErr = string(msg.Data)
			}
		}
	}
	if agentErr != "" && fullReply.Len() == 0 {
		t.Logf("agent error: %s", agentErr)
		// Surface rate limit errors so callers can retry.
		if isProviderRateLimited(agentErr) {
			return "RATE_LIMITED"
		}
		return ""
	}
	return fullReply.String()
}

// TestWriteToTmp asks the agent to write content to a /tmp file and verifies it.
func TestWriteToTmp(t *testing.T) {
	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		t.Skip("OPENROUTER_API_KEY not set")
	}

	agentBin := buildAgent(t)
	port := reservePort(t)
	baseURL := startAgent(t, agentBin, port)
	t.Logf("agent running at %s", baseURL)

	sessionID := createSession(t, baseURL, "e2e-write-test", "E2E Write Test")
	t.Logf("session created: %s", sessionID)

	outFile := "/tmp/quark_e2e_test_output.txt"
	_ = os.Remove(outFile)

	prompt := fmt.Sprintf(`You have access to read, bash, and write tools. Do the following:
1. Find out the current date and time by running a bash command.
2. Read /etc/os-release to get the OS name and version.
3. Write a short system report to %s that includes: the current date/time, the OS name and version, and a one-sentence summary of what you found.`, outFile)

	t.Logf("sending prompt...")
	var reply string
	for attempt := 1; attempt <= 5; attempt++ {
		reply = sendMessage(t, baseURL, sessionID, prompt, 2*time.Minute)
		if !isProviderRateLimited(reply) {
			break
		}
		t.Logf("rate limited on attempt %d, retrying in 5s...", attempt)
		time.Sleep(5 * time.Second)
	}
	t.Logf("reply: %q", reply)

	// Verify the file was written
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("expected %s to exist: %v", outFile, err)
	}
	t.Logf("file content:\n%s", string(data))
	if strings.TrimSpace(string(data)) == "" {
		t.Errorf("file is empty")
	}
}
