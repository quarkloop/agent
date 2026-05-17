package serviceprocess_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/quarkloop/supervisor/pkg/api"
	"github.com/quarkloop/supervisor/pkg/serviceprocess"
)

func TestManagerStartsStopsAndExposesLogs(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is required for process lifecycle test")
	}

	manager := serviceprocess.NewManager()
	spec := serviceprocess.ProcessSpec{
		Space:      "space-1",
		Name:       "fake",
		Binary:     "sh",
		Args:       []string{"-c", "echo service-ready; exec sleep 30"},
		Env:        []string{"PATH=/usr/bin:/bin"},
		WorkingDir: t.TempDir(),
		Endpoint:   "127.0.0.1:1",
		LogPath:    filepath.Join(t.TempDir(), "fake.log"),
	}

	state, err := manager.Start(context.Background(), spec)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if state.PID == 0 || state.Status != api.ServiceStatusStarting {
		t.Fatalf("start state = %+v", state)
	}
	manager.MarkReady("space-1", "fake")
	state, ok := manager.Inspect("space-1", "fake")
	if !ok || state.Status != api.ServiceStatusReady {
		t.Fatalf("inspect state = %+v, ok=%v", state, ok)
	}

	waitForLog(t, manager, "space-1", "fake", "service-ready")
	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	state, err = manager.Stop(stopCtx, "space-1", "fake")
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if state.Status != api.ServiceStatusStopped || state.PID != 0 {
		t.Fatalf("stopped state = %+v", state)
	}
}

func TestManagerRejectsDuplicateStart(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is required for process lifecycle test")
	}

	manager := serviceprocess.NewManager()
	spec := serviceprocess.ProcessSpec{
		Space:      "space-1",
		Name:       "fake",
		Binary:     "sh",
		Args:       []string{"-c", "exec sleep 30"},
		Env:        []string{"PATH=/usr/bin:/bin"},
		WorkingDir: t.TempDir(),
		Endpoint:   "127.0.0.1:1",
		LogPath:    filepath.Join(t.TempDir(), "fake.log"),
	}
	if _, err := manager.Start(context.Background(), spec); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = manager.Stop(ctx, "space-1", "fake")
	})

	if _, err := manager.Start(context.Background(), spec); err == nil {
		t.Fatal("duplicate start unexpectedly succeeded")
	}
}

func TestManagerReportsMissingBinary(t *testing.T) {
	t.Parallel()
	manager := serviceprocess.NewManager()
	_, err := manager.Start(context.Background(), serviceprocess.ProcessSpec{
		Space:      "space-1",
		Name:       "missing",
		Binary:     filepath.Join(t.TempDir(), "missing-service"),
		WorkingDir: t.TempDir(),
		Endpoint:   "127.0.0.1:1",
		LogPath:    filepath.Join(t.TempDir(), "missing.log"),
	})
	if err == nil {
		t.Fatal("missing binary start unexpectedly succeeded")
	}
}

func TestManagerRecordsCrashedProcess(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is required for process lifecycle test")
	}

	manager := serviceprocess.NewManager()
	spec := serviceprocess.ProcessSpec{
		Space:      "space-1",
		Name:       "crashy",
		Binary:     "sh",
		Args:       []string{"-c", "exit 7"},
		Env:        []string{"PATH=/usr/bin:/bin"},
		WorkingDir: t.TempDir(),
		Endpoint:   "127.0.0.1:1",
		LogPath:    filepath.Join(t.TempDir(), "crashy.log"),
	}
	if _, err := manager.Start(context.Background(), spec); err != nil {
		t.Fatalf("start: %v", err)
	}
	waitForStatus(t, manager, "space-1", "crashy", api.ServiceStatusStopped)
	state, ok := manager.Inspect("space-1", "crashy")
	if !ok || len(state.Diagnostics) == 0 {
		t.Fatalf("crash state = %+v, ok=%v", state, ok)
	}
}

func TestManagerRepeatedStopReturnsError(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is required for process lifecycle test")
	}

	manager := serviceprocess.NewManager()
	spec := serviceprocess.ProcessSpec{
		Space:      "space-1",
		Name:       "fake",
		Binary:     "sh",
		Args:       []string{"-c", "exec sleep 30"},
		Env:        []string{"PATH=/usr/bin:/bin"},
		WorkingDir: t.TempDir(),
		Endpoint:   "127.0.0.1:1",
		LogPath:    filepath.Join(t.TempDir(), "fake.log"),
	}
	if _, err := manager.Start(context.Background(), spec); err != nil {
		t.Fatalf("start: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := manager.Stop(ctx, "space-1", "fake"); err != nil {
		t.Fatalf("first stop: %v", err)
	}
	if _, err := manager.Stop(ctx, "space-1", "fake"); err == nil {
		t.Fatal("second stop unexpectedly succeeded")
	}
}

func waitForLog(t *testing.T, manager *serviceprocess.Manager, space, service, want string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("log never contained %q", want)
		default:
			logs, _, ok, err := manager.Logs(space, service, 1024)
			if err != nil {
				t.Fatalf("logs: %v", err)
			}
			if ok && strings.Contains(logs, want) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func waitForStatus(t *testing.T, manager *serviceprocess.Manager, space, service string, want api.ServiceStatus) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("service never reached status %s", want)
		default:
			if state, ok := manager.Inspect(space, service); ok && state.Status == want {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}
