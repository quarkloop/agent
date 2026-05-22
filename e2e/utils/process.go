//go:build e2e

package utils

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// testLogWriter writes lines to t.Logf, prefixing each with a label. It is
// useful for streaming subprocess output to the test log so hangs can be
// diagnosed without waiting for Cleanup.
type testLogWriter struct {
	t      *testing.T
	prefix string
	buf    bytes.Buffer
	mu     sync.Mutex
}

func (w *testLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	for {
		line, err := w.buf.ReadBytes('\n')
		if err == io.EOF {
			w.buf.Write(line)
			break
		}
		w.t.Logf("%s", formatProcessLogLine(w.t.Name(), w.prefix, strings.TrimRight(string(line), "\r\n")))
	}
	return len(p), nil
}

// Logf writes an e2e harness log line with the standard prefix.
func Logf(t testing.TB, format string, args ...any) {
	t.Helper()
	allArgs := append([]any{t.Name()}, args...)
	t.Logf("[e2e][test=%s] "+format, allArgs...)
}

func formatProcessLogLine(testName, defaultProcess, line string) string {
	process := processFromLogLine(defaultProcess, line)
	if process != defaultProcess {
		return fmt.Sprintf("[e2e][test=%s][process=%s][parent_process=%s] %s", testName, process, defaultProcess, line)
	}
	return fmt.Sprintf("[e2e][test=%s][process=%s] %s", testName, defaultProcess, line)
}

func processFromLogLine(defaultProcess, line string) string {
	for _, process := range []string{"runtime", "supervisor"} {
		if strings.Contains(line, "process="+process) || strings.Contains(line, `"process":"`+process+`"`) {
			return process
		}
	}
	return defaultProcess
}

// StartedProcess is a handle for a subprocess launched by StartProcess.
type StartedProcess struct {
	Name string
	Cmd  *exec.Cmd
	done chan error

	mu   sync.Mutex
	logs bytes.Buffer
}

// Logs returns a snapshot of the subprocess's stdout+stderr so far.
func (p *StartedProcess) Logs() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.logs.String()
}

// WaitForLog polls the subprocess log buffer for needle until timeout.
func (p *StartedProcess) WaitForLog(t *testing.T, needle string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(p.Logs(), needle) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for log %q from %s", needle, p.Name)
}

// bufferWriter is a thread-safe writer that appends to the StartedProcess
// log buffer. It is separate from the per-line test logger.
type bufferWriter struct{ p *StartedProcess }

func (b *bufferWriter) Write(p []byte) (int, error) {
	b.p.mu.Lock()
	defer b.p.mu.Unlock()
	return b.p.logs.Write(p)
}

// StartProcess launches binary with args and captures stdout/stderr into an
// in-memory buffer that is logged when the test fails. The child is placed in
// its own process group so Cleanup can reap the entire tree.
func StartProcess(t *testing.T, name, binary string, args, env []string) *StartedProcess {
	t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = env

	proc := &StartedProcess{Name: name, Cmd: cmd, done: make(chan error, 1)}
	// Tee process stdio into both the buffer (for post-mortem) and the test
	// log (so hangs are visible without waiting for Cleanup).
	writer := io.MultiWriter(&bufferWriter{p: proc}, &testLogWriter{t: t, prefix: name})
	cmd.Stdout = writer
	cmd.Stderr = writer
	// Put the child and any grandchildren it spawns in a dedicated process
	// group so cleanup can signal the whole tree. Without this, the
	// supervisor's agent child survives a SIGKILL to the supervisor and
	// keeps the inherited stdout pipe open, blocking cmd.Wait forever.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// WaitDelay caps how long Wait will block for pipe drainage once the
	// main process has exited. After this, the pipe is force-closed even
	// if some grandchild still holds it open.
	cmd.WaitDelay = 2 * time.Second

	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", name, err)
	}
	go func() { proc.done <- cmd.Wait() }()

	t.Cleanup(func() {
		select {
		case <-proc.done:
		default:
			if cmd.Process != nil {
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
			<-proc.done
		}
		if t.Failed() {
			Logf(t, "%s logs:\n%s", proc.Name, proc.Logs())
		}
	})
	return proc
}

