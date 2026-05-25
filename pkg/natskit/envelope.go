package natskit

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/quarkloop/pkg/boundary"
)

const EnvelopeVersion = 1

const (
	HeaderServiceCallID = "Quark-Service-Call-Id"
	HeaderSpaceID       = "Quark-Space-Id"
	HeaderSessionID     = "Quark-Session-Id"
	HeaderAgentID       = "Quark-Agent-Id"
	HeaderRunID         = "Quark-Run-Id"
	HeaderWorkflowID    = "Quark-Workflow-Id"
	HeaderTraceParent   = "traceparent"
	HeaderTraceState    = "tracestate"
)

type Actor string

const (
	ActorUser       Actor = "user"
	ActorAgent      Actor = "agent"
	ActorRuntime    Actor = "runtime"
	ActorWorkflow   Actor = "workflow"
	ActorSupervisor Actor = "supervisor"
)

type Status string

const (
	StatusOK    Status = "ok"
	StatusError Status = "error"
)

type ArtifactRef struct {
	ID     string `json:"id"`
	Kind   string `json:"kind,omitempty"`
	URI    string `json:"uri,omitempty"`
	Digest string `json:"digest,omitempty"`
}

type Usage struct {
	Provider       string          `json:"provider,omitempty"`
	Model          string          `json:"model,omitempty"`
	RequestID      string          `json:"request_id,omitempty"`
	InputTokens    int64           `json:"input_tokens,omitempty"`
	OutputTokens   int64           `json:"output_tokens,omitempty"`
	TotalTokens    int64           `json:"total_tokens,omitempty"`
	AdditionalJSON json.RawMessage `json:"additional_json,omitempty"`
}

// RequestEnvelope deliberately contains no service, function, or subject.
// The NATS subject supplied to Call or used by a responder owns routing.
type RequestEnvelope struct {
	Version       int             `json:"version"`
	ServiceCallID string          `json:"service_call_id"`
	SpaceID       string          `json:"space_id"`
	SessionID     string          `json:"session_id,omitempty"`
	AgentID       string          `json:"agent_id,omitempty"`
	RunID         string          `json:"run_id,omitempty"`
	WorkflowID    string          `json:"workflow_id,omitempty"`
	Actor         Actor           `json:"actor"`
	Payload       json.RawMessage `json:"payload"`
	ApprovalToken string          `json:"approval_token,omitempty"`
	TraceParent   string          `json:"traceparent,omitempty"`
	TraceState    string          `json:"tracestate,omitempty"`
	Artifacts     []ArtifactRef   `json:"artifacts,omitempty"`
}

type ResponseEnvelope struct {
	Version       int                   `json:"version"`
	ServiceCallID string                `json:"service_call_id"`
	ReferenceID   string                `json:"reference_id"`
	AuditRef      string                `json:"audit_ref"`
	TraceID       string                `json:"trace_id,omitempty"`
	Status        Status                `json:"status"`
	Final         bool                  `json:"final,omitempty"`
	Payload       json.RawMessage       `json:"payload,omitempty"`
	Artifacts     []ArtifactRef         `json:"artifacts,omitempty"`
	Usage         *Usage                `json:"usage,omitempty"`
	Diagnostics   []boundary.Diagnostic `json:"diagnostics,omitempty"`
	Error         *ErrorPayload         `json:"error,omitempty"`
}

type ErrorPayload struct {
	Boundary  boundary.Boundary `json:"boundary"`
	Category  boundary.Category `json:"category"`
	Operation string            `json:"operation,omitempty"`
	Message   string            `json:"message"`
}

func NewRequest(serviceCallID, spaceID string, actor Actor, payload json.RawMessage) (RequestEnvelope, error) {
	req := RequestEnvelope{
		Version:       EnvelopeVersion,
		ServiceCallID: strings.TrimSpace(serviceCallID),
		SpaceID:       strings.TrimSpace(spaceID),
		Actor:         actor,
		Payload:       cloneRaw(payload),
	}
	if err := req.Validate(); err != nil {
		return RequestEnvelope{}, err
	}
	return req, nil
}

func (e RequestEnvelope) Validate() error {
	if e.Version != EnvelopeVersion {
		return fmt.Errorf("unsupported nats request envelope version %d", e.Version)
	}
	if strings.TrimSpace(e.ServiceCallID) == "" {
		return errors.New("service_call_id is required")
	}
	if strings.TrimSpace(e.SpaceID) == "" {
		return errors.New("space_id is required")
	}
	switch e.Actor {
	case ActorUser, ActorAgent, ActorRuntime, ActorWorkflow, ActorSupervisor:
	default:
		return fmt.Errorf("invalid actor %q", e.Actor)
	}
	if len(e.Payload) == 0 || !json.Valid(e.Payload) {
		return errors.New("payload must be valid JSON")
	}
	return nil
}

func (e RequestEnvelope) Clone() RequestEnvelope {
	out := e
	out.Payload = cloneRaw(e.Payload)
	out.Artifacts = append([]ArtifactRef(nil), e.Artifacts...)
	return out
}

