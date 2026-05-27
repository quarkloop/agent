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
	natsClientHost = "0.0.0.0"
	natsClientPort = 14222
	natsWebSocketHost = "0.0.0.0"
	natsWebSocketPort = 19222
	natsMonitorHost = "0.0.0.0"
	natsMonitorPort = 18222
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
	if cfg.Client.Host != natsClientHost || cfg.WebSocket.Host != natsWebSocketHost || cfg.Monitoring.Host != natsMonitorHost {
		t.Fatalf("hosts = client:%s ws:%s monitoring:%s", cfg.Client.Host, cfg.WebSocket.Host, cfg.Monitoring.Host)
	}
	if cfg.Client.Port != natsClientPort || cfg.WebSocket.Port != natsWebSocketPort || cfg.Monitoring.Port != natsMonitorPort {
		t.Fatalf("ports = client:%d ws:%d monitoring:%d", cfg.Client.Port, cfg.WebSocket.Port, cfg.Monitoring.Port)
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
	oldClientHost := natsClientHost
	oldClientPort := natsClientPort
	oldWebSocketHost := natsWebSocketHost
	oldWebSocketPort := natsWebSocketPort
	oldMonitorHost := natsMonitorHost
	oldMonitorPort := natsMonitorPort
	oldAuditRetention := natsAuditRetention
	oldAuditMaxMessages := natsAuditMaxMessages
	oldBundledPluginsDir := bundledPluginsDir
	oldInstalledPluginsDir := installedPluginsDir
	return func() {
		natsMode = oldMode
		natsExternalURL = oldURL
		natsStateDir = oldStateDir
		natsClientHost = oldClientHost
		natsClientPort = oldClientPort
		natsWebSocketHost = oldWebSocketHost
		natsWebSocketPort = oldWebSocketPort
		natsMonitorHost = oldMonitorHost
		natsMonitorPort = oldMonitorPort
		natsAuditRetention = oldAuditRetention
		natsAuditMaxMessages = oldAuditMaxMessages
		bundledPluginsDir = oldBundledPluginsDir
		installedPluginsDir = oldInstalledPluginsDir
	}
}
