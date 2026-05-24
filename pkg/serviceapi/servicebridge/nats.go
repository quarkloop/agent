package servicebridge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/boundary"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/pkg/serviceapi/observability"
	"github.com/quarkloop/pkg/serviceapi/servicefunction"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

const (
	DefaultURL           = "nats://127.0.0.1:4222"
	DefaultUser          = "quark-control"
	DefaultTimeout       = 30 * time.Second
	DefaultReconnectWait = 250 * time.Millisecond
	DefaultMaxReconnects = 10
)

type NATSConfig struct {
	URL             string
	Username        string
	Password        string
	Queue           string
	Name            string
	AuditPrefix     string
	TelemetryPrefix string
	Timeout         time.Duration
	ReconnectWait   time.Duration
	MaxReconnects   int
	Logger          *slog.Logger
}

type Binding struct {
	Descriptor *servicev1.ServiceDescriptor
	Services   []RPCService
}

type RPCService struct {
	Service        string
	Implementation any
}

type NATSService struct {
	cfg  NATSConfig
	conn *natsgo.Conn
	subs []*natsgo.Subscription
}

func NewNATSService(cfg NATSConfig) *NATSService {
	return &NATSService{cfg: normalizeNATSConfig(cfg)}
}

func RunNATSService(ctx context.Context, cfg NATSConfig, bindings ...Binding) error {
	if strings.TrimSpace(cfg.URL) == "" {
		return fmt.Errorf("nats url is required")
	}
	service := NewNATSService(cfg)
	if err := service.Start(ctx, bindings...); err != nil {
		return err
	}
	defer service.Close()
	<-ctx.Done()
	return nil
}

func (s *NATSService) Start(ctx context.Context, bindings ...Binding) error {
	if s == nil {
		return fmt.Errorf("nats service bridge is nil")
	}
	cfg := normalizeNATSConfig(s.cfg)
	conn, err := natsgo.Connect(
		cfg.URL,
		natsgo.UserInfo(cfg.Username, cfg.Password),
		natsgo.Name(cfg.Name),
		natsgo.Timeout(cfg.Timeout),
		natsgo.ReconnectWait(cfg.ReconnectWait),
		natsgo.MaxReconnects(cfg.MaxReconnects),
		natsgo.RetryOnFailedConnect(true),
	)
	if err != nil {
		return fmt.Errorf("connect nats service bridge: %w", err)
	}
	s.cfg = cfg
	s.conn = conn
	for _, binding := range bindings {
		if err := s.registerBinding(binding); err != nil {
			s.Close()
			return err
		}
	}
	if err := conn.FlushTimeout(cfg.Timeout); err != nil {
		s.Close()
		return fmt.Errorf("flush nats service bridge subscriptions: %w", err)
	}
	cfg.Logger.Info("nats service bridge ready", "url", cfg.URL, "queue", cfg.Queue, "subscriptions", len(s.subs))
	go func() {
		<-ctx.Done()
		s.Close()
	}()
	return nil
}

func (s *NATSService) Close() {
	if s == nil {
		return
	}
	for _, sub := range s.subs {
		_ = sub.Unsubscribe()
	}
	s.subs = nil
	if s.conn != nil {
		_ = s.conn.Drain()
		s.conn.Close()
		s.conn = nil
	}
}

func (s *NATSService) registerBinding(binding Binding) error {
	if binding.Descriptor == nil {
		return fmt.Errorf("service descriptor is required")
	}
	descriptor := servicekit.CloneDescriptor(binding.Descriptor)
	methods := serviceMethods(binding.Services)
	for _, rpc := range descriptor.GetRpcs() {
		if rpc.GetStreaming() {
			continue
		}
		method, err := methodBindingFor(methods, rpc)
		if err != nil {
			return err
		}
		subject, serviceName, functionName, err := subjectForRPC(descriptor, rpc)
		if err != nil {
			return err
		}
		endpoint := endpoint{
			subject:        subject,
			serviceName:    serviceName,
			functionName:   functionName,
			rpc:            servicekit.CloneDescriptor(&servicev1.ServiceDescriptor{Name: descriptor.GetName(), Rpcs: []*servicev1.RpcDescriptor{rpc}}).GetRpcs()[0],
			method:         method,
			implementation: method.Implementation,
		}
		sub, err := s.conn.QueueSubscribe(subject, s.cfg.Queue, s.handle(endpoint))
		if err != nil {
			return fmt.Errorf("subscribe %s: %w", subject, err)
		}
		s.subs = append(s.subs, sub)
	}
	return nil
}

type endpoint struct {
	subject        string
	serviceName    string
	functionName   string
	rpc            *servicev1.RpcDescriptor
	method         methodBinding
	implementation any
}

type methodBinding struct {
	Service        string
	Method         string
	Request        protoreflect.FullName
	Implementation any
}

