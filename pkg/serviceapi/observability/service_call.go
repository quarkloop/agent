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
	Type           string `json:"type"`
	CallID         string `json:"call_id,omitempty"`
	SpaceID        string `json:"space_id,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
	RunID          string `json:"run_id,omitempty"`
	WorkflowID     string `json:"workflow_id,omitempty"`
	AgentID        string `json:"agent_id,omitempty"`
	Service        string `json:"service"`
	Function       string `json:"function"`
	Subject        string `json:"subject"`
	Status         string `json:"status"`
	ErrorCategory  string `json:"error_category,omitempty"`
	DurationMillis int64  `json:"duration_millis"`
	TraceParent    string `json:"traceparent,omitempty"`
	TraceState     string `json:"tracestate,omitempty"`
	RecordedAt     string `json:"recorded_at"`
}

func NewServiceCallEvent(input ServiceCallEvent) ServiceCallEvent {
	event := input
	event.Type = strings.TrimSpace(firstNonEmpty(event.Type, ServiceCallEventType))
	event.CallID = strings.TrimSpace(event.CallID)
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
	event.TraceParent = strings.TrimSpace(event.TraceParent)
	event.TraceState = strings.TrimSpace(event.TraceState)
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
		"type":        e.Type,
		"service":     e.Service,
		"function":    e.Function,
		"subject":     e.Subject,
		"status":      e.Status,
		"recorded_at": e.RecordedAt,
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
	return prefix + "." + space + "." + serviceCallsToken
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
