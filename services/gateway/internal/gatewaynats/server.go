package gatewaynats

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/boundary"
	gatewayv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/gateway/v1"
	"github.com/quarkloop/pkg/serviceapi/servicefunction"
	"github.com/quarkloop/services/gateway/internal/gatewaysvc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	DefaultURL             = "nats://127.0.0.1:4222"
	DefaultUser            = "quark-control"
	DefaultQueue           = "q.gateway.v1"
	DefaultTimeout         = 30 * time.Second
	serviceName            = "gateway"
	serviceVersion         = "v1"
	functionGenerate       = "generate"
	functionStreamGenerate = "stream_generate"
	functionEmbed          = "embed"
	functionRerank         = "rerank"
	functionCountTokens    = "count_tokens"
	functionListModels     = "list_models"
	functionProviderHealth = "provider_health"
	functionUsageSummary   = "usage_summary"
	functionReloadConfig   = "reload_config"
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
	gateway *gatewaysvc.Server
	conn    *natsgo.Conn
	subs    []*natsgo.Subscription
}

func New(cfg Config, gateway *gatewaysvc.Server) *Server {
	return &Server{cfg: normalizeConfig(cfg), gateway: gateway}
}

func (s *Server) Start(ctx context.Context) error {
	if s == nil || s.gateway == nil {
		return fmt.Errorf("gateway server is required")
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
		functionGenerate,
		functionStreamGenerate,
		functionEmbed,
		functionRerank,
		functionCountTokens,
		functionListModels,
		functionProviderHealth,
		functionUsageSummary,
		functionReloadConfig,
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
		return fmt.Errorf("flush gateway subscriptions: %w", err)
	}
	s.cfg.Logger.Info("gateway nats endpoints ready", "url", s.cfg.URL, "queue", s.cfg.Queue)
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
		s.respond(msg, servicefunction.ErrorResponse("", err, boundary.Service, "gateway.decode_request"))
		return
	}
	if err := req.Validate(); err != nil {
		s.respond(msg, servicefunction.ErrorResponse(req.CallID, err, boundary.Service, req.Subject))
		return
	}
	switch req.Function {
	case functionGenerate:
		s.handleUnary(ctx, msg, req, &gatewayv1.GenerateRequest{}, func(ctx context.Context, request proto.Message) (proto.Message, *gatewayv1.ModelUsage, error) {
			resp, err := s.gateway.Generate(ctx, request.(*gatewayv1.GenerateRequest))
			return resp, resp.GetUsage(), err
		})
	case functionEmbed:
		s.handleUnary(ctx, msg, req, &gatewayv1.EmbedRequest{}, func(ctx context.Context, request proto.Message) (proto.Message, *gatewayv1.ModelUsage, error) {
			resp, err := s.gateway.Embed(ctx, request.(*gatewayv1.EmbedRequest))
			return resp, resp.GetUsage(), err
		})
	case functionRerank:
		s.handleUnary(ctx, msg, req, &gatewayv1.RerankRequest{}, func(ctx context.Context, request proto.Message) (proto.Message, *gatewayv1.ModelUsage, error) {
			resp, err := s.gateway.Rerank(ctx, request.(*gatewayv1.RerankRequest))
			return resp, resp.GetUsage(), err
		})
	case functionCountTokens:
		s.handleUnary(ctx, msg, req, &gatewayv1.CountTokensRequest{}, func(ctx context.Context, request proto.Message) (proto.Message, *gatewayv1.ModelUsage, error) {
			resp, err := s.gateway.CountTokens(ctx, request.(*gatewayv1.CountTokensRequest))
			return resp, resp.GetUsage(), err
		})
	case functionListModels:
		s.handleUnary(ctx, msg, req, &gatewayv1.ListModelsRequest{}, func(ctx context.Context, request proto.Message) (proto.Message, *gatewayv1.ModelUsage, error) {
			resp, err := s.gateway.ListModels(ctx, request.(*gatewayv1.ListModelsRequest))
			return resp, nil, err
		})
	case functionProviderHealth:
		s.handleUnary(ctx, msg, req, &gatewayv1.ProviderHealthRequest{}, func(ctx context.Context, request proto.Message) (proto.Message, *gatewayv1.ModelUsage, error) {
			resp, err := s.gateway.ProviderHealth(ctx, request.(*gatewayv1.ProviderHealthRequest))
			return resp, nil, err
		})
	case functionStreamGenerate:
		s.handleStreamGenerate(ctx, msg, req)
	case functionUsageSummary:
		s.handleUnary(ctx, msg, req, &gatewayv1.UsageSummaryRequest{}, func(ctx context.Context, request proto.Message) (proto.Message, *gatewayv1.ModelUsage, error) {
			resp, err := s.gateway.UsageSummary(ctx, request.(*gatewayv1.UsageSummaryRequest))
			return resp, nil, err
		})
	case functionReloadConfig:
		s.handleUnary(ctx, msg, req, &gatewayv1.ReloadConfigRequest{}, func(ctx context.Context, request proto.Message) (proto.Message, *gatewayv1.ModelUsage, error) {
			resp, err := s.gateway.ReloadConfig(ctx, request.(*gatewayv1.ReloadConfigRequest))
			return resp, nil, err
		})
	default:
		s.respond(msg, servicefunction.ErrorResponse(req.CallID, fmt.Errorf("unknown gateway function %q", req.Function), boundary.Service, req.Subject))
	}
}

