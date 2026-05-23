package secretsnats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/boundary"
	secretsv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/secrets/v1"
	"github.com/quarkloop/pkg/serviceapi/servicefunction"
	"github.com/quarkloop/services/secrets/internal/secretssvc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	DefaultURL                = "nats://127.0.0.1:4222"
	DefaultUser               = "quark-control"
	DefaultQueue              = "q.secrets.v1"
	DefaultTimeout            = 30 * time.Second
	serviceName               = "secrets"
	serviceVersion            = "v1"
	functionResolveRef        = "resolve_ref"
	functionIssueScopedSecret = "issue_scoped_secret"
	functionRenewLease        = "renew_lease"
	functionRevokeLease       = "revoke_lease"
	functionRotateSecret      = "rotate_secret"
	functionAuditAccess       = "audit_access"
)

type Config struct {
	URL           string
	Username      string
	Password      string
	Queue         string
	Name          string
	Timeout       time.Duration
	ReconnectWait time.Duration
	MaxReconnects int
	Logger        *slog.Logger
}

type Server struct {
	cfg     Config
	secrets *secretssvc.Server
	conn    *natsgo.Conn
	subs    []*natsgo.Subscription
}

func New(cfg Config, secrets *secretssvc.Server) *Server {
	return &Server{cfg: normalizeConfig(cfg), secrets: secrets}
}

func (s *Server) Start(ctx context.Context) error {
	if s == nil || s.secrets == nil {
		return fmt.Errorf("secrets server is required")
	}
	conn, err := natsgo.Connect(
		s.cfg.URL,
		natsgo.UserInfo(s.cfg.Username, s.cfg.Password),
		natsgo.Name(s.cfg.Name),
		natsgo.Timeout(s.cfg.Timeout),
		natsgo.ReconnectWait(s.cfg.ReconnectWait),
		natsgo.MaxReconnects(s.cfg.MaxReconnects),
	)
	if err != nil {
		return fmt.Errorf("connect nats: %w", err)
	}
	s.conn = conn
	for _, fn := range []string{
		functionResolveRef,
		functionIssueScopedSecret,
		functionRenewLease,
		functionRevokeLease,
		functionRotateSecret,
		functionAuditAccess,
	} {
		subject, err := servicefunction.Subject(serviceName, serviceVersion, fn)
		if err != nil {
			s.Close()
			return err
		}
		sub, err := conn.QueueSubscribe(subject, s.cfg.Queue, s.handle)
		if err != nil {
			s.Close()
			return fmt.Errorf("subscribe %s: %w", subject, err)
		}
		s.subs = append(s.subs, sub)
	}
	if err := conn.FlushTimeout(s.cfg.Timeout); err != nil {
		s.Close()
		return fmt.Errorf("flush secrets subscriptions: %w", err)
	}
	s.cfg.Logger.Info("secrets nats endpoints ready", "url", s.cfg.URL, "queue", s.cfg.Queue)
	go func() {
		<-ctx.Done()
		s.Close()
	}()
	return nil
}

func (s *Server) Close() {
	if s == nil {
		return
	}
	for _, sub := range s.subs {
		_ = sub.Unsubscribe()
	}
	s.subs = nil
	if s.conn != nil {
		s.conn.Drain()
		s.conn.Close()
		s.conn = nil
	}
}

