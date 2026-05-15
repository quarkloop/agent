package runtime

import (
	"os/exec"
	"testing"
	"time"

	"github.com/quarkloop/supervisor/pkg/api"
)

func TestRegistryRunningStoppingStoppedTransitions(t *testing.T) {
	registry := NewRegistry()
	rt := NewRuntime("rt-1", "space-1", "/tmp/space", "/tmp/plugins")
	registry.Register(rt)

	started := time.Now().UTC()
	if err := registry.MarkRunning("rt-1", ProcessHandle{Cmd: exec.Command("true"), PID: 123, StartedAt: started}); err != nil {
		t.Fatalf("mark running: %v", err)
	}
	got, err := registry.Get("rt-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status() != api.RuntimeRunning || got.PID() != 123 || !got.StartedAt().Equal(started) {
		t.Fatalf("running state = %+v", got)
	}

	if err := registry.MarkStopping("rt-1"); err != nil {
		t.Fatalf("mark stopping: %v", err)
	}
	if got.Status() != api.RuntimeStopping {
		t.Fatalf("status = %s", got.Status())
	}

	registry.SetStopped("rt-1")
	if got.Status() != api.RuntimeStopped || got.PID() != 0 || got.cmd != nil {
		t.Fatalf("stopped state = %+v", got)
	}
}

func TestRegistryRepeatedStopIsRejected(t *testing.T) {
	registry := NewRegistry()
	rt := NewRuntime("rt-1", "space-1", "/tmp/space", "/tmp/plugins")
	registry.Register(rt)
	if err := registry.MarkRunning("rt-1", ProcessHandle{Cmd: exec.Command("true"), PID: 123, StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := registry.MarkStopping("rt-1"); err != nil {
		t.Fatal(err)
	}
	if err := registry.MarkStopping("rt-1"); err == nil {
		t.Fatal("expected repeated stop to fail")
	}
}
