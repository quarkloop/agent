package nats

import (
	"os"
	"strings"
	"time"
)

const (
	EnvURL      = "QUARK_NATS_URL"
	EnvUser     = "QUARK_NATS_USER"
	EnvPassword = "QUARK_NATS_PASSWORD"

	DefaultURL      = "nats://127.0.0.1:4222"
	DefaultUser     = "quark-runtime"
	DefaultPassword = ""
	DefaultQueue    = "q.runtime.sessions"
)

type Config struct {
	URL           string
	Username      string
	Password      string
	Name          string
	Queue         string
	Timeout       time.Duration
	ReconnectWait time.Duration
	MaxReconnects int
}

func ConfigFromEnv() Config {
	return Config{
		URL:           firstNonEmpty(os.Getenv(EnvURL), DefaultURL),
		Username:      firstNonEmpty(os.Getenv(EnvUser), DefaultUser),
		Password:      firstNonEmpty(os.Getenv(EnvPassword), DefaultPassword),
		Name:          "quark-runtime",
		Queue:         DefaultQueue,
		Timeout:       5 * time.Second,
		ReconnectWait: 250 * time.Millisecond,
		MaxReconnects: 10,
	}
}

func normalizeConfig(cfg Config) Config {
	if strings.TrimSpace(cfg.URL) == "" {
		cfg.URL = DefaultURL
	}
	if strings.TrimSpace(cfg.Name) == "" {
		cfg.Name = "quark-runtime"
	}
	if strings.TrimSpace(cfg.Queue) == "" {
		cfg.Queue = DefaultQueue
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	if cfg.ReconnectWait <= 0 {
		cfg.ReconnectWait = 250 * time.Millisecond
	}
	if cfg.MaxReconnects == 0 {
		cfg.MaxReconnects = 10
	}
	return cfg
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
