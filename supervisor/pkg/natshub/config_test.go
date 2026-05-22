package natshub

import (
	"path/filepath"
	"testing"
	"time"
)

func TestBuildOptionsDefaultsAreDeterministic(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "nats"))
	cfg.Client.Port = 0
	cfg.WebSocket.Port = 0
	cfg.Monitoring.Port = 0
	cfg.NoLog = true

	opts, err := BuildOptions(cfg)
	if err != nil {
		t.Fatalf("build options: %v", err)
	}
	if opts.ServerName != defaultServerName {
		t.Fatalf("server name = %q", opts.ServerName)
	}
	if opts.Host != defaultClientHost || opts.Port != 0 {
		t.Fatalf("client listener = %s:%d", opts.Host, opts.Port)
	}
	if !opts.JetStream {
		t.Fatal("jetstream is not enabled")
	}
	if opts.StoreDir != filepath.Join(cfg.StateDir, "jetstream") {
		t.Fatalf("store dir = %q", opts.StoreDir)
	}
	if opts.SystemAccount != SystemAccountName {
		t.Fatalf("system account = %q", opts.SystemAccount)
	}
	if len(opts.Accounts) != 3 {
		t.Fatalf("accounts = %d", len(opts.Accounts))
	}
	if len(opts.Users) != 3 {
		t.Fatalf("users = %d", len(opts.Users))
	}
	if opts.Websocket.Host != defaultWebSocketHost || opts.Websocket.Port != 0 || !opts.Websocket.NoTLS {
		t.Fatalf("websocket config = %+v", opts.Websocket)
	}
	if opts.HTTPHost != defaultMonitoringHost || opts.HTTPPort != 0 {
		t.Fatalf("monitoring listener = %s:%d", opts.HTTPHost, opts.HTTPPort)
	}
}

func TestNormalizeRejectsInvalidAccountConfig(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "nats"))
	cfg.Accounts = []AccountConfig{{Name: ControlAccountName}}
	if _, err := Normalize(cfg); err == nil {
		t.Fatal("expected missing system account error")
	}

	cfg = DefaultConfig(filepath.Join(t.TempDir(), "nats"))
	cfg.Accounts = append(cfg.Accounts, AccountConfig{Name: ControlAccountName})
	if _, err := Normalize(cfg); err == nil {
		t.Fatal("expected duplicate account error")
	}

	cfg = DefaultConfig(filepath.Join(t.TempDir(), "nats"))
	cfg.Accounts[1].Users = append(cfg.Accounts[1].Users, UserConfig{Name: DefaultSystemUser, Password: "secret"})
	if _, err := Normalize(cfg); err == nil {
		t.Fatal("expected duplicate user error")
	}
}

func TestNormalizeCopiesMutableConfig(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "nats"))
	normalized, err := Normalize(cfg)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	cfg.Accounts[0].Users[0].Permissions.PublishAllow[0] = "mutated"
	if normalized.Accounts[0].Users[0].Permissions.PublishAllow[0] != ">" {
		t.Fatalf("normalized config reused caller backing array: %+v", normalized.Accounts[0].Users[0].Permissions)
	}
}

func TestNormalizeExternalModeRequiresURL(t *testing.T) {
	if _, err := Normalize(Config{Mode: ModeExternal}); err == nil {
		t.Fatal("expected external url error")
	}
	cfg, err := Normalize(Config{
		Mode:        ModeExternal,
		ExternalURL: "nats://127.0.0.1:4222",
		Accounts:    DefaultAccounts(),
	})
	if err != nil {
		t.Fatalf("normalize external: %v", err)
	}
	if cfg.ReadyTimeout != defaultReadyTimeout {
		t.Fatalf("ready timeout = %v", cfg.ReadyTimeout)
	}
}

func TestNormalizeEmbeddedRequiresStateDir(t *testing.T) {
	if _, err := Normalize(Config{Mode: ModeEmbedded, Accounts: DefaultAccounts()}); err == nil {
		t.Fatal("expected state dir error")
	}
}

func TestDefaultConfigAllowsTimeoutOverride(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "nats"))
	cfg.ReadyTimeout = time.Second
	normalized, err := Normalize(cfg)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if normalized.ReadyTimeout != time.Second {
		t.Fatalf("ready timeout = %v", normalized.ReadyTimeout)
	}
}
