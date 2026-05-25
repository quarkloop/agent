package natsclient

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/quarkloop/pkg/boundary"
	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
)

const (
	EnvURL      = "QUARK_NATS_URL"
	EnvUser     = "QUARK_NATS_USER"
	EnvPassword = "QUARK_NATS_PASSWORD"

	DefaultURL      = "nats://127.0.0.1:4222"
	DefaultUser     = "quark-control"
	DefaultPassword = "quark-control-dev"
)

type Config struct {
	URL           string
	Username      string
	Password      string
	Name          string
	Timeout       time.Duration
	ReconnectWait time.Duration
	MaxReconnects int
	AsyncError    func(error)
}

func ConfigFromEnv() Config {
	return Config{
		URL:           firstNonEmpty(os.Getenv(EnvURL), DefaultURL),
		Username:      firstNonEmpty(os.Getenv(EnvUser), DefaultUser),
		Password:      firstNonEmpty(os.Getenv(EnvPassword), DefaultPassword),
		Name:          "quark-cli",
		Timeout:       5 * time.Second,
		ReconnectWait: 250 * time.Millisecond,
		MaxReconnects: 10,
	}
}

type Client struct {
	client  *natskit.Client
	timeout time.Duration
}

type ResponseError struct {
	Category boundary.Category
	Message  string
}

func (e *ResponseError) Error() string {
	if e == nil {
		return ""
	}
	if e.Category == "" {
		return e.Message
	}
	return string(e.Category) + ": " + e.Message
}

func IsNotFound(err error) bool {
	return responseHasCategory(err, boundary.NotFound)
}

func IsConflict(err error) bool {
	return responseHasCategory(err, boundary.Conflict)
}

func Connect(ctx context.Context, cfg Config) (*Client, error) {
	normalized := normalizeConfig(cfg)
	client, err := natskit.Connect(ctx, natskit.Config{
		URL: normalized.URL, Username: normalized.Username, Password: normalized.Password,
		Name: normalized.Name, Timeout: normalized.Timeout, ReconnectWait: normalized.ReconnectWait,
		MaxReconnects: normalized.MaxReconnects, AsyncError: normalized.AsyncError,
	})
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}
	return &Client{client: client, timeout: normalized.Timeout}, nil
}

func (c *Client) Close() {
	if c != nil && c.client != nil {
		c.client.Close()
	}
}

func (c *Client) Request(ctx context.Context, subject string, req clientcontract.RequestEnvelope) (clientcontract.ResponseEnvelope, error) {
	if c == nil || c.client == nil {
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
	reply, err := c.client.Request(ctx, subject, data, req.CorrelationHeaders())
	if err != nil {
		return clientcontract.ResponseEnvelope{}, fmt.Errorf("request %s: %w", subject, err)
	}
	var resp clientcontract.ResponseEnvelope
	if err := json.Unmarshal(reply, &resp); err != nil {
		return clientcontract.ResponseEnvelope{}, fmt.Errorf("decode response envelope: %w", err)
	}
	if err := resp.Validate(); err != nil {
		return clientcontract.ResponseEnvelope{}, err
	}
	return resp, nil
}

func ConnectFromEnv(ctx context.Context) (*Client, error) {
	return Connect(ctx, ConfigFromEnv())
}

func ConnectWithCredential(ctx context.Context, credential clientcontract.NATSCredential) (*Client, error) {
	cfg := ConfigFromEnv()
	if strings.TrimSpace(credential.URL) != "" {
		cfg.URL = credential.URL
	}
	cfg.Username = credential.Username
	cfg.Password = credential.Password
	if strings.TrimSpace(credential.SessionID) != "" {
		cfg.Name = "quark-cli-session-" + credential.SessionID
	}
	return Connect(ctx, cfg)
}

func notifySubscriptionError(errs chan<- error, err error) {
	select {
	case errs <- err:
	default:
	}
}

func (c *Client) flush(ctx context.Context) error {
	return c.client.Flush(ctx)
}
func requestPayload[T any](ctx context.Context, c *Client, subject, spaceID string, payload any) (T, error) {
	var out T
	req, err := clientcontract.NewRequest(newRequestID(), spaceID, payload)
	if err != nil {
		return out, err
	}
	resp, err := c.Request(ctx, subject, req)
	if err != nil {
		return out, err
	}
	if resp.Status == "error" {
		return out, responseError(resp)
	}
	if err := resp.DecodePayload(&out); err != nil {
		return out, err
	}
	return out, nil
}

func responseError(resp clientcontract.ResponseEnvelope) error {
	if resp.Error == nil {
		return &ResponseError{Category: boundary.Internal, Message: "missing response error"}
	}
	return &ResponseError{
		Category: boundary.Category(resp.Error.Category),
		Message:  resp.Error.Message,
	}
}

func responseHasCategory(err error, category boundary.Category) bool {
	var responseErr *ResponseError
	if errors.As(err, &responseErr) {
		return responseErr.Category == category
	}
	return boundary.IsCategory(err, category)
}

func newRequestID() string {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("req-%d", time.Now().UnixNano())
	}
	return "req-" + hex.EncodeToString(buf[:])
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
