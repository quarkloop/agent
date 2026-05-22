package commands

import (
	"path/filepath"
	"testing"

	"github.com/quarkloop/supervisor/pkg/natshub"
)

func TestStartNATSConfigBuildsEmbeddedConfig(t *testing.T) {
	restore := captureStartNATSFlags()
	defer restore()

	natsMode = string(natshub.ModeEmbedded)
	natsStateDir = filepath.Join(t.TempDir(), "nats")
	natsClientPort = 14222
	natsWebSocketPort = 19222
	natsMonitorPort = 18222

	cfg, err := startNATSConfig()
	if err != nil {
		t.Fatalf("start nats config: %v", err)
	}
	if cfg.Mode != natshub.ModeEmbedded {
		t.Fatalf("mode = %q", cfg.Mode)
	}
	if cfg.StateDir != natsStateDir {
		t.Fatalf("state dir = %q", cfg.StateDir)
	}
	if cfg.Client.Port != natsClientPort || cfg.WebSocket.Port != natsWebSocketPort || cfg.Monitoring.Port != natsMonitorPort {
		t.Fatalf("ports = client:%d ws:%d monitoring:%d", cfg.Client.Port, cfg.WebSocket.Port, cfg.Monitoring.Port)
	}
}

func TestStartNATSConfigBuildsExternalConfig(t *testing.T) {
	restore := captureStartNATSFlags()
	defer restore()

	natsMode = string(natshub.ModeExternal)
	natsExternalURL = "nats://127.0.0.1:4222"

	cfg, err := startNATSConfig()
	if err != nil {
		t.Fatalf("start nats config: %v", err)
	}
	if cfg.Mode != natshub.ModeExternal || cfg.ExternalURL != natsExternalURL {
		t.Fatalf("external config = %+v", cfg)
	}
}

func TestStartNATSConfigRejectsUnsupportedMode(t *testing.T) {
	restore := captureStartNATSFlags()
	defer restore()

	natsMode = "invalid"
	if _, err := startNATSConfig(); err == nil {
		t.Fatal("expected unsupported mode error")
	}
}

func captureStartNATSFlags() func() {
	oldMode := natsMode
	oldURL := natsExternalURL
	oldStateDir := natsStateDir
	oldClientPort := natsClientPort
	oldWebSocketPort := natsWebSocketPort
	oldMonitorPort := natsMonitorPort
	return func() {
		natsMode = oldMode
		natsExternalURL = oldURL
		natsStateDir = oldStateDir
		natsClientPort = oldClientPort
		natsWebSocketPort = oldWebSocketPort
		natsMonitorPort = oldMonitorPort
	}
}
