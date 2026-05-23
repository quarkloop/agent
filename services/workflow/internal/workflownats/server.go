package workflownats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/boundary"
	workflowv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/workflow/v1"
	"github.com/quarkloop/pkg/serviceapi/servicefunction"
	"github.com/quarkloop/services/workflow/internal/workflowsvc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	DefaultURL           = "nats://127.0.0.1:4222"
	DefaultUser          = "quark-control"
	DefaultQueue         = "q.workflow.v1"
	DefaultTimeout       = 30 * time.Second
	serviceName          = "workflow"
	serviceVersion       = "v1"
	functionStart        = "start"
	functionSignal       = "signal"
	functionQuery        = "query"
	functionCancel       = "cancel"
	functionDescribe     = "describe"
	functionList         = "list"
	functionStreamEvents = "stream_events"
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
	cfg       Config
	workflows *workflowsvc.Server
	conn      *natsgo.Conn
	subs      []*natsgo.Subscription
}

func New(cfg Config, workflows *workflowsvc.Server) *Server {
	return &Server{cfg: normalizeConfig(cfg), workflows: workflows}
}

func (s *Server) Start(ctx context.Context) error {
	if s == nil || s.workflows == nil {
		return fmt.Errorf("workflow server is required")
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
		functionStart,
		functionSignal,
		functionQuery,
		functionCancel,
		functionDescribe,
		functionList,
		functionStreamEvents,
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
		return fmt.Errorf("flush workflow subscriptions: %w", err)
	}
	s.cfg.Logger.Info("workflow nats endpoints ready", "url", s.cfg.URL, "queue", s.cfg.Queue)
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
		s.respond(msg, servicefunction.ErrorResponse("", err, boundary.Service, "workflow.decode_request"))
		return
	}
	if err := req.Validate(); err != nil {
		s.respond(msg, servicefunction.ErrorResponse(req.CallID, err, boundary.Service, req.Subject))
		return
	}
	switch req.Function {
	case functionStart:
		s.handleUnary(ctx, msg, req, &workflowv1.StartWorkflowRequest{}, func(ctx context.Context, request proto.Message) (proto.Message, error) {
			return s.workflows.Start(ctx, request.(*workflowv1.StartWorkflowRequest))
		})
	case functionSignal:
		s.handleUnary(ctx, msg, req, &workflowv1.SignalWorkflowRequest{}, func(ctx context.Context, request proto.Message) (proto.Message, error) {
			return s.workflows.Signal(ctx, request.(*workflowv1.SignalWorkflowRequest))
		})
	case functionQuery:
		s.handleUnary(ctx, msg, req, &workflowv1.QueryWorkflowRequest{}, func(ctx context.Context, request proto.Message) (proto.Message, error) {
			return s.workflows.Query(ctx, request.(*workflowv1.QueryWorkflowRequest))
		})
	case functionCancel:
		s.handleUnary(ctx, msg, req, &workflowv1.CancelWorkflowRequest{}, func(ctx context.Context, request proto.Message) (proto.Message, error) {
			return s.workflows.Cancel(ctx, request.(*workflowv1.CancelWorkflowRequest))
		})
	case functionDescribe:
		s.handleUnary(ctx, msg, req, &workflowv1.DescribeWorkflowRequest{}, func(ctx context.Context, request proto.Message) (proto.Message, error) {
			return s.workflows.Describe(ctx, request.(*workflowv1.DescribeWorkflowRequest))
		})
	case functionList:
		s.handleUnary(ctx, msg, req, &workflowv1.ListWorkflowsRequest{}, func(ctx context.Context, request proto.Message) (proto.Message, error) {
			return s.workflows.List(ctx, request.(*workflowv1.ListWorkflowsRequest))
		})
	case functionStreamEvents:
		s.handleStreamEvents(ctx, msg, req)
	default:
		s.respond(msg, servicefunction.ErrorResponse(req.CallID, fmt.Errorf("unknown workflow function %q", req.Function), boundary.Service, req.Subject))
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

func (s *Server) handleStreamEvents(ctx context.Context, msg *natsgo.Msg, req servicefunction.RequestEnvelope) {
	if msg.Reply == "" {
		return
	}
	var request workflowv1.StreamWorkflowEventsRequest
	if err := protojson.Unmarshal(req.Payload, &request); err != nil {
		s.publish(msg.Reply, servicefunction.ErrorResponse(req.CallID, err, boundary.Service, req.Subject))
		return
	}
	events, err := s.workflows.EngineEvents(ctx, &request)
	if err != nil {
		s.publish(msg.Reply, servicefunction.ErrorResponse(req.CallID, err, boundary.Service, req.Subject))
		return
	}
	for event := range events {
		payload, err := protojson.MarshalOptions{UseProtoNames: false}.Marshal(event)
		if err != nil {
			s.publish(msg.Reply, servicefunction.ErrorResponse(req.CallID, err, boundary.Service, req.Subject))
			return
		}
		s.publish(msg.Reply, servicefunction.OKResponse(req.CallID, payload))
	}
}

func (s *Server) respond(msg *natsgo.Msg, resp servicefunction.ResponseEnvelope) {
	if msg == nil {
		return
	}
	data, err := encodeResponse(resp)
	if err != nil {
		s.cfg.Logger.Error("encode workflow response", "error", err)
		return
	}
	if err := msg.Respond(data); err != nil {
		s.cfg.Logger.Error("publish workflow response", "error", err)
	}
}

func (s *Server) publish(reply string, resp servicefunction.ResponseEnvelope) {
	if s == nil || s.conn == nil || strings.TrimSpace(reply) == "" {
		return
	}
	data, err := encodeResponse(resp)
	if err != nil {
		s.cfg.Logger.Error("encode workflow stream response", "error", err)
		return
	}
	if err := s.conn.Publish(reply, data); err != nil {
		s.cfg.Logger.Error("publish workflow stream response", "error", err)
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
	cfg.Name = firstNonEmpty(cfg.Name, "quark-workflow")
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
