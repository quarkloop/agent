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
	"github.com/quarkloop/runtime/pkg/activity"
	"github.com/quarkloop/runtime/pkg/channel"
	"github.com/quarkloop/runtime/pkg/message"
	"github.com/quarkloop/runtime/pkg/plan"
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
	conn   *natsgo.Conn
	subs   []*natsgo.Subscription
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

func New(cfg Config, poster message.Poster, sessions SessionStore, opts ...Option) *Channel {
	channel := &Channel{
		cfg:      normalizeConfig(cfg),
		poster:   poster,
		sessions: sessions,
	}
	for _, opt := range opts {
		opt(channel)
	}
	return channel
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
	subs, err := c.subscribe(conn, cfg)
	if err != nil {
		cancel()
		conn.Close()
		return err
	}

	c.mu.Lock()
	c.cfg = cfg
	c.conn = conn
	c.subs = subs
	c.ctx = runCtx
	c.cancel = cancel
	c.mu.Unlock()
	c.forwardActivity(runCtx)
	slog.Info("nats channel listening", "subject", "session.*.input", "queue", cfg.Queue)
	return nil
}

func (c *Channel) subscribe(conn *natsgo.Conn, cfg Config) ([]*natsgo.Subscription, error) {
	handlers := map[string]func(*natsgo.Msg){
		"session.*.input":                         c.handleInput,
		clientcontract.SubjectRuntimeInfoGet:      c.handleInfoGet,
		clientcontract.SubjectRuntimeSessionGet:   c.handleSessionGet,
		clientcontract.SubjectRuntimePlanGet:      c.handlePlanGet,
		clientcontract.SubjectRuntimePlanApprove:  c.handlePlanApprove,
		clientcontract.SubjectRuntimePlanReject:   c.handlePlanReject,
		clientcontract.SubjectRuntimeActivityList: c.handleActivityList,
	}
	subs := make([]*natsgo.Subscription, 0, len(handlers))
	for subject, handler := range handlers {
		subject := subject
		handler := handler
		sub, err := conn.QueueSubscribe(subject, cfg.Queue, handler)
		if err != nil {
			for _, sub := range subs {
				_ = sub.Unsubscribe()
			}
			return nil, fmt.Errorf("subscribe %s: %w", subject, err)
		}
		subs = append(subs, sub)
	}
	if err := conn.FlushTimeout(cfg.Timeout); err != nil {
		for _, sub := range subs {
			_ = sub.Unsubscribe()
		}
		return nil, fmt.Errorf("flush runtime subscriptions: %w", err)
	}
	return subs, nil
}

func (c *Channel) Stop(ctx context.Context) error {
	c.mu.Lock()
	conn := c.conn
	subs := append([]*natsgo.Subscription(nil), c.subs...)
	cancel := c.cancel
	c.conn = nil
	c.subs = nil
	c.cancel = nil
	c.ctx = nil
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	for _, sub := range subs {
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

func (c *Channel) handleInfoGet(msg *natsgo.Msg) {
	req, ok := decodeRequest(msg)
	if !ok {
		respond(msg, clientcontract.Error("unknown", string(boundary.InvalidArgument), "invalid request envelope"))
		return
	}
	var payload clientcontract.RuntimeInfoRequest
	if err := req.DecodePayload(&payload); err != nil {
		respond(msg, clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), err.Error()))
		return
	}
	if strings.TrimSpace(payload.SpaceID) == "" {
		respond(msg, clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), "space_id is required"))
		return
	}
	respondPayload(msg, req.RequestID, clientcontract.RuntimeInfoResponse{Sessions: len(c.sessions.List())})
}

func (c *Channel) handleSessionGet(msg *natsgo.Msg) {
	req, ok := decodeRequest(msg)
	if !ok {
		respond(msg, clientcontract.Error("unknown", string(boundary.InvalidArgument), "invalid request envelope"))
		return
	}
	var payload clientcontract.RuntimeSessionRequest
	if err := req.DecodePayload(&payload); err != nil {
		respond(msg, clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), err.Error()))
		return
	}
	if strings.TrimSpace(payload.SpaceID) == "" {
		respond(msg, clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), "space_id is required"))
		return
	}
	if strings.TrimSpace(payload.SessionID) == "" {
		respond(msg, clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), "session_id is required"))
		return
	}
	respondPayload(msg, req.RequestID, clientcontract.RuntimeSessionResponse{
		SessionID: payload.SessionID,
		Found:     c.sessions.Has(payload.SessionID),
	})
}

func (c *Channel) handlePlanGet(msg *natsgo.Msg) {
	req, ok := decodeRequest(msg)
	if !ok {
		respond(msg, clientcontract.Error("unknown", string(boundary.InvalidArgument), "invalid request envelope"))
		return
	}
	if _, err := decodeRuntimePlanRequest(req); err != nil {
		respond(msg, clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), err.Error()))
		return
	}
	respondPayload(msg, req.RequestID, c.planResponse())
}

func (c *Channel) handlePlanApprove(msg *natsgo.Msg) {
	req, ok := decodeRequest(msg)
	if !ok {
		respond(msg, clientcontract.Error("unknown", string(boundary.InvalidArgument), "invalid request envelope"))
		return
	}
	if _, err := decodeRuntimePlanRequest(req); err != nil {
		respond(msg, clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), err.Error()))
		return
	}
	if c.plan != nil {
		c.plan.Resume()
	}
	respondPayload(msg, req.RequestID, c.planResponse())
}