type unaryHandler func(context.Context, proto.Message) (proto.Message, *gatewayv1.ModelUsage, error)

func (s *Server) handleUnary(ctx context.Context, msg *natsgo.Msg, req servicefunction.RequestEnvelope, request proto.Message, handler unaryHandler) {
	if err := protojson.Unmarshal(req.Payload, request); err != nil {
		s.respond(msg, servicefunction.ErrorResponse(req.CallID, err, boundary.Service, req.Subject))
		return
	}
	resp, usage, err := handler(ctx, request)
	if err != nil {
		s.respond(msg, servicefunction.ErrorResponse(req.CallID, err, boundary.Service, req.Subject))
		return
	}
	payload, err := protojson.MarshalOptions{UseProtoNames: false}.Marshal(resp)
	if err != nil {
		s.respond(msg, servicefunction.ErrorResponse(req.CallID, err, boundary.Service, req.Subject))
		return
	}
	envelope := servicefunction.OKResponse(req.CallID, payload)
	envelope.Usage = usageEnvelope(usage)
	s.respond(msg, envelope)
}

func (s *Server) handleStreamGenerate(ctx context.Context, msg *natsgo.Msg, req servicefunction.RequestEnvelope) {
	if msg.Reply == "" {
		return
	}
	started := time.Now()
	var request gatewayv1.StreamGenerateRequest
	if err := protojson.Unmarshal(req.Payload, &request); err != nil {
		s.cfg.Logger.Error("gateway stream request decode failed", "call_id", req.CallID, "error", err)
		s.publish(msg.Reply, servicefunction.ErrorResponse(req.CallID, err, boundary.Service, req.Subject))
		return
	}
	logAttrs := []any{
		"call_id", req.CallID,
		"session_id", req.SessionID,
		"run_id", req.RunID,
		"provider", request.GetProvider(),
		"model", request.GetModel(),
	}
	s.cfg.Logger.Info("gateway stream request started", logAttrs...)
	events, err := s.gateway.StreamGenerateEvents(ctx, &request)
	if err != nil {
		s.cfg.Logger.Error("gateway stream request failed", append(logAttrs, "duration", time.Since(started), "error", err)...)
		s.publish(msg.Reply, servicefunction.ErrorResponse(req.CallID, err, boundary.Service, req.Subject))
		s.flushStream(logAttrs)
		return
	}
	count := 0
	for event := range events {
		if event.Err != nil {
			s.cfg.Logger.Error("gateway stream event failed", append(logAttrs, "duration", time.Since(started), "events", count, "error", event.Err)...)
			s.publish(msg.Reply, servicefunction.ErrorResponse(req.CallID, event.Err, boundary.Service, req.Subject))
			s.flushStream(logAttrs)
			return
		}
		payload, err := protojson.MarshalOptions{UseProtoNames: false}.Marshal(event.Response)
		if err != nil {
			s.cfg.Logger.Error("gateway stream marshal failed", append(logAttrs, "duration", time.Since(started), "events", count, "error", err)...)
			s.publish(msg.Reply, servicefunction.ErrorResponse(req.CallID, err, boundary.Service, req.Subject))
			s.flushStream(logAttrs)
			return
		}
		envelope := servicefunction.OKResponse(req.CallID, payload)
		envelope.Usage = usageEnvelope(event.Response.GetUsage())
		s.publish(msg.Reply, envelope)
		count++
		if event.Response.GetDone() {
			s.flushStream(logAttrs)
			s.cfg.Logger.Info("gateway stream request completed", append(logAttrs, "duration", time.Since(started), "events", count)...)
			return
		}
	}
	s.flushStream(logAttrs)
	s.cfg.Logger.Warn("gateway stream closed without done event", append(logAttrs, "duration", time.Since(started), "events", count)...)
}

