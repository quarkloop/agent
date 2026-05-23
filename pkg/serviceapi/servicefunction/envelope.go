package servicefunction

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/quarkloop/pkg/boundary"
	"github.com/quarkloop/pkg/boundary/redaction"
)

const EnvelopeVersion = 1

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
	HeaderCallID      = "Quark-Call-Id"
	HeaderSpaceID     = "Quark-Space-Id"
	HeaderSessionID   = "Quark-Session-Id"
	HeaderAgentID     = "Quark-Agent-Id"
	HeaderRunID       = "Quark-Run-Id"
	HeaderWorkflowID  = "Quark-Workflow-Id"
	HeaderTraceParent = "traceparent"
	HeaderTraceState  = "tracestate"
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
	CallID        string          `json:"call_id"`
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
	Version     int                   `json:"version"`
	CallID      string                `json:"call_id"`
	Status      ResponseStatus        `json:"status"`
	Payload     json.RawMessage       `json:"payload,omitempty"`
	Artifacts   []ArtifactRef         `json:"artifacts,omitempty"`
	Usage       *Usage                `json:"usage,omitempty"`
	Diagnostics []boundary.Diagnostic `json:"diagnostics,omitempty"`
	Error       *ErrorPayload         `json:"error,omitempty"`
}

type ErrorPayload struct {
	Boundary  boundary.Boundary `json:"boundary"`
	Category  boundary.Category `json:"category"`
	Operation string            `json:"operation,omitempty"`
	Message   string            `json:"message"`
}

func NewRequest(callID, spaceID string, actor Actor, descriptor Descriptor, payload json.RawMessage) (RequestEnvelope, error) {
	req := RequestEnvelope{
		Version:  EnvelopeVersion,
		CallID:   strings.TrimSpace(callID),
		SpaceID:  strings.TrimSpace(spaceID),
		Actor:    actor,
		Service:  descriptor.Service,
		Function: descriptor.Function,
		Subject:  descriptor.Subject,
		Payload:  cloneRawMessage(payload),
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
		"call_id":  e.CallID,
		"space_id": e.SpaceID,
		"service":  e.Service,
		"function": e.Function,
		"subject":  e.Subject,
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
	addHeader(headers, HeaderCallID, e.CallID)
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
	if err := requireNonEmpty("call_id", e.CallID); err != nil {
		return err
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

func OKResponse(callID string, payload json.RawMessage) ResponseEnvelope {
	return ResponseEnvelope{
		Version: EnvelopeVersion,
		CallID:  strings.TrimSpace(callID),
		Status:  StatusOK,
		Payload: cloneRawMessage(payload),
	}
}

func ErrorResponse(callID string, err error, defaultBoundary boundary.Boundary, operation string) ResponseEnvelope {
	boundaryErr := BoundaryError(err, defaultBoundary, operation)
	if boundaryErr == nil {
		boundaryErr = boundary.New(defaultBoundary, boundary.Unknown, operation, "unknown service function error")
	}
	diag := boundary.DiagnosticFromError(boundaryErr, defaultBoundary, operation)
	return ResponseEnvelope{
		Version:     EnvelopeVersion,
		CallID:      strings.TrimSpace(callID),
		Status:      StatusError,
		Diagnostics: []boundary.Diagnostic{diag},
		Error: &ErrorPayload{
			Boundary:  boundaryErr.Boundary,
			Category:  boundaryErr.Category,
			Operation: boundaryErr.Operation,
			Message:   boundaryErr.Message,
		},
	}
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