func (c *Channel) handlePlanReject(msg *natsgo.Msg) {
	req, ok := decodeRequest(msg)
	if !ok {
		respond(msg, clientcontract.Error("unknown", string(boundary.InvalidArgument), "invalid request envelope"))
		return
	}
	if _, err := decodeRuntimePlanRequest(req); err != nil {
		respond(msg, clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), err.Error()))
		return
	}
	if c.plan != nil {
		c.plan.Pause()
	}
	respondPayload(msg, req.RequestID, c.planResponse())
}

func (c *Channel) handleActivityList(msg *natsgo.Msg) {
	req, ok := decodeRequest(msg)
	if !ok {
		respond(msg, clientcontract.Error("unknown", string(boundary.InvalidArgument), "invalid request envelope"))
		return
	}
	var payload clientcontract.RuntimeActivityListRequest
	if err := req.DecodePayload(&payload); err != nil {
		respond(msg, clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), err.Error()))
		return
	}
	if strings.TrimSpace(payload.SpaceID) == "" {
		respond(msg, clientcontract.Error(req.RequestID, string(boundary.InvalidArgument), "space_id is required"))
		return
	}
	var records []clientcontract.RuntimeActivityRecord
	if c.activity != nil {
		records = mapActivityRecords(c.activity.List(payload.Limit))
	}
	respondPayload(msg, req.RequestID, clientcontract.RuntimeActivityListResponse{Records: records})
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

func decodeRuntimePlanRequest(req clientcontract.RequestEnvelope) (clientcontract.RuntimePlanRequest, error) {
	var payload clientcontract.RuntimePlanRequest
	if err := req.DecodePayload(&payload); err != nil {
		return clientcontract.RuntimePlanRequest{}, err
	}
	if strings.TrimSpace(payload.SpaceID) == "" {
		return clientcontract.RuntimePlanRequest{}, errors.New("space_id is required")
	}
	return payload, nil
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
		SpaceID:   payload.SpaceID,
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

func (c *Channel) forwardActivity(ctx context.Context) {
	if c.activity == nil {
		return
	}
	records := c.activity.Subscribe()
	go func() {
		defer c.activity.Unsubscribe(records)
		for {
			select {
			case record, ok := <-records:
				if !ok {
					return
				}
				if err := c.publishActivity(record); err != nil {
					slog.Error("publish runtime activity event", "id", record.ID, "error", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (c *Channel) publishActivity(record activity.Record) error {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn == nil {
		return errors.New("nats runtime channel is not connected")
	}
	payload := mapActivityRecord(record)
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal activity record: %w", err)
	}
	if err := conn.Publish(clientcontract.SubjectRuntimeActivityFeed, data); err != nil {
		return fmt.Errorf("publish %s: %w", clientcontract.SubjectRuntimeActivityFeed, err)
	}
	return nil
}

func (c *Channel) planResponse() clientcontract.RuntimePlanResponse {
	now := time.Now().UTC()
	if c.plan == nil {
		return clientcontract.RuntimePlanResponse{
			Goal:      "No active plan",
			Status:    "idle",
			Complete:  true,
			Summary:   "No active work.",
			CreatedAt: now,
			UpdatedAt: now,
		}
	}
	steps := c.plan.GetSteps()
	resp := clientcontract.RuntimePlanResponse{
		Goal:      "Current runtime plan",
		Status:    mapPlanStatus(c.plan.GetStatus()),
		Steps:     make([]clientcontract.RuntimePlanStep, 0, len(steps)),
		Complete:  c.plan.GetStatus() == "completed" || c.plan.GetStatus() == "idle",
		Summary:   c.plan.GetSummary(),
		CreatedAt: now,
		UpdatedAt: now,
	}
	for index, step := range steps {
		resp.Steps = append(resp.Steps, clientcontract.RuntimePlanStep{
			ID:          fmt.Sprintf("step-%d", index+1),
			Agent:       "main",
			Description: step.Description(),
			Status:      mapStepStatus(step.Status()),
			Result:      step.Result(),
		})
	}
	return resp
}

func mapPlanStatus(status string) string {
	switch status {
	case "active":
		return "executing"
	case "paused":
		return "draft"
	case "completed":
		return "succeeded"
	case "failed":
		return "failed"
	default:
		return status
	}
}

func mapStepStatus(status string) string {
	switch status {
	case "active":
		return "running"
	case "completed":
		return "complete"
	default:
		return status
	}
}

func mapActivityRecords(records []activity.Record) []clientcontract.RuntimeActivityRecord {
	out := make([]clientcontract.RuntimeActivityRecord, 0, len(records))
	for _, record := range records {
		out = append(out, mapActivityRecord(record))
	}
	return out
}

func mapActivityRecord(record activity.Record) clientcontract.RuntimeActivityRecord {
	return clientcontract.RuntimeActivityRecord{
		ID:        record.ID,
		SessionID: record.SessionID,
		Type:      record.Type,
		Timestamp: record.Timestamp,
		Data:      append(json.RawMessage(nil), record.Data...),
	}
}

func respondPayload(msg *natsgo.Msg, requestID string, payload any) {
	resp, err := clientcontract.OK(requestID, payload)
	if err != nil {
		respond(msg, clientcontract.Error(requestID, string(boundary.Internal), err.Error()))
		return
	}
	respond(msg, resp)
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
