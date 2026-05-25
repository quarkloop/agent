package observability

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/serviceapi/servicefunction"
)

const (
	DefaultAuditRetention   = 90 * 24 * time.Hour
	DefaultSnapshotMaxBytes = 4096
	SnapshotPolicyNone      = "none"
	SnapshotPolicyRedacted  = "redacted"
)

type AuditPolicy struct {
	Retention        time.Duration
	SnapshotPolicy   string
	MaxSnapshotBytes int
}

type RecorderConfig struct {
	AuditPrefix     string
	TelemetryPrefix string
	Policy          AuditPolicy
}

type Endpoint struct {
	Service  string
	Function string
	Subject  string
}

type Recorder struct {
	conn *natsgo.Conn
	js   natsgo.JetStreamContext
	cfg  RecorderConfig
	now  func() time.Time
}

func DefaultAuditPolicy() AuditPolicy {
	return AuditPolicy{
		Retention:        DefaultAuditRetention,
		SnapshotPolicy:   SnapshotPolicyNone,
		MaxSnapshotBytes: DefaultSnapshotMaxBytes,
	}
}

func NormalizeAuditPolicy(policy AuditPolicy) (AuditPolicy, error) {
	if policy.Retention <= 0 {
		policy.Retention = DefaultAuditRetention
	}
	policy.SnapshotPolicy = strings.ToLower(strings.TrimSpace(policy.SnapshotPolicy))
	if policy.SnapshotPolicy == "" {
		policy.SnapshotPolicy = SnapshotPolicyNone
	}
	if policy.SnapshotPolicy != SnapshotPolicyNone && policy.SnapshotPolicy != SnapshotPolicyRedacted {
		return AuditPolicy{}, fmt.Errorf("unsupported audit snapshot policy %q", policy.SnapshotPolicy)
	}
	if policy.MaxSnapshotBytes <= 0 {
		policy.MaxSnapshotBytes = DefaultSnapshotMaxBytes
	}
	return policy, nil
}

func NewRecorder(conn *natsgo.Conn, cfg RecorderConfig) (*Recorder, error) {
	if conn == nil {
		return nil, fmt.Errorf("audit recorder nats connection is required")
	}
	policy, err := NormalizeAuditPolicy(cfg.Policy)
	if err != nil {
		return nil, err
	}
	cfg.Policy = policy
	var js natsgo.JetStreamContext
	if strings.TrimSpace(cfg.AuditPrefix) != "" {
		js, err = conn.JetStream()
		if err != nil {
			return nil, fmt.Errorf("open audit jetstream context: %w", err)
		}
	}
	return &Recorder{conn: conn, js: js, cfg: cfg, now: time.Now}, nil
}

func (r *Recorder) Record(req servicefunction.RequestEnvelope, endpoint Endpoint, resp servicefunction.ResponseEnvelope, duration time.Duration) error {
	if r == nil {
		return nil
	}
	now := r.now().UTC()
	event := ServiceCallEvent{
		ServiceCallID:      resp.ServiceCallID,
		ReferenceID:        resp.ReferenceID,
		AuditRef:           resp.AuditRef,
		SpaceID:            req.SpaceID,
		SessionID:          req.SessionID,
		RunID:              req.RunID,
		WorkflowID:         req.WorkflowID,
		AgentID:            req.AgentID,
		Service:            endpoint.Service,
		Function:           endpoint.Function,
		Subject:            endpoint.Subject,
		Status:             string(resp.Status),
		DurationMillis:     duration.Milliseconds(),
		TraceID:            resp.TraceID,
		RetentionExpiresAt: now.Add(r.cfg.Policy.Retention).Format(time.RFC3339Nano),
		RecordedAt:         now.Format(time.RFC3339Nano),
	}
	if resp.Error != nil {
		event.ErrorCategory = string(resp.Error.Category)
	}
	if r.cfg.Policy.SnapshotPolicy == SnapshotPolicyRedacted {
		event.RequestSnapshot = snapshotMetadata(req.Payload, r.cfg.Policy.MaxSnapshotBytes)
		event.ResponseSnapshot = snapshotMetadata(resp.Payload, r.cfg.Policy.MaxSnapshotBytes)
	}
	data, err := MarshalServiceCallEvent(event)
	if err != nil {
		return err
	}
	if strings.TrimSpace(r.cfg.AuditPrefix) != "" {
		msg := natsgo.NewMsg(ServiceCallRecordSubject(r.cfg.AuditPrefix, req.SpaceID, resp.ReferenceID))
		msg.Header.Set(natsgo.MsgIdHdr, resp.ReferenceID)
		msg.Data = data
		if _, err := r.js.PublishMsg(msg); err != nil {
			return fmt.Errorf("persist service call audit event: %w", err)
		}
	}
	if subject := ServiceCallRecordSubject(r.cfg.TelemetryPrefix, req.SpaceID, resp.ReferenceID); subject != "" {
		if err := r.conn.Publish(subject, data); err != nil {
			return fmt.Errorf("publish service call telemetry event: %w", err)
		}
	}
	return nil
}

// snapshotMetadata records that a payload existed and its boundedness without
// persisting user content. Generic service envelopes cannot determine whether
// otherwise innocuous text is a private document or prompt.
func snapshotMetadata(raw json.RawMessage, maxBytes int) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	data, _ := json.Marshal(map[string]any{
		"content_stored": false,
		"bytes":          len(raw),
		"over_limit":     len(raw) > maxBytes,
	})
	return data
}