// ReservePort allocates a free TCP port on loopback and immediately releases
// it. Callers race with any other process for the port, but this is good
// enough for e2e tests.
func ReservePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// WaitForURL polls url until it returns a non-5xx status or timeout elapses.
func WaitForURL(t *testing.T, url string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode < 500 {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", url)
}

// WaitForTCP polls addr until a TCP connection succeeds or timeout elapses.
func WaitForTCP(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for tcp %s", addr)
}

// SupervisorProcessEnv returns the constrained environment needed by the e2e
// supervisor: non-secret process basics plus provider credentials that the
// space service may resolve only when Quarkfile explicitly declares them.
func SupervisorProcessEnv(overrides map[string]string) []string {
	return constrainedProcessEnv(overrides, providerCredentialEnvKeys()...)
}

// ServiceProcessEnv returns a constrained service process environment. Service
// start helpers must pass provider keys explicitly when that service requires
// provider access.
func ServiceProcessEnv(overrides map[string]string, extraAllowed ...string) []string {
	return constrainedProcessEnv(overrides, extraAllowed...)
}

// RuntimeProcessEnv returns the constrained environment needed by a runtime
// launched directly by the e2e harness. Runtime launch is deployment-owned in
// the NATS-native architecture; the supervisor no longer spawns the process.
func RuntimeProcessEnv(overrides map[string]string) []string {
	return constrainedProcessEnv(overrides, providerCredentialEnvKeys()...)
}

func constrainedProcessEnv(overrides map[string]string, extraAllowed ...string) []string {
	allowed := map[string]struct{}{}
	for _, key := range baseProcessEnvKeys() {
		allowed[key] = struct{}{}
	}
	for _, key := range extraAllowed {
		if key != "" {
			allowed[key] = struct{}{}
		}
	}
	values := make(map[string]string, len(allowed)+len(overrides))
	order := make([]string, 0, len(allowed)+len(overrides))
	add := func(key, value string) {
		if key == "" {
			return
		}
		if _, exists := values[key]; !exists {
			order = append(order, key)
		}
		values[key] = value
	}
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, keep := allowed[key]; keep {
			add(key, value)
		}
	}
	overrideKeys := make([]string, 0, len(overrides))
	for key := range overrides {
		overrideKeys = append(overrideKeys, key)
	}
	sort.Strings(overrideKeys)
	for _, key := range overrideKeys {
		add(key, overrides[key])
	}
	out := make([]string, 0, len(order))
	for _, key := range order {
		out = append(out, key+"="+values[key])
	}
	return out
}

func baseProcessEnvKeys() []string {
	return []string{
		"PATH",
		"HOME",
		"USER",
		"LOGNAME",
		"TMPDIR",
		"TEMP",
		"TMP",
		"LANG",
		"LC_ALL",
		"SSL_CERT_FILE",
		"SSL_CERT_DIR",
		"TZ",
		"TERM",
		"XDG_CACHE_HOME",
		"XDG_CONFIG_HOME",
		"XDG_DATA_HOME",
		"GOCACHE",
		"GOMODCACHE",
		"GOPATH",
		"GOROOT",
		"GOENV",
		"GOFLAGS",
		"CGO_ENABLED",
		"DOCKER_CONFIG",
		"DOCKER_CONTEXT",
		"DOCKER_HOST",
	}
}

func providerCredentialEnvKeys() []string {
	return []string{
		"OPENROUTER_API_KEY",
		"OPENROUTER_BASE_URL",
		"OPENAI_API_KEY",
		"OPENAI_BASE_URL",
		"ANTHROPIC_API_KEY",
		"ANTHROPIC_BASE_URL",
		"ZHIPU_API_KEY",
		"ZHIPU_BASE_URL",
	}
}
