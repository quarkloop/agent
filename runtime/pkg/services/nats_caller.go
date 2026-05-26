package services

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/quarkloop/pkg/boundary"
	"github.com/quarkloop/pkg/natskit"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/runtime/pkg/runcontext"
)

const (
	EnvNATSURL      = "QUARK_NATS_URL"
	EnvNATSUser     = "QUARK_NATS_USER"
	EnvNATSPassword = "QUARK_NATS_PASSWORD"
	EnvRuntimeID    = "QUARK_RUNTIME_ID"
	EnvSpaceID      = "QUARK_SPACE"

	defaultServiceNATSName = "quark-runtime-service-functions"
)

type serviceFunctionCaller interface {
	Call(context.Context, serviceFunctionCall) (natskit.ResponseEnvelope, error)
}

type serviceFunctionCall struct {
	Operation natskit.Operation
	Payload   json.RawMessage
	RPC       *servicev1.RpcDescriptor
}

type NATSCallerConfig struct {
	URL           string
	Username      string
	Password      string
	Name          string
	RuntimeID     string
	SpaceID       string
	Timeout       time.Duration
	ReconnectWait time.Duration
	MaxReconnects int
}

func NATSCallerConfigFromEnv() NATSCallerConfig {
	return NATSCallerConfig{
		URL:           strings.TrimSpace(os.Getenv(EnvNATSURL)),
		Username:      strings.TrimSpace(os.Getenv(EnvNATSUser)),
		Password:      strings.TrimSpace(os.Getenv(EnvNATSPassword)),
		Name:          defaultServiceNATSName,
		RuntimeID:     strings.TrimSpace(os.Getenv(EnvRuntimeID)),
		SpaceID:       strings.TrimSpace(os.Getenv(EnvSpaceID)),
		Timeout:       30 * time.Second,
		ReconnectWait: 250 * time.Millisecond,
		MaxReconnects: 10,
	}
}

type NATSCaller struct {
	cfg    NATSCallerConfig
	mu     sync.Mutex
	client *natskit.Client
}

func NewNATSCaller(cfg NATSCallerConfig) *NATSCaller {
	return &NATSCaller{cfg: normalizeNATSCallerConfig(cfg)}
}

func (c *NATSCaller) Call(ctx context.Context, call serviceFunctionCall) (natskit.ResponseEnvelope, error) {
	if c == nil {
		return natskit.ResponseEnvelope{}, boundary.New(boundary.Runtime, boundary.Unavailable, call.Operation.Subject, "service function caller is not configured")
	}
	cfg := normalizeNATSCallerConfig(c.cfg)
	if cfg.URL == "" {
		return natskit.ResponseEnvelope{}, boundary.New(boundary.Runtime, boundary.Unavailable, call.Operation.Subject, "QUARK_NATS_URL is required for service function calls")
	}
	spaceID := strings.TrimSpace(runcontext.SpaceID(ctx))
	if spaceID == "" {
		spaceID = cfg.SpaceID
	}
	if spaceID == "" {
		return natskit.ResponseEnvelope{}, boundary.New(boundary.Runtime, boundary.InvalidArgument, call.Operation.Subject, "space id is required for service function calls")
	}
	timeout := serviceFunctionTimeout(call.RPC, cfg.Timeout)
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client, err := c.connection(callCtx)
	if err != nil {
		return natskit.ResponseEnvelope{}, boundary.Wrap(boundary.Service, boundary.Transport, call.Operation.Subject, err)
	}
	request, err := natskit.NewRequest(natskit.NewServiceCallID(), spaceID, natskit.ActorRuntime, append(json.RawMessage(nil), call.Payload...))
	if err != nil {
		return natskit.ResponseEnvelope{}, boundary.Wrap(boundary.Runtime, boundary.InvalidArgument, call.Operation.Subject, err)
	}
	request.SessionID = runcontext.SessionID(ctx)
	request.RunID = runcontext.RunID(ctx)
	if request.RunID != "" {
		request.AgentID = "main"
	}
	if err := request.Validate(); err != nil {
		return natskit.ResponseEnvelope{}, boundary.Wrap(boundary.Runtime, boundary.InvalidArgument, call.Operation.Subject, err)
	}
	response, err := client.Call(callCtx, call.Operation, request)
	if err != nil {
		return natskit.ResponseEnvelope{}, boundary.FromError(boundary.Service, call.Operation.Subject, err)
	}
	return response, nil
}

func (c *NATSCaller) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil {
		c.client.Close()
		c.client = nil
	}
}

func (c *NATSCaller) connection(ctx context.Context) (*natskit.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil {
		return c.client, nil
	}
	cfg := normalizeNATSCallerConfig(c.cfg)
	client, err := natskit.Connect(ctx, natskit.Config{
		URL:           cfg.URL,
		Username:      cfg.Username,
		Password:      cfg.Password,
		Name:          cfg.Name,
		Timeout:       cfg.Timeout,
		ReconnectWait: cfg.ReconnectWait,
		MaxReconnects: cfg.MaxReconnects,
	})
	if err != nil {
		return nil, err
	}
	c.client = client
	return client, nil
}

func normalizeNATSCallerConfig(cfg NATSCallerConfig) NATSCallerConfig {
	cfg.URL = strings.TrimSpace(cfg.URL)
	cfg.Username = strings.TrimSpace(cfg.Username)
	cfg.Password = strings.TrimSpace(cfg.Password)
	cfg.Name = strings.TrimSpace(cfg.Name)
	cfg.RuntimeID = strings.TrimSpace(cfg.RuntimeID)
	cfg.SpaceID = strings.TrimSpace(cfg.SpaceID)
	if cfg.Name == "" {
		cfg.Name = defaultServiceNATSName
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.ReconnectWait <= 0 {
		cfg.ReconnectWait = 250 * time.Millisecond
	}
	if cfg.MaxReconnects == 0 {
		cfg.MaxReconnects = 10
	}
	return cfg
}

func serviceFunctionTimeout(rpc *servicev1.RpcDescriptor, fallback time.Duration) time.Duration {
	if rpc != nil && rpc.GetTimeoutMillis() > 0 {
		return time.Duration(rpc.GetTimeoutMillis()) * time.Millisecond
	}
	if fallback > 0 {
		return fallback
	}
	return 30 * time.Second
}
