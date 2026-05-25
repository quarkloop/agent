package server

import (
	"path/filepath"
	"testing"

	"github.com/quarkloop/supervisor/pkg/natshub"
)

func TestDefaultInstalledPluginsDirIsSupervisorStateSibling(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "nats")

	got, err := defaultInstalledPluginsDir(natshub.Config{StateDir: stateDir})
	if err != nil {
		t.Fatalf("resolve installed plugin directory: %v", err)
	}
	want := filepath.Join(filepath.Dir(stateDir), "plugins")
	if got != want {
		t.Fatalf("installed plugin directory = %q, want %q", got, want)
	}
}
