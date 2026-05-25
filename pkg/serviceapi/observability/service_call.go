package observability

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	DefaultAuditPrefix     = "audit"
	DefaultTelemetryPrefix = "telemetry"

	ServiceCallEventType = "service_call"
	serviceCallsToken    = "service_calls"
	unknownSpaceToken    = "unknown"
)

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

func NewServiceCallEvent(input ServiceCallEvent) ServiceCallEvent {
	event := input
	event.Type = strings.TrimSpace(firstNonEmpty(event.Type, ServiceCallEventType))
	event.ServiceCallID = strings.TrimSpace(event.ServiceCallID)
	event.ReferenceID = strings.TrimSpace(event.ReferenceID)
	event.AuditRef = strings.TrimSpace(event.AuditRef)
	event.SpaceID = strings.TrimSpace(event.SpaceID)
	event.SessionID = strings.TrimSpace(event.SessionID)
	event.RunID = strings.TrimSpace(event.RunID)
	event.WorkflowID = strings.TrimSpace(event.WorkflowID)
	event.AgentID = strings.TrimSpace(event.AgentID)
	event.Service = strings.TrimSpace(event.Service)
	event.Function = strings.TrimSpace(event.Function)
	event.Subject = strings.TrimSpace(event.Subject)
	event.Status = strings.TrimSpace(event.Status)
	event.ErrorCategory = strings.TrimSpace(event.ErrorCategory)
	event.TraceID = strings.TrimSpace(event.TraceID)
	event.RetentionExpiresAt = strings.TrimSpace(event.RetentionExpiresAt)
	event.RecordedAt = strings.TrimSpace(event.RecordedAt)
	if event.RecordedAt == "" {
		event.RecordedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return event
}

func MarshalServiceCallEvent(input ServiceCallEvent) ([]byte, error) {
	event := NewServiceCallEvent(input)
	if err := event.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(event)
}

func (e ServiceCallEvent) Validate() error {
	required := map[string]string{
		"type":                 e.Type,
		"service_call_id":      e.ServiceCallID,
		"reference_id":         e.ReferenceID,
		"audit_ref":            e.AuditRef,
		"service":              e.Service,
		"function":             e.Function,
		"subject":              e.Subject,
		"status":               e.Status,
		"retention_expires_at": e.RetentionExpiresAt,
		"recorded_at":          e.RecordedAt,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("service call event %s is required", name)
		}
	}
	if e.DurationMillis < 0 {
		return fmt.Errorf("service call event duration_millis must be >= 0")
	}
	if _, err := time.Parse(time.RFC3339Nano, e.RecordedAt); err != nil {
		return fmt.Errorf("service call event recorded_at must be RFC3339Nano: %w", err)
	}
	if _, err := time.Parse(time.RFC3339Nano, e.RetentionExpiresAt); err != nil {
		return fmt.Errorf("service call event retention_expires_at must be RFC3339Nano: %w", err)
	}
	for name, snapshot := range map[string]json.RawMessage{"request_snapshot": e.RequestSnapshot, "response_snapshot": e.ResponseSnapshot} {
		if len(snapshot) > 0 && !json.Valid(snapshot) {
			return fmt.Errorf("service call event %s must be valid JSON", name)
		}
	}
	return nil
}

func ServiceCallSubject(prefix, spaceID string) string {
	prefix = strings.Trim(strings.TrimSpace(prefix), ".")
	if prefix == "" {
		return ""
	}
	space := SubjectToken(spaceID)
	if space == "" {
		space = unknownSpaceToken
	}
	return prefix + "." + space + "." + serviceCallsToken + ".>"
}

func ServiceCallRecordSubject(prefix, spaceID, referenceID string) string {
	prefix = strings.Trim(strings.TrimSpace(prefix), ".")
	reference := SubjectToken(referenceID)
	if prefix == "" || reference == "" {
		return ""
	}
	space := SubjectToken(spaceID)
	if space == "" {
		space = unknownSpaceToken
	}
	return prefix + "." + space + "." + serviceCallsToken + "." + reference
}

func SubjectToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastUnderscore = false
		case r == '_' || r == '-' || r == '.' || r == '/':
			if !lastUnderscore && b.Len() > 0 {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	return strings.Trim(b.String(), "_")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
