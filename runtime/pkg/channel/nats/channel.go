// Package nats provides the runtime channel for NATS-native user sessions.
package nats

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	"github.com/quarkloop/runtime/pkg/activity"
	"github.com/quarkloop/runtime/pkg/channel"
	"github.com/quarkloop/runtime/pkg/message"
	"github.com/quarkloop/runtime/pkg/plan"
	"github.com/quarkloop/runtime/pkg/session"
)

type SessionStore interface {
	Has(id string) bool
	List() []*session.Conversation
	GetOrCreate(id, sessionType, title string) *session.Conversation
}

type Option func(*Channel)

func WithPlan(plan *plan.Plan) Option {
	return func(c *Channel) {
		c.plan = plan
	}
}

func WithActivity(store *activity.Store) Option {
	return func(c *Channel) {
		c.activity = store
	}
}

type Channel struct {
	cfg      Config
	poster   message.Poster
	sessions SessionStore
	plan     *plan.Plan
	activity *activity.Store

	mu     sync.Mutex
	host   *natskit.ApplicationHost
	ctx    context.Context
	cancel context.CancelFunc
}

func New(cfg Config, poster message.Poster, sessions SessionStore, opts ...Option) *Channel {
	natsChannel := &Channel{
		cfg:      normalizeConfig(cfg),
		poster:   poster,
		sessions: sessions,
	}
	for _, opt := range opts {
		opt(natsChannel)
	}
	return natsChannel
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
	host, err := natskit.NewApplicationHost(ctx, natskit.Config{
		URL: cfg.URL, Username: cfg.Username, Password: cfg.Password, Name: cfg.Name,
		Timeout: cfg.Timeout, ReconnectWait: cfg.ReconnectWait, MaxReconnects: cfg.MaxReconnects,
	}, cfg.Queue)
	if err != nil {
		return fmt.Errorf("connect nats runtime channel: %w", err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.cfg = cfg
	c.host = host
	c.ctx = runCtx
	c.cancel = cancel
	c.mu.Unlock()
	if err := c.registerHandlers(host); err != nil {
		c.mu.Lock()
		if c.host == host {
			c.host = nil
			c.ctx = nil
			c.cancel = nil
		}
		c.mu.Unlock()
		cancel()
		host.Close()
		return err
	}
	c.forwardActivity(runCtx)
	slog.Info("nats channel listening", "subject", clientcontract.SubjectSessionInputWildcard, "queue", cfg.Queue)
	return nil
}

func (c *Channel) registerHandlers(host *natskit.ApplicationHost) error {
	handlers := []struct {
		name    string
		subject string
		handler func(natskit.Message) clientcontract.ResponseEnvelope
	}{
		{"session.input", clientcontract.SubjectSessionInputWildcard, c.handleInput},
		{"runtime.info.get", clientcontract.SubjectRuntimeInfoGet, c.handleInfoGet},
		{"runtime.session.get", clientcontract.SubjectRuntimeSessionGet, c.handleSessionGet},
		{"runtime.plan.get", clientcontract.SubjectRuntimePlanGet, c.handlePlanGet},
		{"runtime.plan.approve", clientcontract.SubjectRuntimePlanApprove, c.handlePlanApprove},
		{"runtime.plan.reject", clientcontract.SubjectRuntimePlanReject, c.handlePlanReject},
		{"runtime.activity.list", clientcontract.SubjectRuntimeActivityList, c.handleActivityList},
	}
	for _, registration := range handlers {
		operation, err := natskit.NewApplicationOperation(registration.name, registration.subject)
		if err != nil {
			return fmt.Errorf("build runtime operation %s: %w", registration.name, err)
		}
		handler := registration.handler
		if err := host.Register(operation, func(_ context.Context, msg natskit.Message) ([]byte, error) {
			return encodeResponse(handler(msg)), nil
		}); err != nil {
			return err
		}
	}
	if err := host.Ready(context.Background()); err != nil {
		return fmt.Errorf("flush runtime subscriptions: %w", err)
	}
	return nil
}

func (c *Channel) Stop(ctx context.Context) error {
	c.mu.Lock()
	host := c.host
	cancel := c.cancel
	c.host = nil
	c.cancel = nil
	c.ctx = nil
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if host == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		host.Close()
		return err
	}
	host.Close()
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
