package servicefunction

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/quarkloop/pkg/boundary"
	"github.com/quarkloop/pkg/boundary/redaction"
)

const EnvelopeVersion = 2

type Actor string

const (
	ActorUser       Actor = "user"
	ActorAgent      Actor = "agent"
	ActorRuntime    Actor = "runtime"
	ActorWorkflow   Actor = "workflow"
	ActorSupervisor Actor = "supervisor"
)

type ResponseStatus string

const (
	StatusOK    ResponseStatus = "ok"
	StatusError ResponseStatus = "error"
)

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

type RequestEnvelope struct {
	Version       int             `json:"version"`
	ServiceCallID string          `json:"service_call_id"`
	SpaceID       string          `json:"space_id"`
	SessionID     string          `json:"session_id,omitempty"`
	AgentID       string          `json:"agent_id,omitempty"`
	RunID         string          `json:"run_id,omitempty"`
	WorkflowID    string          `json:"workflow_id,omitempty"`
	Actor         Actor           `json:"actor"`
	Service       string          `json:"service"`
	Function      string          `json:"function"`
	Subject       string          `json:"subject"`
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
	Status        ResponseStatus        `json:"status"`
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

func NewRequest(serviceCallID, spaceID string, actor Actor, descriptor Descriptor, payload json.RawMessage) (RequestEnvelope, error) {
	req := RequestEnvelope{
		Version:       EnvelopeVersion,
		ServiceCallID: strings.TrimSpace(serviceCallID),
		SpaceID:       strings.TrimSpace(spaceID),
		Actor:         actor,
		Service:       descriptor.Service,
		Function:      descriptor.Function,
		Subject:       descriptor.Subject,
		Payload:       cloneRawMessage(payload),
	}
	if err := req.Validate(); err != nil {
		return RequestEnvelope{}, err
	}
	return req, nil
}

func (e RequestEnvelope) Validate() error {
	if e.Version != EnvelopeVersion {
		return fmt.Errorf("unsupported request envelope version %d", e.Version)
	}
	for name, value := range map[string]string{
		"service_call_id": e.ServiceCallID,
		"space_id":        e.SpaceID,
		"service":         e.Service,
		"function":        e.Function,
		"subject":         e.Subject,
	} {
		if err := requireNonEmpty(name, value); err != nil {
			return err
		}
	}
	switch e.Actor {
	case ActorUser, ActorAgent, ActorRuntime, ActorWorkflow, ActorSupervisor:
	default:
		return fmt.Errorf("invalid actor %q", e.Actor)
	}
	if err := ValidateSubject(e.Subject); err != nil {
		return err
	}
	if len(e.Payload) == 0 {
		return errorsForPayload("payload is required")
	}
	if !json.Valid(e.Payload) {
		return errorsForPayload("payload must be valid JSON")
	}
	return nil
}

func (e RequestEnvelope) Clone() RequestEnvelope {
	out := e
	out.Payload = cloneRawMessage(e.Payload)
	out.Artifacts = cloneArtifacts(e.Artifacts)
	return out
}

func (e RequestEnvelope) RedactedClone() RequestEnvelope {
	out := e.Clone()
	out.Payload = redactRawJSON(out.Payload)
	if out.ApprovalToken != "" {
		out.ApprovalToken = "[redacted]"
	}
	out.TraceParent = redaction.RedactString(out.TraceParent)
	out.TraceState = redaction.RedactString(out.TraceState)
	for i := range out.Artifacts {
		out.Artifacts[i].URI = redaction.RedactString(out.Artifacts[i].URI)
		out.Artifacts[i].Digest = redaction.RedactString(out.Artifacts[i].Digest)
	}
	return out
}

func (e RequestEnvelope) CorrelationHeaders() map[string]string {
	headers := make(map[string]string)
	addHeader(headers, HeaderServiceCallID, e.ServiceCallID)
	addHeader(headers, HeaderSpaceID, e.SpaceID)
	addHeader(headers, HeaderSessionID, e.SessionID)
	addHeader(headers, HeaderAgentID, e.AgentID)
	addHeader(headers, HeaderRunID, e.RunID)
	addHeader(headers, HeaderWorkflowID, e.WorkflowID)
	addHeader(headers, HeaderTraceParent, e.TraceParent)
	addHeader(headers, HeaderTraceState, e.TraceState)
	return headers
}

func (e ResponseEnvelope) Validate() error {
	if e.Version != EnvelopeVersion {
		return fmt.Errorf("unsupported response envelope version %d", e.Version)
	}
	for name, value := range map[string]string{
		"service_call_id": e.ServiceCallID,
		"reference_id":    e.ReferenceID,
		"audit_ref":       e.AuditRef,
	} {
		if err := requireNonEmpty(name, value); err != nil {
			return err
		}
	}
	if e.TraceID != "" && !validTraceID.MatchString(e.TraceID) {
		return fmt.Errorf("trace_id must be a W3C trace identifier")
	}
	if e.ReferenceID != ReferenceIDForServiceCall(e.ServiceCallID) {
		return fmt.Errorf("reference_id does not match service_call_id")
	}
	if e.AuditRef != AuditRefForReference(e.ReferenceID) {
		return fmt.Errorf("audit_ref does not match reference_id")
	}
	switch e.Status {
	case StatusOK:
		if e.Error != nil {
			return fmt.Errorf("ok response must not include error")
		}
	case StatusError:
		if e.Error == nil {
			return fmt.Errorf("error response requires error payload")
		}
	default:
		return fmt.Errorf("invalid response status %q", e.Status)
	}
	if len(e.Payload) > 0 && !json.Valid(e.Payload) {
		return errorsForPayload("payload must be valid JSON")
	}
	if e.Usage != nil && len(e.Usage.AdditionalJSON) > 0 && !json.Valid(e.Usage.AdditionalJSON) {
		return fmt.Errorf("usage.additional_json must be valid JSON")
	}
	return nil
}

func (e ResponseEnvelope) Clone() ResponseEnvelope {
	out := e
	out.Payload = cloneRawMessage(e.Payload)
	out.Artifacts = cloneArtifacts(e.Artifacts)
	out.Diagnostics = append([]boundary.Diagnostic(nil), e.Diagnostics...)
	if e.Usage != nil {
		usage := *e.Usage
		usage.AdditionalJSON = cloneRawMessage(e.Usage.AdditionalJSON)
		out.Usage = &usage
	}
	if e.Error != nil {
		errPayload := *e.Error
		out.Error = &errPayload
	}
	return out
}

func (e ResponseEnvelope) RedactedClone() ResponseEnvelope {
	out := e.Clone()
	out.Payload = redactRawJSON(out.Payload)
	for i := range out.Artifacts {
		out.Artifacts[i].URI = redaction.RedactString(out.Artifacts[i].URI)
		out.Artifacts[i].Digest = redaction.RedactString(out.Artifacts[i].Digest)
	}
	if out.Usage != nil {
		out.Usage.RequestID = redaction.RedactString(out.Usage.RequestID)
		out.Usage.AdditionalJSON = redactRawJSON(out.Usage.AdditionalJSON)
	}
	if out.Error != nil {
		out.Error.Message = redaction.RedactString(out.Error.Message)
	}
	for i := range out.Diagnostics {
		out.Diagnostics[i].Message = redaction.RedactString(out.Diagnostics[i].Message)
		out.Diagnostics[i].Hint = redaction.RedactString(out.Diagnostics[i].Hint)
	}
	return out
}

func OKResponse(serviceCallID string, payload json.RawMessage) ResponseEnvelope {
	refs := NewReferences(serviceCallID, "")
	return ResponseEnvelope{
		Version:       EnvelopeVersion,
		ServiceCallID: refs.ServiceCallID,
		ReferenceID:   refs.ReferenceID,
		AuditRef:      refs.AuditRef,
		Status:        StatusOK,
		Payload:       cloneRawMessage(payload),
	}
}

func ErrorResponse(serviceCallID string, err error, defaultBoundary boundary.Boundary, operation string) ResponseEnvelope {
	boundaryErr := BoundaryError(err, defaultBoundary, operation)
	if boundaryErr == nil {
		boundaryErr = boundary.New(defaultBoundary, boundary.Unknown, operation, "unknown service function error")
	}
	diag := boundary.DiagnosticFromError(boundaryErr, defaultBoundary, operation)
	refs := NewReferences(serviceCallID, "")
	return ResponseEnvelope{
		Version:       EnvelopeVersion,
		ServiceCallID: refs.ServiceCallID,
		ReferenceID:   refs.ReferenceID,
		AuditRef:      refs.AuditRef,
		Status:        StatusError,
		Diagnostics:   []boundary.Diagnostic{diag},
		Error: &ErrorPayload{
			Boundary:  boundaryErr.Boundary,
			Category:  boundaryErr.Category,
			Operation: boundaryErr.Operation,
			Message:   boundaryErr.Message,
		},
	}
}

type References struct {
	ServiceCallID string
	ReferenceID   string
	AuditRef      string
	TraceID       string
}

var (
	validTraceID     = regexp.MustCompile(`^[0-9a-f]{32}$`)
	validTraceParent = regexp.MustCompile(`^00-([0-9a-f]{32})-([0-9a-f]{16})-([0-9a-f]{2})$`)
)

func NewServiceCallID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return "svc-call-" + hex.EncodeToString(buf[:])
	}
	return fmt.Sprintf("svc-call-%d", time.Now().UnixNano())
}

