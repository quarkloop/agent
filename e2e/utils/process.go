//go:build e2e

package utils

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

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