func (s *NATSService) handle(ep endpoint) func(*natsgo.Msg) {
	return func(msg *natsgo.Msg) {
		started := time.Now()
		timeout := serviceTimeout(ep.rpc, s.cfg.Timeout)
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		req, err := decodeRequest(msg.Data)
		if err != nil {
			s.respondAndRecord(msg, servicefunction.RequestEnvelope{}, ep, servicefunction.ErrorResponse("", err, boundary.Service, ep.subject), started)
			return
		}
		if err := req.Validate(); err != nil {
			s.respondAndRecord(msg, req, ep, servicefunction.ErrorResponse(req.CallID, err, boundary.Service, ep.subject), started)
			return
		}
		if req.Subject != ep.subject || req.Service != ep.serviceName || req.Function != ep.functionName {
			err := fmt.Errorf("request targets %s/%s on %s, endpoint is %s/%s on %s", req.Service, req.Function, req.Subject, ep.serviceName, ep.functionName, ep.subject)
			s.respondAndRecord(msg, req, ep, servicefunction.ErrorResponse(req.CallID, err, boundary.Service, ep.subject), started)
			return
		}
		protoReq, err := newRequestMessage(ep.method.Request, req.Payload)
		if err != nil {
			s.respondAndRecord(msg, req, ep, servicefunction.ErrorResponse(req.CallID, err, boundary.Service, ep.subject), started)
			return
		}
		resp, err := callUnaryMethod(ctx, ep.implementation, ep.method.Method, protoReq)
		if err != nil {
			s.respondAndRecord(msg, req, ep, servicefunction.ErrorResponse(req.CallID, err, boundary.Service, ep.subject), started)
			return
		}
		protoResp, ok := resp.(proto.Message)
		if !ok {
			s.respondAndRecord(msg, req, ep, servicefunction.ErrorResponse(req.CallID, fmt.Errorf("response is not protobuf message"), boundary.Service, ep.subject), started)
			return
		}
		payload, err := protojson.MarshalOptions{UseProtoNames: false}.Marshal(protoResp)
		if err != nil {
			s.respondAndRecord(msg, req, ep, servicefunction.ErrorResponse(req.CallID, err, boundary.Service, ep.subject), started)
			return
		}
		s.respondAndRecord(msg, req, ep, servicefunction.OKResponse(req.CallID, payload), started)
	}
}

func (s *NATSService) respond(msg *natsgo.Msg, resp servicefunction.ResponseEnvelope) {
	if msg == nil {
		return
	}
	data, err := encodeResponse(resp)
	if err != nil {
		s.cfg.Logger.Error("encode nats service response", "error", err)
		return
	}
	if err := msg.Respond(data); err != nil {
		s.cfg.Logger.Error("publish nats service response", "error", err)
	}
}

func (s *NATSService) respondAndRecord(msg *natsgo.Msg, req servicefunction.RequestEnvelope, ep endpoint, resp servicefunction.ResponseEnvelope, started time.Time) {
	s.respond(msg, resp)
	s.publishServiceCallEvents(req, ep, resp, time.Since(started))
}

func (s *NATSService) publishServiceCallEvents(req servicefunction.RequestEnvelope, ep endpoint, resp servicefunction.ResponseEnvelope, duration time.Duration) {
	if s == nil || s.conn == nil {
		return
	}
	if strings.TrimSpace(s.cfg.AuditPrefix) == "" && strings.TrimSpace(s.cfg.TelemetryPrefix) == "" {
		return
	}
	event := observability.ServiceCallEvent{
		CallID:         req.CallID,
		SpaceID:        req.SpaceID,
		SessionID:      req.SessionID,
		RunID:          req.RunID,
		WorkflowID:     req.WorkflowID,
		AgentID:        req.AgentID,
		Service:        ep.serviceName,
		Function:       ep.functionName,
		Subject:        ep.subject,
		Status:         string(resp.Status),
		DurationMillis: duration.Milliseconds(),
		TraceParent:    req.TraceParent,
		TraceState:     req.TraceState,
	}
	if resp.Error != nil {
		event.ErrorCategory = string(resp.Error.Category)
	}
	data, err := observability.MarshalServiceCallEvent(event)
	if err != nil {
		s.cfg.Logger.Error("encode service call event", "subject", ep.subject, "error", err)
		return
	}
	for _, prefix := range []string{s.cfg.AuditPrefix, s.cfg.TelemetryPrefix} {
		subject := observability.ServiceCallSubject(prefix, req.SpaceID)
		if subject == "" {
			continue
		}
		if err := s.conn.Publish(subject, data); err != nil {
			s.cfg.Logger.Error("publish service call event", "subject", subject, "error", err)
		}
	}
}

func serviceMethods(services []RPCService) map[string]methodBinding {
	out := make(map[string]methodBinding)
	for _, service := range services {
		if strings.TrimSpace(service.Service) == "" || service.Implementation == nil {
			continue
		}
		out[serviceMethodKey(service.Service, "*")] = methodBinding{
			Service:        strings.TrimSpace(service.Service),
			Implementation: service.Implementation,
		}
	}
	return out
}

