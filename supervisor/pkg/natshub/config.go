package natshub

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
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
	Enabled   bool
	StoreDir  string
	MaxMemory int64
	MaxStore  int64
	Domain    string
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
		Mode:          ModeEmbedded,
		ServerName:    defaultServerName,
		StateDir:      stateDir,
		Client:        ListenerConfig{Enabled: true, Host: defaultClientHost, Port: defaultClientPort},
		WebSocket:     ListenerConfig{Enabled: true, Host: defaultWebSocketHost, Port: defaultWebSocketPort},
		Monitoring:    ListenerConfig{Enabled: true, Host: defaultMonitoringHost, Port: defaultMonitoringPort},
		JetStream:     JetStreamConfig{Enabled: true, StoreDir: filepath.Join(stateDir, "jetstream")},
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
				Permissions: allowAllPermissions(),
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

func BuildOptions(cfg Config) (*natsserver.Options, error) {
	normalized, err := Normalize(cfg)
	if err != nil {
		return nil, err
	}
	if normalized.Mode != ModeEmbedded {
		return nil, errors.New("embedded nats options requested for external mode")
	}
	accounts, users := buildAccounts(normalized.Accounts)
	opts := &natsserver.Options{
		ServerName:         normalized.ServerName,
		Host:               normalized.Client.Host,
		Port:               normalized.Client.Port,
		NoSigs:             true,
		NoLog:              normalized.NoLog,
		Accounts:           accounts,
		Users:              users,
		SystemAccount:      normalized.SystemAccount,
		JetStream:          normalized.JetStream.Enabled,
		StoreDir:           normalized.JetStream.StoreDir,
		JetStreamDomain:    normalized.JetStream.Domain,
		JetStreamMaxMemory: normalized.JetStream.MaxMemory,
		JetStreamMaxStore:  normalized.JetStream.MaxStore,
	}
	if normalized.Monitoring.Enabled {
		opts.HTTPHost = normalized.Monitoring.Host
		opts.HTTPPort = normalized.Monitoring.Port
	}
	if normalized.WebSocket.Enabled {
		opts.Websocket = natsserver.WebsocketOpts{
			Host:  normalized.WebSocket.Host,
			Port:  normalized.WebSocket.Port,
			NoTLS: true,
		}
	}
	return opts, nil
}

func validateAccounts(systemAccount string, accounts []AccountConfig) error {
	names := make(map[string]struct{}, len(accounts))
	hasSystem := false
	users := map[string]struct{}{}
	for _, account := range accounts {
		name := strings.TrimSpace(account.Name)
		if name == "" {
			return errors.New("nats account name is required")
		}
		if _, ok := names[name]; ok {
			return fmt.Errorf("duplicate nats account %q", name)
		}
		names[name] = struct{}{}
		if name == systemAccount {
			hasSystem = true
		}
		for _, user := range account.Users {
			userName := strings.TrimSpace(user.Name)
			if userName == "" {
				return fmt.Errorf("nats account %q has a user without a name", name)
			}
			if strings.TrimSpace(user.Password) == "" {
				return fmt.Errorf("nats user %q in account %q requires a password", userName, name)
			}
			if _, ok := users[userName]; ok {
				return fmt.Errorf("duplicate nats user %q", userName)
			}
			users[userName] = struct{}{}
		}
	}
	if !hasSystem {
		return fmt.Errorf("nats system account %q is not declared", systemAccount)
	}
	return nil
}

func buildAccounts(configs []AccountConfig) ([]*natsserver.Account, []*natsserver.User) {
	accounts := make([]*natsserver.Account, 0, len(configs))
	users := make([]*natsserver.User, 0)
	for _, config := range configs {
		account := natsserver.NewAccount(strings.TrimSpace(config.Name))
		accounts = append(accounts, account)
		for _, user := range config.Users {
			users = append(users, &natsserver.User{
				Username:    strings.TrimSpace(user.Name),
				Password:    user.Password,
				Account:     account,
				Permissions: toServerPermissions(user.Permissions),
			})
		}
	}
	return accounts, users
}

func toServerPermissions(config PermissionConfig) *natsserver.Permissions {
	if isEmptyPermissions(config) {
		return nil
	}
	return &natsserver.Permissions{
		Publish: &natsserver.SubjectPermission{
			Allow: append([]string(nil), config.PublishAllow...),
			Deny:  append([]string(nil), config.PublishDeny...),
		},
		Subscribe: &natsserver.SubjectPermission{
			Allow: append([]string(nil), config.SubscribeAllow...),
			Deny:  append([]string(nil), config.SubscribeDeny...),
		},
	}
}

func isEmptyPermissions(config PermissionConfig) bool {
	return len(config.PublishAllow) == 0 &&
		len(config.PublishDeny) == 0 &&
		len(config.SubscribeAllow) == 0 &&
		len(config.SubscribeDeny) == 0
}

func allowAllPermissions() PermissionConfig {
	return PermissionConfig{
		PublishAllow:   []string{">"},
		SubscribeAllow: []string{">"},
	}
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
