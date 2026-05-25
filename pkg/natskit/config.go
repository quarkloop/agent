package natskit

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	natsgo "github.com/nats-io/nats.go"
)

const (
	DefaultURL           = "nats://127.0.0.1:4222"
	DefaultUser          = "quark-control"
	DefaultTimeout       = 30 * time.Second
	DefaultReconnectWait = 250 * time.Millisecond
	DefaultMaxReconnects = 10
)

type Config struct {
	URL             string
	Username        string
	Password        string
	Name            string
	Timeout         time.Duration
	ReconnectWait   time.Duration
	MaxReconnects   int
	AuditPrefix     string
	TelemetryPrefix string
	AuditPolicy     AuditPolicy
	Logger          *slog.Logger
	AsyncError      func(error)
}

type Client struct {
	conn *natsgo.Conn
	cfg  Config
}

func Connect(ctx context.Context, cfg Config) (*Client, error) {
	cfg = normalizeConfig(cfg)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	options := []natsgo.Option{
		natsgo.Name(cfg.Name),
		natsgo.Timeout(cfg.Timeout),
		natsgo.ReconnectWait(cfg.ReconnectWait),
		natsgo.MaxReconnects(cfg.MaxReconnects),
		natsgo.RetryOnFailedConnect(true),
	}
	if cfg.AsyncError != nil {
		options = append(options, natsgo.ErrorHandler(func(_ *natsgo.Conn, _ *natsgo.Subscription, err error) {
			cfg.AsyncError(err)
		}))
	}
	if cfg.Username != "" || cfg.Password != "" {
		options = append(options, natsgo.UserInfo(cfg.Username, cfg.Password))
	}
	conn, err := natsgo.Connect(cfg.URL, options...)
	if err != nil {
		return nil, fmt.Errorf("connect nats application client: %w", err)
	}
	client := &Client{conn: conn, cfg: cfg}
	if err := client.Flush(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("verify nats application client: %w", err)
	}
	return client, nil
}

func (c *Client) Close() {
	if c == nil || c.conn == nil {
		return
	}
	_ = c.conn.Drain()
	c.conn.Close()
	c.conn = nil
}

func (c *Client) Flush(ctx context.Context) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("nats client is not connected")
	}
	if _, ok := ctx.Deadline(); ok {
		return c.conn.FlushWithContext(ctx)
	}
	return c.conn.FlushTimeout(c.cfg.Timeout)
}

func (c *Client) jetStream() (natsgo.JetStreamContext, error) {
	if c == nil || c.conn == nil {
		return nil, fmt.Errorf("nats client is not connected")
	}
	return c.conn.JetStream()
}

func normalizeConfig(cfg Config) Config {
	cfg.URL = firstNonEmpty(cfg.URL, DefaultURL)
	cfg.Name = firstNonEmpty(cfg.Name, "quark-app")
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.ReconnectWait <= 0 {
		cfg.ReconnectWait = DefaultReconnectWait
	}
	if cfg.MaxReconnects == 0 {
		cfg.MaxReconnects = DefaultMaxReconnects
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	cfg.AuditPrefix = strings.Trim(strings.TrimSpace(cfg.AuditPrefix), ".")
	cfg.TelemetryPrefix = strings.Trim(strings.TrimSpace(cfg.TelemetryPrefix), ".")
	return cfg
}
