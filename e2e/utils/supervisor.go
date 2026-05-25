//go:build e2e

package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func startSupervisor(t *testing.T, bins BuiltBinaries, extraEnv map[string]string) (string, string, NATSEndpoints) {
	t.Helper()

	spacesDir := filepath.Join(t.TempDir(), "spaces")
	if err := os.MkdirAll(spacesDir, 0o755); err != nil {
		t.Fatalf("mkdir spaces: %v", err)
	}
	natsClientPort := ReservePort(t)
	natsWebSocketPort := ReservePort(t)
	natsMonitorPort := ReservePort(t)
	natsStateDir := filepath.Join(t.TempDir(), "nats")
	installedPluginsDir := filepath.Join(t.TempDir(), "plugins")

	overrides := map[string]string{}
	for k, v := range extraEnv {
		overrides[k] = v
	}
	env := SupervisorProcessEnv(overrides)
	StartProcess(t, "supervisor", bins.Supervisor, []string{
		"start",
		"--bundled-plugins-dir", filepath.Join(QuarkRoot(t), "plugins"),
		"--installed-plugins-dir", installedPluginsDir,
		"--nats-state-dir", natsStateDir,
		"--nats-client-port", fmt.Sprint(natsClientPort),
		"--nats-websocket-port", fmt.Sprint(natsWebSocketPort),
		"--nats-monitor-port", fmt.Sprint(natsMonitorPort),
	}, env)

	natsEndpoints := NATSEndpoints{
		ClientURL:     fmt.Sprintf("nats://127.0.0.1:%d", natsClientPort),
		WebSocketURL:  fmt.Sprintf("ws://127.0.0.1:%d", natsWebSocketPort),
		MonitoringURL: fmt.Sprintf("http://127.0.0.1:%d", natsMonitorPort),
		StateDir:      natsStateDir,
	}
	waitForControlNATS(t, natsEndpoints, 10*time.Second)

	return "", spacesDir, natsEndpoints
}

// StartOptions tunes the fixture StartE2E builds. Zero-valued options yield
// the default behaviour (lib mode for tools when .so is available, binary
