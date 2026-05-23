package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/boundary"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/pkg/serviceapi/servicefunction"
	"github.com/quarkloop/runtime/pkg/modelservice"
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
	Call(context.Context, serviceFunctionCall) (servicefunction.ResponseEnvelope, error)
}

type serviceFunctionCall struct {
	Subject  string
	Service  string
	Function string
	Payload  json.RawMessage
	RPC      *servicev1.RpcDescriptor
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
	cfg  NATSCallerConfig
	mu   sync.Mutex
	conn *natsgo.Conn
}

func NewNATSCaller(cfg NATSCallerConfig) *NATSCaller {
	return &NATSCaller{cfg: normalizeNATSCallerConfig(cfg)}
}

func (c *NATSCaller) Call(ctx context.Context, call serviceFunctionCall) (servicefunction.ResponseEnvelope, error) {
	if c == nil {
		return servicefunction.ResponseEnvelope{}, boundary.New(boundary.Runtime, boundary.Unavailable, call.Subject, "service function caller is not configured")
	}
	cfg := normalizeNATSCallerConfig(c.cfg)
	if cfg.URL == "" {
		return servicefunction.ResponseEnvelope{}, boundary.New(boundary.Runtime, boundary.Unavailable, call.Subject, "QUARK_NATS_URL is required for service function calls")
	}
	spaceID := strings.TrimSpace(modelservice.SpaceID(ctx))
	if spaceID == "" {
		spaceID = cfg.SpaceID
	}
	if spaceID == "" {
		return servicefunction.ResponseEnvelope{}, boundary.New(boundary.Runtime, boundary.InvalidArgument, call.Subject, "space id is required for service function calls")
	}
	timeout := serviceFunctionTimeout(call.RPC, cfg.Timeout)
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := c.connection()
	if err != nil {
		return servicefunction.ResponseEnvelope{}, boundary.Wrap(boundary.Service, boundary.Transport, call.Subject, err)
	}
	request := servicefunction.RequestEnvelope{
		Version:     servicefunction.EnvelopeVersion,
		CallID:      newServiceCallID(),
		SpaceID:     spaceID,
		SessionID:   modelservice.SessionID(ctx),
		RunID:       modelservice.RunID(ctx),
		Actor:       servicefunction.ActorRuntime,
		Service:     call.Service,
		Function:    call.Function,
		Subject:     call.Subject,
		Payload:     append(json.RawMessage(nil), call.Payload...),
		TraceParent: "",
	}
	if request.RunID != "" {
		request.AgentID = "main"
	}
	if err := request.Validate(); err != nil {
		return servicefunction.ResponseEnvelope{}, boundary.Wrap(boundary.Runtime, boundary.InvalidArgument, call.Subject, err)
	}
	data, err := json.Marshal(request)
	if err != nil {
		return servicefunction.ResponseEnvelope{}, boundary.Wrap(boundary.Runtime, boundary.Internal, call.Subject, err)
	}
	msg := natsgo.NewMsg(call.Subject)
	msg.Data = data
	for key, value := range request.CorrelationHeaders() {
		msg.Header.Set(key, value)
	}
	reply, err := conn.RequestMsgWithContext(callCtx, msg)
	if err != nil {
		return servicefunction.ResponseEnvelope{}, servicefunction.BoundaryError(err, boundary.Service, call.Subject)
	}
	var response servicefunction.ResponseEnvelope
	if err := json.Unmarshal(reply.Data, &response); err != nil {
		return servicefunction.ResponseEnvelope{}, boundary.Wrap(boundary.Service, boundary.InvalidArgument, call.Subject, err)
	}
	if err := response.Validate(); err != nil {
		return servicefunction.ResponseEnvelope{}, boundary.Wrap(boundary.Service, boundary.InvalidArgument, call.Subject, err)
	}
	return response, nil
}

func (c *NATSCaller) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		_ = c.conn.Drain()
		c.conn.Close()
		c.conn = nil
	}
}

func (c *NATSCaller) connection() (*natsgo.Conn, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil && c.conn.IsConnected() {
		return c.conn, nil
	}
	cfg := normalizeNATSCallerConfig(c.cfg)
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
		return nil, fmt.Errorf("connect nats service functions: %w", err)
	}
	if err := conn.FlushTimeout(cfg.Timeout); err != nil {
		conn.Close()
		return nil, fmt.Errorf("verify nats service functions: %w", err)
	}
	c.conn = conn
	return conn, nil
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

func newServiceCallID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return "svc-call-" + hex.EncodeToString(buf[:])
	}
	return fmt.Sprintf("svc-call-%d", time.Now().UnixNano())
}
