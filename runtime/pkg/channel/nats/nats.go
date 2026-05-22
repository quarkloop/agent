// Package nats provides the runtime channel for NATS-native user sessions.
package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	natsgo "github.com/nats-io/nats.go"

	"github.com/quarkloop/pkg/boundary"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	"github.com/quarkloop/runtime/pkg/channel"
	"github.com/quarkloop/runtime/pkg/message"
	"github.com/quarkloop/runtime/pkg/session"
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

type SessionStore interface {
	Has(id string) bool
	GetOrCreate(id, sessionType, title string) *session.Conversation
}

type Channel struct {
	cfg      Config
	poster   message.Poster
	sessions SessionStore

	mu     sync.Mutex
	conn   *natsgo.Conn
	sub    *natsgo.Subscription
	ctx    context.Context
	cancel context.CancelFunc
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

func New(cfg Config, poster message.Poster, sessions SessionStore) *Channel {
	return &Channel{
		cfg:      normalizeConfig(cfg),
		poster:   poster,
		sessions: sessions,
	}
}

func (c *Channel) Type() channel.ChannelType { return channel.NATSChannelType }

func (c *Channel) Start(ctx context.Context) error {
	if c.poster == nil {
		return errors.New("message poster is required")
	}
	if c.sessions == nil {
		return errors.New("session store is required")
	}
	cfg := normalizeConfig(c.cfg)
	options := []natsgo.Option{
		natsgo.Name(cfg.Name),
		natsgo.Timeout(cfg.Timeout),
		natsgo.ReconnectWait(cfg.ReconnectWait),
		natsgo.MaxReconnects(cfg.MaxReconnects),
		natsgo.RetryOnFailedConnect(true),
	}
	if cfg.Username != "" || cfg.Password != "" {
		options = append(options, natsgo.UserInfo(cfg.Username, cfg.Password))
	}
	conn, err := natsgo.Connect(cfg.URL, options...)
	if err != nil {
		return fmt.Errorf("connect nats runtime channel: %w", err)
	}
	if err := conn.FlushTimeout(cfg.Timeout); err != nil {
		conn.Close()
		return fmt.Errorf("verify nats runtime channel: %w", err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	sub, err := conn.QueueSubscribe("session.*.input", cfg.Queue, c.handleInput)
	if err != nil {
		cancel()
		conn.Close()
		return fmt.Errorf("subscribe session input: %w", err)
	}
	if err := conn.FlushTimeout(cfg.Timeout); err != nil {
		cancel()
		_ = sub.Unsubscribe()
		conn.Close()
		return fmt.Errorf("flush session input subscription: %w", err)
	}

	c.mu.Lock()
	c.cfg = cfg
	c.conn = conn
	c.sub = sub
	c.ctx = runCtx
	c.cancel = cancel
	c.mu.Unlock()
	slog.Info("nats channel listening", "subject", "session.*.input", "queue", cfg.Queue)
	return nil
}

func (c *Channel) Stop(ctx context.Context) error {
	c.mu.Lock()
	conn := c.conn
	sub := c.sub
	cancel := c.cancel
	c.conn = nil
	c.sub = nil
	c.cancel = nil
	c.ctx = nil
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if sub != nil {
		_ = sub.Unsubscribe()
	}
	if conn == nil {
		return nil
	}
	done := make(chan error, 1)
	go func() {
		done <- conn.Drain()
		conn.Close()
	}()
	select {
	case <-ctx.Done():
		conn.Close()
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func (c *Channel) handleInput(msg *natsgo.Msg) {
	req, ok := decodeRequest(msg)
	if !ok {
		respond(msg, clientcontract.Error("unknown", string(boundary.InvalidArgument), "invalid request envelope"))
		return
	}
	var payload clientcontract.SendMessageRequest
	if err := req.DecodePayload(&payload); err != nil {
		respond(msg, clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), err.Error()))
		return
	}
	if err := validateSendMessage(payload); err != nil {
		respond(msg, clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), err.Error()))
		return
	}
	c.sessions.GetOrCreate(payload.SessionID, string(clientcontract.SessionTypeChat), "")
	ack, err := clientcontract.OK(req.RequestID, clientcontract.SendMessageResponse{
		SessionID: payload.SessionID,
		Accepted:  true,
	})
	if err != nil {
		respond(msg, clientcontract.Error(req.RequestID, string(boundary.Internal), err.Error()))
		return
	}
	respond(msg, ack)

	go c.postAndStream(c.requestContext(), payload)
}

func decodeRequest(msg *natsgo.Msg) (clientcontract.RequestEnvelope, bool) {
	var req clientcontract.RequestEnvelope
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		return clientcontract.RequestEnvelope{}, false
	}
	if err := req.Validate(); err != nil {
		return clientcontract.RequestEnvelope{}, false
	}
	return req.Clone(), true
}

func validateSendMessage(payload clientcontract.SendMessageRequest) error {
	if strings.TrimSpace(payload.SpaceID) == "" {
		return errors.New("space_id is required")
	}
	if strings.TrimSpace(payload.SessionID) == "" {
		return errors.New("session_id is required")
	}
	if strings.TrimSpace(payload.Content) == "" {
		return errors.New("content is required")
	}
	return nil
}

func (c *Channel) requestContext() context.Context {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ctx == nil {
		return context.Background()
	}
	return c.ctx
}

func (c *Channel) postAndStream(ctx context.Context, payload clientcontract.SendMessageRequest) {
	resp := make(chan message.StreamMessage, 64)
	c.poster.Post(ctx, message.PostRequest{
		SessionID: payload.SessionID,
		Content:   payload.Content,
	}, resp)
	for stream := range resp {
		if err := c.publishStreamEvent(payload.SessionID, stream); err != nil {
			slog.Error("publish nats session event", "session_id", payload.SessionID, "type", stream.Type, "error", err)
			return
		}
	}
	if err := c.publishEvent(clientcontract.SessionEvent{
		Type:      "done",
		SessionID: payload.SessionID,
	}); err != nil {
		slog.Error("publish nats session done event", "session_id", payload.SessionID, "error", err)
	}
}

func (c *Channel) publishStreamEvent(sessionID string, stream message.StreamMessage) error {
	payload, err := json.Marshal(stream.Data)
	if err != nil {
		return fmt.Errorf("marshal stream payload: %w", err)
	}
	return c.publishEvent(clientcontract.SessionEvent{
		Type:      stream.Type,
		SessionID: sessionID,
		Payload:   append(json.RawMessage(nil), payload...),
	})
}

func (c *Channel) publishEvent(event clientcontract.SessionEvent) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return errors.New("nats runtime channel is not connected")
	}
	subject, err := clientcontract.SessionEventsSubject(event.SessionID)
	if err != nil {
		return err
	}
	event.Payload = append(json.RawMessage(nil), event.Payload...)
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal session event: %w", err)
	}
	if err := conn.Publish(subject, data); err != nil {
		return fmt.Errorf("publish %s: %w", subject, err)
	}
	return nil
}

func respond(msg *natsgo.Msg, resp clientcontract.ResponseEnvelope) {
	data, err := json.Marshal(resp)
	if err != nil {
		data = []byte(`{"version":"v1","request_id":"unknown","status":"error","error":{"category":"internal","message":"marshal response"}}`)
	}
	_ = msg.Respond(data)
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