func NewReferences(serviceCallID, traceParent string) References {
	serviceCallID = strings.TrimSpace(serviceCallID)
	if serviceCallID == "" {
		serviceCallID = NewServiceCallID()
	}
	referenceID := ReferenceIDForServiceCall(serviceCallID)
	return References{
		ServiceCallID: serviceCallID,
		ReferenceID:   referenceID,
		AuditRef:      AuditRefForReference(referenceID),
		TraceID:       TraceIDFromTraceParent(traceParent),
	}
}

func ReferenceIDForServiceCall(serviceCallID string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(serviceCallID)))
	return "svc-ref-" + hex.EncodeToString(digest[:16])
}

func AuditRefForReference(referenceID string) string {
	return "urn:quark:audit:service-call:" + strings.TrimSpace(referenceID)
}

func TraceIDFromTraceParent(traceParent string) string {
	parts := validTraceParent.FindStringSubmatch(strings.TrimSpace(traceParent))
	if len(parts) != 4 || parts[1] == strings.Repeat("0", 32) || parts[2] == strings.Repeat("0", 16) {
		return ""
	}
	return parts[1]
}

func (e ResponseEnvelope) WithTraceParent(traceParent string) ResponseEnvelope {
	out := e.Clone()
	out.TraceID = TraceIDFromTraceParent(traceParent)
	return out
}

func addHeader(headers map[string]string, key, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		headers[key] = value
	}
}

func cloneArtifacts(in []ArtifactRef) []ArtifactRef {
	return append([]ArtifactRef(nil), in...)
}

func redactRawJSON(in json.RawMessage) json.RawMessage {
	if len(in) == 0 {
		return nil
	}
	return json.RawMessage(redaction.RedactBytes(in))
}

func errorsForPayload(message string) error {
	return errors.New(message)
}