func (s *Server) flushStream(logAttrs []any) {
	if s == nil || s.conn == nil {
		return
	}
	if err := s.conn.FlushTimeout(s.cfg.Timeout); err != nil {
		s.cfg.Logger.Error("gateway stream flush failed", append(logAttrs, "error", err)...)
	}
}

func (s *Server) respond(msg *natsgo.Msg, resp servicefunction.ResponseEnvelope) {
	if msg == nil {
		return
	}
	data, err := encodeResponse(resp)
	if err != nil {
		s.cfg.Logger.Error("encode gateway response", "error", err)
		return
	}
	if err := msg.Respond(data); err != nil {
		s.cfg.Logger.Error("publish gateway response", "error", err)
	}
}

func (s *Server) publish(reply string, resp servicefunction.ResponseEnvelope) {
	if s == nil || s.conn == nil || strings.TrimSpace(reply) == "" {
		return
	}
	data, err := encodeResponse(resp)
	if err != nil {
		s.cfg.Logger.Error("encode gateway stream response", "error", err)
		return
	}
	if err := s.conn.Publish(reply, data); err != nil {
		s.cfg.Logger.Error("publish gateway stream response", "error", err)
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

func usageEnvelope(usage *gatewayv1.ModelUsage) *servicefunction.Usage {
	if usage == nil || usage.GetProvider() == "" {
		return nil
	}
	additional, _ := json.Marshal(map[string]any{
		"latency_millis": usage.GetLatencyMillis(),
		"cost_estimate":  usage.GetCostEstimate(),
		"fallback_chain": usage.GetFallbackChain(),
		"finish_reason":  usage.GetFinishReason(),
	})
	return &servicefunction.Usage{
		Provider:       usage.GetProvider(),
		Model:          usage.GetModel(),
		RequestID:      usage.GetRequestId(),
		InputTokens:    usage.GetInputTokens(),
		OutputTokens:   usage.GetOutputTokens(),
		TotalTokens:    usage.GetInputTokens() + usage.GetOutputTokens() + usage.GetEmbeddingTokens(),
		AdditionalJSON: additional,
	}
}

func normalizeConfig(cfg Config) Config {
	cfg.URL = firstNonEmpty(cfg.URL, DefaultURL)
	cfg.Username = firstNonEmpty(cfg.Username, DefaultUser)
	cfg.Queue = firstNonEmpty(cfg.Queue, DefaultQueue)
	cfg.Name = firstNonEmpty(cfg.Name, "quark-gateway")
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
