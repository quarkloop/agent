// Package runtime handles launching and stopping runtime processes.
package runtime

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/quarkloop/supervisor/pkg/api"
	"github.com/quarkloop/supervisor/pkg/runtime/launchenv"
)

// StopCallback is invoked under the registry lock when a runtime process exits.
type StopCallback func(runtimeID string)

// Launcher manages runtime process lifecycle.
type Launcher struct {
	runtimeBin string
	onStop     StopCallback
}

// NewLauncher creates a Launcher that spawns the given runtime binary.
// onStop is called under the registry lock when a runtime process exits.
func NewLauncher(runtimeBin string, onStop StopCallback) *Launcher {
	return &Launcher{runtimeBin: runtimeBin, onStop: onStop}
}

// Start launches a runtime process for the registry entry. On success it
// sets entry.Cmd, entry.PID, entry.Status = RuntimeRunning. When the
// process exits the status is transitioned to RuntimeStopped.
func (l *Launcher) Start(ctx context.Context, rt *Runtime, spec launchenv.ProcessSpec) error {
	if spec.Port == 0 {
		return fmt.Errorf("launch runtime %s: port not assigned", rt.ID())
	}
	// Use a detached context: the child runtime's lifetime is owned by the
	// registry, not by the HTTP request that spawned it.
	// ctx is intentionally unused; the goroutine manages its own lifecycle.
	cmd := exec.Command(l.runtimeBin,
		"start",
		"--port", fmt.Sprintf("%d", spec.Port),
	)
	cmd.Dir = spec.WorkingDir
	cmd.Env = append([]string(nil), spec.Env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launch runtime %s: %w", rt.ID(), err)
	}

	rt.SetCmd(cmd)
	rt.SetPID(cmd.Process.Pid)
	rt.SetStartedAt(time.Now().UTC())
	rt.SetStatus(api.RuntimeRunning)

	go func() {
		if err := cmd.Wait(); err != nil {
			slog.Error("runtime exited with error", "runtime_id", rt.ID(), "error", err)
		}
		if l.onStop != nil {
			l.onStop(rt.ID())
		}
	}()

	return nil
}

// Stop sends SIGTERM to the runtime process. The caller must hold the registry
// write lock for the duration of this call to avoid a data race on rt.Status.
func (l *Launcher) Stop(rt *Runtime) error {
	if rt.Cmd() == nil || rt.Cmd().Process == nil {
		return fmt.Errorf("runtime %s is not running", rt.ID())
	}
	rt.SetStatus(api.RuntimeStopping)
	if err := rt.Cmd().Process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("signal runtime %s: %w", rt.ID(), err)
	}
	return nil
}