func methodBindingFor(methods map[string]methodBinding, rpc *servicev1.RpcDescriptor) (methodBinding, error) {
	if rpc == nil {
		return methodBinding{}, fmt.Errorf("rpc descriptor is required")
	}
	method, ok := methods[serviceMethodKey(rpc.GetService(), rpc.GetMethod())]
	if !ok {
		method, ok = methods[serviceMethodKey(rpc.GetService(), "*")]
	}
	if !ok || method.Implementation == nil {
		return methodBinding{}, fmt.Errorf("missing implementation for %s/%s", rpc.GetService(), rpc.GetMethod())
	}
	method.Service = strings.TrimSpace(rpc.GetService())
	method.Method = strings.TrimSpace(rpc.GetMethod())
	method.Request = protoreflect.FullName(strings.TrimSpace(rpc.GetRequest()))
	if method.Method == "" {
		return methodBinding{}, fmt.Errorf("rpc method is required for %s", rpc.GetService())
	}
	if method.Request == "" {
		return methodBinding{}, fmt.Errorf("rpc request type is required for %s/%s", rpc.GetService(), rpc.GetMethod())
	}
	return method, nil
}

func newRequestMessage(name protoreflect.FullName, payload json.RawMessage) (proto.Message, error) {
	messageType, err := protoregistry.GlobalTypes.FindMessageByName(name)
	if err != nil {
		return nil, fmt.Errorf("resolve protobuf request %s: %w", name, err)
	}
	message := messageType.New().Interface()
	if len(payload) == 0 {
		payload = json.RawMessage("{}")
	}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(payload, message); err != nil {
		return nil, fmt.Errorf("decode protobuf request %s: %w", name, err)
	}
	return message, nil
}

func callUnaryMethod(ctx context.Context, implementation any, methodName string, req proto.Message) (proto.Message, error) {
	if implementation == nil {
		return nil, fmt.Errorf("service implementation is required")
	}
	method := reflect.ValueOf(implementation).MethodByName(methodName)
	if !method.IsValid() {
		return nil, fmt.Errorf("service implementation does not expose %s", methodName)
	}
	methodType := method.Type()
	if methodType.NumIn() != 2 || methodType.NumOut() != 2 {
		return nil, fmt.Errorf("service method %s must have signature func(context.Context, proto.Message) (proto.Message, error)", methodName)
	}
	if !reflect.TypeOf(ctx).AssignableTo(methodType.In(0)) {
		return nil, fmt.Errorf("service method %s first argument must accept context.Context", methodName)
	}
	reqValue := reflect.ValueOf(req)
	if !reqValue.Type().AssignableTo(methodType.In(1)) {
		return nil, fmt.Errorf("service method %s request type mismatch: got %s want %s", methodName, reqValue.Type(), methodType.In(1))
	}
	results := method.Call([]reflect.Value{reflect.ValueOf(ctx), reqValue})
	if errValue := results[1]; !errValue.IsNil() {
		err, _ := errValue.Interface().(error)
		return nil, err
	}
	resp, ok := results[0].Interface().(proto.Message)
	if !ok {
		return nil, fmt.Errorf("service method %s response is not protobuf message", methodName)
	}
	return resp, nil
}

func subjectForRPC(desc *servicev1.ServiceDescriptor, rpc *servicev1.RpcDescriptor) (string, string, string, error) {
	owner := strings.TrimSpace(rpc.GetOwner())
	if owner == "" && desc != nil {
		owner = desc.GetName()
	}
	if owner == "" {
		return "", "", "", fmt.Errorf("owner is required for %s/%s", rpc.GetService(), rpc.GetMethod())
	}
	function := strings.TrimSpace(rpc.GetFunctionName())
	if function == "" {
		function = rpc.GetMethod()
	}
	subject, err := servicefunction.SubjectFromOwnerAndFunctionName(owner, function)
	if err != nil {
		return "", "", "", err
	}
	functionToken, err := servicefunction.FunctionTokenFromOwnerAndFunctionName(owner, function)
	if err != nil {
		return "", "", "", err
	}
	serviceToken := strings.Split(subject, ".")[1]
	return subject, serviceToken, functionToken, nil
}

func serviceMethodKey(service, method string) string {
	return strings.TrimSpace(service) + "/" + strings.TrimSpace(method)
}

func serviceTimeout(rpc *servicev1.RpcDescriptor, fallback time.Duration) time.Duration {
	if rpc != nil && rpc.GetTimeoutMillis() > 0 {
		return time.Duration(rpc.GetTimeoutMillis()) * time.Millisecond
	}
	if fallback > 0 {
		return fallback
	}
	return DefaultTimeout
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

func normalizeNATSConfig(cfg NATSConfig) NATSConfig {
	cfg.URL = firstNonEmpty(cfg.URL, DefaultURL)
	cfg.Username = firstNonEmpty(cfg.Username, DefaultUser)
	cfg.Queue = firstNonEmpty(cfg.Queue, "q.service.v1")
	cfg.Name = firstNonEmpty(cfg.Name, "quark-service")
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.ReconnectWait <= 0 {
		cfg.ReconnectWait = DefaultReconnectWait
	}
	if cfg.MaxReconnects == 0 {
		cfg.MaxReconnects = DefaultMaxReconnects
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
