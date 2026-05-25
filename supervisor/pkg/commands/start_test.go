package commands

import (
	"path/filepath"
	"testing"
	"time"

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
	natsArtifactHandoffMaxBytes = 64 * 1024 * 1024
	natsAuditRetention = 48 * time.Hour
	natsAuditMaxMessages = 2048

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
	if cfg.JetStream.ArtifactHandoffMaxBytes != natsArtifactHandoffMaxBytes {
		t.Fatalf("artifact handoff max bytes = %d", cfg.JetStream.ArtifactHandoffMaxBytes)
	}
	if cfg.JetStream.AuditRetention != natsAuditRetention || cfg.JetStream.AuditMaxMessages != natsAuditMaxMessages {
		t.Fatalf("audit retention config = duration:%s messages:%d", cfg.JetStream.AuditRetention, cfg.JetStream.AuditMaxMessages)
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
	oldArtifactHandoffMaxBytes := natsArtifactHandoffMaxBytes
	oldAuditRetention := natsAuditRetention
	oldAuditMaxMessages := natsAuditMaxMessages
	oldBundledPluginsDir := bundledPluginsDir
	return func() {
		natsMode = oldMode
		natsExternalURL = oldURL
		natsStateDir = oldStateDir
		natsClientPort = oldClientPort
		natsWebSocketPort = oldWebSocketPort
		natsMonitorPort = oldMonitorPort
		natsArtifactHandoffMaxBytes = oldArtifactHandoffMaxBytes
		natsAuditRetention = oldAuditRetention
		natsAuditMaxMessages = oldAuditMaxMessages
		bundledPluginsDir = oldBundledPluginsDir
	}
}
