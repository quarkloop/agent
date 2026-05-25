package natskit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	natsgo "github.com/nats-io/nats.go"
)

const (
	DefaultAuditRetention   = 90 * 24 * time.Hour
	DefaultSnapshotMaxBytes = 4096
	SnapshotPolicyNone      = "none"
	SnapshotPolicyMetadata  = "metadata"
)

type AuditPolicy struct {
	Retention        time.Duration
	SnapshotPolicy   string
	MaxSnapshotBytes int
}

type ServiceCallEvent struct {
	Type               string          `json:"type"`
	ServiceCallID      string          `json:"service_call_id"`
	ReferenceID        string          `json:"reference_id"`
	AuditRef           string          `json:"audit_ref"`
	SpaceID            string          `json:"space_id,omitempty"`
	SessionID          string          `json:"session_id,omitempty"`
	RunID              string          `json:"run_id,omitempty"`
	WorkflowID         string          `json:"workflow_id,omitempty"`
	AgentID            string          `json:"agent_id,omitempty"`
	Service            string          `json:"service"`
	Function           string          `json:"function"`
	Subject            string          `json:"subject"`
	Status             string          `json:"status"`
	ErrorCategory      string          `json:"error_category,omitempty"`
	DurationMillis     int64           `json:"duration_millis"`
	TraceID            string          `json:"trace_id,omitempty"`
	RequestSnapshot    json.RawMessage `json:"request_snapshot,omitempty"`
	ResponseSnapshot   json.RawMessage `json:"response_snapshot,omitempty"`
	RetentionExpiresAt string          `json:"retention_expires_at"`
	RecordedAt         string          `json:"recorded_at"`
}

type Recorder struct {
	client *Client
	js     natsgo.JetStreamContext
	cfg    Config
	now    func() time.Time
}

func DefaultAuditPolicy() AuditPolicy {
	return AuditPolicy{Retention: DefaultAuditRetention, SnapshotPolicy: SnapshotPolicyNone, MaxSnapshotBytes: DefaultSnapshotMaxBytes}
}

func newRecorder(client *Client) (*Recorder, error) {
	policy, err := normalizeAuditPolicy(client.cfg.AuditPolicy)
	if err != nil {
		return nil, err
	}
	client.cfg.AuditPolicy = policy
	var js natsgo.JetStreamContext
	if client.cfg.AuditPrefix != "" {
		js, err = client.jetStream()
		if err != nil {
			return nil, fmt.Errorf("open audit jetstream context: %w", err)
		}
	}
	return &Recorder{client: client, js: js, cfg: client.cfg, now: time.Now}, nil
}

func normalizeAuditPolicy(policy AuditPolicy) (AuditPolicy, error) {
	if policy.Retention <= 0 {
		policy.Retention = DefaultAuditRetention
	}
	policy.SnapshotPolicy = strings.ToLower(strings.TrimSpace(policy.SnapshotPolicy))
	if policy.SnapshotPolicy == "" {
		policy.SnapshotPolicy = SnapshotPolicyNone
	}
	if policy.SnapshotPolicy == "redacted" {
		policy.SnapshotPolicy = SnapshotPolicyMetadata
	}
	if policy.SnapshotPolicy != SnapshotPolicyNone && policy.SnapshotPolicy != SnapshotPolicyMetadata {
		return AuditPolicy{}, fmt.Errorf("unsupported audit snapshot policy %q", policy.SnapshotPolicy)
	}
	if policy.MaxSnapshotBytes <= 0 {
		policy.MaxSnapshotBytes = DefaultSnapshotMaxBytes
	}
	return policy, nil
}

func (r *Recorder) Record(req RequestEnvelope, operation Operation, resp ResponseEnvelope, duration time.Duration) error {
	if r == nil {
		return nil
	}
	now := r.now().UTC()
	event := ServiceCallEvent{
		Type:               "service_call",
		ServiceCallID:      resp.ServiceCallID,
		ReferenceID:        resp.ReferenceID,
		AuditRef:           resp.AuditRef,
		SpaceID:            req.SpaceID,
		SessionID:          req.SessionID,
		RunID:              req.RunID,
		WorkflowID:         req.WorkflowID,
		AgentID:            req.AgentID,
		Service:            operation.Owner,
		Function:           operation.Function,
		Subject:            operation.Subject,
		Status:             string(resp.Status),
		DurationMillis:     duration.Milliseconds(),
		TraceID:            resp.TraceID,
		RetentionExpiresAt: now.Add(r.cfg.AuditPolicy.Retention).Format(time.RFC3339Nano),
		RecordedAt:         now.Format(time.RFC3339Nano),
	}
	if resp.Error != nil {
		event.ErrorCategory = string(resp.Error.Category)
	}
	if r.cfg.AuditPolicy.SnapshotPolicy == SnapshotPolicyMetadata {
		event.RequestSnapshot = snapshotMetadata(req.Payload, r.cfg.AuditPolicy.MaxSnapshotBytes)
		event.ResponseSnapshot = snapshotMetadata(resp.Payload, r.cfg.AuditPolicy.MaxSnapshotBytes)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal nats service call audit: %w", err)
	}
	if r.cfg.AuditPrefix != "" {
		msg := natsgo.NewMsg(ServiceCallRecordSubject(r.cfg.AuditPrefix, req.SpaceID, resp.ReferenceID))
		msg.Header.Set(natsgo.MsgIdHdr, resp.ReferenceID)
		msg.Data = data
		if _, err := r.js.PublishMsg(msg); err != nil {
			return fmt.Errorf("persist nats service call audit: %w", err)
		}
	}
	if r.cfg.TelemetryPrefix != "" {
		ctx, cancel := context.WithTimeout(context.Background(), r.cfg.Timeout)
		err := r.client.Publish(ctx, ServiceCallRecordSubject(r.cfg.TelemetryPrefix, req.SpaceID, resp.ReferenceID), data, nil)
		cancel()
		if err != nil {
			return fmt.Errorf("publish service call telemetry: %w", err)
		}
	}
	return nil
}

func ServiceCallRecordSubject(prefix, spaceID, referenceID string) string {
	prefix = strings.Trim(strings.TrimSpace(prefix), ".")
	if prefix == "" {
		return ""
	}
	space := stableToken(spaceID)
	if space == "" {
		space = "unknown"
	}
	reference := stableToken(referenceID)
	return prefix + "." + space + ".service_calls." + reference
}

func snapshotMetadata(raw json.RawMessage, maxBytes int) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	data, _ := json.Marshal(map[string]any{"content_stored": false, "bytes": len(raw), "over_limit": len(raw) > maxBytes})
	return data
}
