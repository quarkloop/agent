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

	"github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/boundary"
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
	conn *nats.Conn
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

func ConnectFromEnv(ctx context.Context) (*Client, error) {
	return Connect(ctx, ConfigFromEnv())
}

func (c *Client) CreateSpace(ctx context.Context, req clientcontract.CreateSpaceRequest) (clientcontract.SpaceInfo, error) {
	return requestPayload[clientcontract.SpaceInfo](ctx, c, clientcontract.SubjectSpaceCreate, "", req)
}

func (c *Client) ListSpaces(ctx context.Context) (clientcontract.ListSpacesResponse, error) {
	return requestPayload[clientcontract.ListSpacesResponse](ctx, c, clientcontract.SubjectSpaceList, "", struct{}{})
}

func (c *Client) GetSpace(ctx context.Context, name string) (clientcontract.SpaceInfo, error) {
	return requestPayload[clientcontract.SpaceInfo](ctx, c, clientcontract.SubjectSpaceGet, "", clientcontract.GetSpaceRequest{Name: name})
}

func (c *Client) UpdateSpace(ctx context.Context, name string, quarkfile []byte) (clientcontract.SpaceInfo, error) {
	return requestPayload[clientcontract.SpaceInfo](ctx, c, clientcontract.SubjectSpaceUpdate, name, clientcontract.UpdateSpaceRequest{
		Name:      name,
		Quarkfile: append([]byte(nil), quarkfile...),
	})
}

func (c *Client) CreateSession(ctx context.Context, req clientcontract.CreateSessionRequest) (clientcontract.SessionInfo, error) {
	return requestPayload[clientcontract.SessionInfo](ctx, c, clientcontract.SubjectSessionCreate, req.SpaceID, req)
}

func (c *Client) ListSessions(ctx context.Context, spaceID string) (clientcontract.ListSessionsResponse, error) {
	return requestPayload[clientcontract.ListSessionsResponse](ctx, c, clientcontract.SubjectSessionList, spaceID, clientcontract.ListSessionsRequest{SpaceID: spaceID})
}

func (c *Client) GetSession(ctx context.Context, spaceID, sessionID string) (clientcontract.SessionInfo, error) {
	return requestPayload[clientcontract.SessionInfo](ctx, c, clientcontract.SubjectSessionGet, spaceID, clientcontract.SessionRefRequest{
		SpaceID:   spaceID,
		SessionID: sessionID,
	})
}

func (c *Client) DeleteSession(ctx context.Context, spaceID, sessionID string) error {
	_, err := requestPayload[struct{}](ctx, c, clientcontract.SubjectSessionDelete, spaceID, clientcontract.SessionRefRequest{
		SpaceID:   spaceID,
		SessionID: sessionID,
	})
	return err
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
