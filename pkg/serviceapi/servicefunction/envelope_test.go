package servicefunction

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/quarkloop/pkg/boundary"
)

func TestRequestEnvelopeValidationHeadersCloneAndRedaction(t *testing.T) {
	descriptor := validDescriptor(t)
	req, err := NewRequest("call-1", "space-1", ActorAgent, descriptor, json.RawMessage(`{"authorization":"Bearer secret-token-value"}`))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.SessionID = "session-1"
	req.RunID = "run-1"
	req.TraceParent = "00-trace"
	req.TraceState = "vendor=value"
	req.ApprovalToken = "approval-secret"
	req.Artifacts = []ArtifactRef{{ID: "artifact-1", URI: "file:///tmp/doc.pdf"}}

	headers := req.CorrelationHeaders()
	if headers[HeaderCallID] != "call-1" ||
		headers[HeaderSessionID] != "session-1" ||
		headers[HeaderTraceParent] != "00-trace" ||
		headers[HeaderTraceState] != "vendor=value" {
		t.Fatalf("headers = %+v", headers)
	}

	clone := req.Clone()
	clone.Payload[0] = '['
	clone.Artifacts[0].URI = "mutated"
	if !strings.HasPrefix(string(req.Payload), "{") || req.Artifacts[0].URI != "file:///tmp/doc.pdf" {
		t.Fatalf("request clone reused mutable data: %+v", req)
	}

	redacted := req.RedactedClone()
	if strings.Contains(string(redacted.Payload), "secret-token-value") || redacted.ApprovalToken != "[redacted]" {
		t.Fatalf("redacted request leaked secret: %+v", redacted)
	}
}

func TestRequestEnvelopeRejectsMalformedPayload(t *testing.T) {
	descriptor := validDescriptor(t)
	req, err := NewRequest("call-1", "space-1", ActorAgent, descriptor, json.RawMessage(`{`))
	if err == nil {
		t.Fatalf("expected malformed payload error, got request %+v", req)
	}
}

func TestResponseEnvelopeValidationCloneRedactionAndErrorMapping(t *testing.T) {
	resp := OKResponse("call-1", json.RawMessage(`{"token":"sk-or-v1-secret-value"}`))
	resp.Artifacts = []ArtifactRef{{ID: "artifact-1", Digest: "secret-digest"}}
	resp.Usage = &Usage{Provider: "openrouter", RequestID: "sk-or-v1-request-secret", AdditionalJSON: json.RawMessage(`{"api_key":"secret-key-value"}`)}
	if err := resp.Validate(); err != nil {
		t.Fatalf("validate ok response: %v", err)
	}

	clone := resp.Clone()
	clone.Payload[0] = '['
	clone.Artifacts[0].Digest = "mutated"
	clone.Usage.AdditionalJSON[0] = '['
	if !strings.HasPrefix(string(resp.Payload), "{") || resp.Artifacts[0].Digest != "secret-digest" || !strings.HasPrefix(string(resp.Usage.AdditionalJSON), "{") {
		t.Fatalf("response clone reused mutable data: %+v", resp)
	}

	redacted := resp.RedactedClone()
	if strings.Contains(string(redacted.Payload), "secret") ||
		strings.Contains(redacted.Usage.RequestID, "request-secret") ||
		strings.Contains(string(redacted.Usage.AdditionalJSON), "secret-key-value") {
		t.Fatalf("redacted response leaked secret: %+v", redacted)
	}

	errResp := ErrorResponse("call-2", context.DeadlineExceeded, boundary.Service, "svc.io.v1.read_file")
	if err := errResp.Validate(); err != nil {
		t.Fatalf("validate error response: %v", err)
	}
	if errResp.Error.Category != boundary.Deadline || errResp.Diagnostics[0].Category != string(boundary.Deadline) {
		t.Fatalf("deadline error was not mapped: %+v", errResp)
	}

	unknown := ErrorPayloadFromError(errors.New("no responders available for request"), boundary.Runtime, "svc.io.v1.missing")
	if unknown.Boundary != boundary.Service || unknown.Category != boundary.Unavailable {
		t.Fatalf("no responders mapping = %+v", unknown)
	}
}

func TestResponseEnvelopeRejectsMalformedPayload(t *testing.T) {
	resp := OKResponse("call-1", json.RawMessage(`{`))
	if err := resp.Validate(); err == nil {
		t.Fatal("expected malformed response payload error")
	}
}

func validDescriptor(t *testing.T) Descriptor {
	t.Helper()
	descriptor, err := NewDescriptor("io", "ReadFile", DescriptorOptions{
		InputSchema:  objectSchema,
		OutputSchema: objectSchema,
		Risk:         RiskRead,
	})
	if err != nil {
		t.Fatalf("new descriptor: %v", err)
	}
	return descriptor
}
