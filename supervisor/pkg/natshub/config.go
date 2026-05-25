package natshub

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Mode string

const (
	ModeEmbedded Mode = "embedded"
	ModeExternal Mode = "external"

	SystemAccountName        = "SYS"
	ControlAccountName       = "CONTROL"
	ObservabilityAccountName = "OBSERVABILITY"

	DefaultControlUser       = "quark-control"
	DefaultControlPassword   = "quark-control-dev"
	DefaultSystemUser        = "quark-system"
	DefaultSystemPassword    = "quark-system-dev"
	DefaultObservabilityUser = "quark-observability"
	DefaultObservabilityPass = "quark-observability-dev"
)

const (
	defaultServerName     = "quark-supervisor-nats"
	defaultReadyTimeout   = 10 * time.Second
	defaultClientHost     = "127.0.0.1"
	defaultClientPort     = 4222
	defaultWebSocketHost  = "127.0.0.1"
	defaultWebSocketPort  = 9222
	defaultMonitoringHost = "127.0.0.1"
	defaultMonitoringPort = 8222
)

type ListenerConfig struct {
	Enabled bool
	Host    string
	Port    int
}

type JetStreamConfig struct {
	Enabled          bool
	StoreDir         string
	MaxMemory        int64
	MaxStore         int64
	Domain           string
	AuditRetention   time.Duration
	AuditMaxMessages int64
}

type PermissionConfig struct {
	PublishAllow   []string
	PublishDeny    []string
	SubscribeAllow []string
	SubscribeDeny  []string
}

type UserConfig struct {
	Name        string
	Password    string
	Permissions PermissionConfig
}

type AccountConfig struct {
	Name  string
	Users []UserConfig
}

type Config struct {
	Mode          Mode
	ExternalURL   string
	ServerName    string
	StateDir      string
	Client        ListenerConfig
	WebSocket     ListenerConfig
	Monitoring    ListenerConfig
	JetStream     JetStreamConfig
	SystemAccount string
	Accounts      []AccountConfig
	ReadyTimeout  time.Duration
	NoLog         bool
}

func DefaultStateDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".quarkloop", "supervisor", "nats"), nil
}

func DefaultConfig(stateDir string) Config {
	return Config{
		Mode:       ModeEmbedded,
		ServerName: defaultServerName,
		StateDir:   stateDir,
		Client:     ListenerConfig{Enabled: true, Host: defaultClientHost, Port: defaultClientPort},
		WebSocket:  ListenerConfig{Enabled: true, Host: defaultWebSocketHost, Port: defaultWebSocketPort},
		Monitoring: ListenerConfig{Enabled: true, Host: defaultMonitoringHost, Port: defaultMonitoringPort},
		JetStream: JetStreamConfig{
			Enabled:          true,
			StoreDir:         filepath.Join(stateDir, "jetstream"),
			AuditRetention:   90 * 24 * time.Hour,
			AuditMaxMessages: 10_000_000,
		},
		SystemAccount: SystemAccountName,
		Accounts:      DefaultAccounts(),
		ReadyTimeout:  defaultReadyTimeout,
	}
}

func DefaultAccounts() []AccountConfig {
	return []AccountConfig{
		{
			Name: SystemAccountName,
			Users: []UserConfig{{
				Name:        DefaultSystemUser,
				Password:    DefaultSystemPassword,
				Permissions: allowAllPermissions(),
			}},
		},
		{
			Name: ControlAccountName,
			Users: []UserConfig{{
				Name:        DefaultControlUser,
				Password:    DefaultControlPassword,
				Permissions: SupervisorPermissions(),
			}},
		},
		{
			Name: ObservabilityAccountName,
			Users: []UserConfig{{
				Name:        DefaultObservabilityUser,
				Password:    DefaultObservabilityPass,
				Permissions: allowAllPermissions(),
			}},
		},
	}
}

func Normalize(cfg Config) (Config, error) {
	if cfg.Mode == "" {
		cfg.Mode = ModeEmbedded
	}
	switch cfg.Mode {
	case ModeEmbedded:
		if strings.TrimSpace(cfg.StateDir) == "" {
			return Config{}, errors.New("nats state dir is required in embedded mode")
		}
		if strings.TrimSpace(cfg.ServerName) == "" {
			cfg.ServerName = defaultServerName
		}
		if strings.TrimSpace(cfg.Client.Host) == "" {
			cfg.Client.Host = defaultClientHost
		}
		cfg.Client.Enabled = true
		if cfg.WebSocket.Enabled && strings.TrimSpace(cfg.WebSocket.Host) == "" {
			cfg.WebSocket.Host = defaultWebSocketHost
		}
		if cfg.Monitoring.Enabled && strings.TrimSpace(cfg.Monitoring.Host) == "" {
			cfg.Monitoring.Host = defaultMonitoringHost
		}
		if cfg.JetStream.Enabled && strings.TrimSpace(cfg.JetStream.StoreDir) == "" {
			cfg.JetStream.StoreDir = filepath.Join(cfg.StateDir, "jetstream")
		}
		if cfg.JetStream.Enabled && cfg.JetStream.AuditRetention <= 0 {
			cfg.JetStream.AuditRetention = 90 * 24 * time.Hour
		}
		if cfg.JetStream.Enabled && cfg.JetStream.AuditMaxMessages <= 0 {
			cfg.JetStream.AuditMaxMessages = 10_000_000
		}
	case ModeExternal:
		if strings.TrimSpace(cfg.ExternalURL) == "" {
			return Config{}, errors.New("nats external url is required in external mode")
		}
	default:
		return Config{}, fmt.Errorf("unsupported nats mode %q", cfg.Mode)
	}
	if cfg.ReadyTimeout <= 0 {
		cfg.ReadyTimeout = defaultReadyTimeout
	}
	if strings.TrimSpace(cfg.SystemAccount) == "" {
		cfg.SystemAccount = SystemAccountName
	}
	if len(cfg.Accounts) == 0 {
		cfg.Accounts = DefaultAccounts()
	}
	if err := validateAccounts(cfg.SystemAccount, cfg.Accounts); err != nil {
		return Config{}, err
	}
	return cloneConfig(cfg), nil
}

func cloneConfig(in Config) Config {
	out := in
	out.Accounts = make([]AccountConfig, 0, len(in.Accounts))
	for _, account := range in.Accounts {
		cp := account
		cp.Users = make([]UserConfig, 0, len(account.Users))
		for _, user := range account.Users {
			userCp := user
			userCp.Permissions = clonePermissions(user.Permissions)
			cp.Users = append(cp.Users, userCp)
		}
		out.Accounts = append(out.Accounts, cp)
	}
	return out
}

func clonePermissions(in PermissionConfig) PermissionConfig {
	return PermissionConfig{
		PublishAllow:   append([]string(nil), in.PublishAllow...),
		PublishDeny:    append([]string(nil), in.PublishDeny...),
		SubscribeAllow: append([]string(nil), in.SubscribeAllow...),
		SubscribeDeny:  append([]string(nil), in.SubscribeDeny...),
	}
}