func (e RequestEnvelope) CorrelationHeaders() map[string]string {
	headers := make(map[string]string)
	for name, value := range map[string]string{
		HeaderServiceCallID: e.ServiceCallID,
		HeaderSpaceID:       e.SpaceID,
		HeaderSessionID:     e.SessionID,
		HeaderAgentID:       e.AgentID,
		HeaderRunID:         e.RunID,
		HeaderWorkflowID:    e.WorkflowID,
		HeaderTraceParent:   e.TraceParent,
		HeaderTraceState:    e.TraceState,
	} {
		if strings.TrimSpace(value) != "" {
			headers[name] = value
		}
	}
	return headers
}

func (e ResponseEnvelope) Validate() error {
	if e.Version != EnvelopeVersion {
		return fmt.Errorf("unsupported nats response envelope version %d", e.Version)
	}
	if strings.TrimSpace(e.ServiceCallID) == "" || strings.TrimSpace(e.ReferenceID) == "" || strings.TrimSpace(e.AuditRef) == "" {
		return errors.New("service_call_id, reference_id, and audit_ref are required")
	}
	if e.ReferenceID != ReferenceIDForServiceCall(e.ServiceCallID) || e.AuditRef != AuditRefForReference(e.ReferenceID) {
		return errors.New("audit references do not match service_call_id")
	}
	if e.TraceID != "" && !validTraceID.MatchString(e.TraceID) {
		return errors.New("trace_id must be a W3C trace identifier")
	}
	switch e.Status {
	case StatusOK:
		if e.Error != nil {
			return errors.New("ok response must not include error")
		}
	case StatusError:
		if e.Error == nil {
			return errors.New("error response requires error payload")
		}
	default:
		return fmt.Errorf("invalid response status %q", e.Status)
	}
	if len(e.Payload) > 0 && !json.Valid(e.Payload) {
		return errors.New("payload must be valid JSON")
	}
	return nil
}

func OKResponse(serviceCallID string, payload json.RawMessage) ResponseEnvelope {
	return response(serviceCallID, StatusOK, payload, nil)
}

func ErrorResponse(serviceCallID string, err error, defaultBoundary boundary.Boundary, operation string) ResponseEnvelope {
	payload := errorPayload(err, defaultBoundary, operation)
	return response(firstNonEmpty(serviceCallID, NewServiceCallID()), StatusError, nil, &payload)
}

func response(serviceCallID string, status Status, payload json.RawMessage, err *ErrorPayload) ResponseEnvelope {
	referenceID := ReferenceIDForServiceCall(serviceCallID)
	return ResponseEnvelope{
		Version:       EnvelopeVersion,
		ServiceCallID: serviceCallID,
		ReferenceID:   referenceID,
		AuditRef:      AuditRefForReference(referenceID),
		Status:        status,
		Payload:       cloneRaw(payload),
		Error:         err,
	}
}

func (e ResponseEnvelope) WithTraceParent(traceParent string) ResponseEnvelope {
	e.TraceID = TraceIDFromTraceParent(traceParent)
	return e
}

func NewServiceCallID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "call-unavailable"
	}
	return "call-" + hex.EncodeToString(data[:])
}

func ReferenceIDForServiceCall(serviceCallID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(serviceCallID)))
	return "ref-" + hex.EncodeToString(sum[:12])
}

func AuditRefForReference(referenceID string) string {
	return "service-call/" + strings.TrimSpace(referenceID)
}

var validTraceID = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TraceIDFromTraceParent(value string) string {
	parts := strings.Split(strings.TrimSpace(value), "-")
	if len(parts) != 4 || parts[0] != "00" || !validTraceID.MatchString(parts[1]) ||
		parts[1] == strings.Repeat("0", 32) || !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(parts[2]) ||
		parts[2] == strings.Repeat("0", 16) || !regexp.MustCompile(`^[0-9a-f]{2}$`).MatchString(parts[3]) {
		return ""
	}
	return parts[1]
}

func errorPayload(err error, defaultBoundary boundary.Boundary, operation string) ErrorPayload {
	if err == nil {
		return ErrorPayload{}
	}
	var bound *boundary.Error
	if errors.As(err, &bound) {
		return ErrorPayload{Boundary: bound.Boundary, Category: bound.Category, Operation: bound.Operation, Message: bound.Message}
	}
	category := boundary.FromError(defaultBoundary, operation, err)
	if errors.Is(err, context.DeadlineExceeded) {
		category = boundary.Wrap(defaultBoundary, boundary.Deadline, operation, err)
	}
	if errors.Is(err, context.Canceled) {
		category = boundary.Wrap(defaultBoundary, boundary.Canceled, operation, err)
	}
	return ErrorPayload{Boundary: category.Boundary, Category: category.Category, Operation: category.Operation, Message: category.Message}
}

func cloneRaw(in json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), in...)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
