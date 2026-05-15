package runtime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/quarkloop/supervisor/pkg/runtime/launchenv"
)

func TestLauncherStartsAndStopsProcess(t *testing.T) {
	done := make(chan string, 1)
	launcher := NewLauncher(fakeRuntimeBinary(t), func(id string) {
		done <- id
	})
	registry := NewRegistry()
	rt := NewRuntime("rt-1", "space-1", t.TempDir(), "")
	registry.Register(rt)
	spec := launchenv.ProcessSpec{
		RuntimeID:  "rt-1",
		WorkingDir: rt.WorkingDir(),
		Port:       7777,
		Env:        []string{"PATH=" + os.Getenv("PATH")},
	}

	handle, err := launcher.Start(context.Background(), rt.ID(), spec)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := registry.MarkRunning(rt.ID(), handle); err != nil {
		t.Fatalf("mark running: %v", err)
	}
	if err := registry.MarkStopping(rt.ID()); err != nil {
		t.Fatalf("mark stopping: %v", err)
	}
	if err := launcher.Stop(rt); err != nil {
		t.Fatalf("stop: %v", err)
	}
	select {
	case id := <-done:
		if id != "rt-1" {
			t.Fatalf("stopped id = %q", id)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("runtime process did not stop")
	}
	registry.SetStopped(rt.ID())
}

func TestLauncherFailedLaunch(t *testing.T) {
	launcher := NewLauncher(filepath.Join(t.TempDir(), "missing-runtime"), nil)
	_, err := launcher.Start(context.Background(), "rt-1", launchenv.ProcessSpec{
		RuntimeID:  "rt-1",
		WorkingDir: t.TempDir(),
		Port:       7777,
	})
	if err == nil {
		t.Fatal("expected failed launch")
	}
}

func fakeRuntimeBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "runtime")
	script := `#!/bin/sh
if [ "$1" != "start" ]; then
  exit 2
fi
while true; do
  sleep 1
done
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