func (s *Server) handle(msg *natsgo.Msg) {
	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.Timeout)
	defer cancel()

	req, err := decodeRequest(msg.Data)
	if err != nil {
		s.respond(msg, servicefunction.ErrorResponse("", err, boundary.Service, "secrets.decode_request"))
		return
	}
	if err := req.Validate(); err != nil {
		s.respond(msg, servicefunction.ErrorResponse(req.CallID, err, boundary.Service, req.Subject))
		return
	}
	switch req.Function {
	case functionResolveRef:
		s.handleUnary(ctx, msg, req, &secretsv1.ResolveRefRequest{}, func(ctx context.Context, request proto.Message) (proto.Message, error) {
			return s.secrets.ResolveRef(ctx, request.(*secretsv1.ResolveRefRequest))
		})
	case functionIssueScopedSecret:
		s.handleUnary(ctx, msg, req, &secretsv1.IssueScopedSecretRequest{}, func(ctx context.Context, request proto.Message) (proto.Message, error) {
			return s.secrets.IssueScopedSecret(ctx, request.(*secretsv1.IssueScopedSecretRequest))
		})
	case functionRenewLease:
		s.handleUnary(ctx, msg, req, &secretsv1.RenewLeaseRequest{}, func(ctx context.Context, request proto.Message) (proto.Message, error) {
			return s.secrets.RenewLease(ctx, request.(*secretsv1.RenewLeaseRequest))
		})
	case functionRevokeLease:
		s.handleUnary(ctx, msg, req, &secretsv1.RevokeLeaseRequest{}, func(ctx context.Context, request proto.Message) (proto.Message, error) {
			return s.secrets.RevokeLease(ctx, request.(*secretsv1.RevokeLeaseRequest))
		})
	case functionRotateSecret:
		s.handleUnary(ctx, msg, req, &secretsv1.RotateSecretRequest{}, func(ctx context.Context, request proto.Message) (proto.Message, error) {
			return s.secrets.RotateSecret(ctx, request.(*secretsv1.RotateSecretRequest))
		})
	case functionAuditAccess:
		s.handleUnary(ctx, msg, req, &secretsv1.AuditAccessRequest{}, func(ctx context.Context, request proto.Message) (proto.Message, error) {
			return s.secrets.AuditAccess(ctx, request.(*secretsv1.AuditAccessRequest))
		})
	default:
		s.respond(msg, servicefunction.ErrorResponse(req.CallID, fmt.Errorf("unknown secrets function %q", req.Function), boundary.Service, req.Subject))
	}
}

type unaryHandler func(context.Context, proto.Message) (proto.Message, error)

func (s *Server) handleUnary(ctx context.Context, msg *natsgo.Msg, req servicefunction.RequestEnvelope, request proto.Message, handler unaryHandler) {
	if err := protojson.Unmarshal(req.Payload, request); err != nil {
		s.respond(msg, servicefunction.ErrorResponse(req.CallID, err, boundary.Service, req.Subject))
		return
	}
	resp, err := handler(ctx, request)
	if err != nil {
		s.respond(msg, servicefunction.ErrorResponse(req.CallID, err, boundary.Service, req.Subject))
		return
	}
	payload, err := protojson.MarshalOptions{UseProtoNames: false}.Marshal(resp)
	if err != nil {
		s.respond(msg, servicefunction.ErrorResponse(req.CallID, err, boundary.Service, req.Subject))
		return
	}
	s.respond(msg, servicefunction.OKResponse(req.CallID, payload))
}

func (s *Server) respond(msg *natsgo.Msg, resp servicefunction.ResponseEnvelope) {
	if msg == nil {
		return
	}
	data, err := encodeResponse(resp)
	if err != nil {
		s.cfg.Logger.Error("encode secrets response", "error", err)
		return
	}
	if err := msg.Respond(data); err != nil {
		s.cfg.Logger.Error("publish secrets response", "error", err)
	}
}

func decodeRequest(data []byte) (servicefunction.RequestEnvelope, error) {
	var req servicefunction.RequestEnvelope
	if err := json.Unmarshal(data, &req); err != nil {
		return servicefunction.RequestEnvelope{}, err
	}
	return req, nil
}

func encodeResponse(resp servicefunction.ResponseEnvelope) ([]byte, error) {
	if err := resp.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(resp)
}

func normalizeConfig(cfg Config) Config {
	cfg.URL = firstNonEmpty(cfg.URL, DefaultURL)
	cfg.Username = firstNonEmpty(cfg.Username, DefaultUser)
	cfg.Queue = firstNonEmpty(cfg.Queue, DefaultQueue)
	cfg.Name = firstNonEmpty(cfg.Name, "quark-secrets")
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.ReconnectWait <= 0 {
		cfg.ReconnectWait = 250 * time.Millisecond
	}
	if cfg.MaxReconnects == 0 {
		cfg.MaxReconnects = 10
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return cfg
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
