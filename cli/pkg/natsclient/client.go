package natsclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
)

const (
	EnvURL      = "QUARK_NATS_URL"
	EnvUser     = "QUARK_NATS_USER"
	EnvPassword = "QUARK_NATS_PASSWORD"

	DefaultURL = "nats://127.0.0.1:4222"
)

type Config struct {
	URL           string
	Username      string
	Password      string
	Name          string
	Timeout       time.Duration
	ReconnectWait time.Duration
	MaxReconnects int
}

func ConfigFromEnv() Config {
	return Config{
		URL:           firstNonEmpty(os.Getenv(EnvURL), DefaultURL),
		Username:      os.Getenv(EnvUser),
		Password:      os.Getenv(EnvPassword),
		Name:          "quark-cli",
		Timeout:       5 * time.Second,
		ReconnectWait: 250 * time.Millisecond,
		MaxReconnects: 10,
	}
}

type Client struct {
	conn *nats.Conn
}

func Connect(ctx context.Context, cfg Config, opts ...nats.Option) (*Client, error) {
	normalized := normalizeConfig(cfg)
	options := []nats.Option{
		nats.Name(normalized.Name),
		nats.Timeout(normalized.Timeout),
		nats.ReconnectWait(normalized.ReconnectWait),
		nats.MaxReconnects(normalized.MaxReconnects),
		nats.RetryOnFailedConnect(true),
	}
	if normalized.Username != "" || normalized.Password != "" {
		options = append(options, nats.UserInfo(normalized.Username, normalized.Password))
	}
	options = append(options, opts...)

	if err := ctx.Err(); err != nil {
		return nil, ctx.Err()
	}
	conn, err := nats.Connect(normalized.URL, options...)
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}
	if err := ctx.Err(); err != nil {
		conn.Close()
		return nil, err
	}
	if err := conn.FlushTimeout(normalized.Timeout); err != nil {
		conn.Close()
		return nil, fmt.Errorf("verify nats connection: %w", err)
	}
	return &Client{conn: conn}, nil
}

func (c *Client) Close() {
	if c != nil && c.conn != nil {
		c.conn.Close()
	}
}

func (c *Client) Request(ctx context.Context, subject string, req clientcontract.RequestEnvelope) (clientcontract.ResponseEnvelope, error) {
	if c == nil || c.conn == nil {
		return clientcontract.ResponseEnvelope{}, errors.New("nats client is not connected")
	}
	if strings.TrimSpace(subject) == "" {
		return clientcontract.ResponseEnvelope{}, errors.New("subject is required")
	}
	if err := req.Validate(); err != nil {
		return clientcontract.ResponseEnvelope{}, err
	}
	data, err := json.Marshal(req)
	if err != nil {
		return clientcontract.ResponseEnvelope{}, fmt.Errorf("marshal request envelope: %w", err)
	}
	msg, err := c.conn.RequestWithContext(ctx, subject, data)
	if err != nil {
		return clientcontract.ResponseEnvelope{}, fmt.Errorf("request %s: %w", subject, err)
	}
	var resp clientcontract.ResponseEnvelope
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		return clientcontract.ResponseEnvelope{}, fmt.Errorf("decode response envelope: %w", err)
	}
	if err := resp.Validate(); err != nil {
		return clientcontract.ResponseEnvelope{}, err
	}
	return resp, nil
}

func normalizeConfig(cfg Config) Config {
	if strings.TrimSpace(cfg.URL) == "" {
		cfg.URL = DefaultURL
	}
	if strings.TrimSpace(cfg.Name) == "" {
		cfg.Name = "quark-cli"
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
