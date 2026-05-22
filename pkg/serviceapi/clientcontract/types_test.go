package clientcontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRequestResponseEnvelopeValidationAndCopying(t *testing.T) {
	req, err := NewRequest("req-1", "space-1", CreateSessionRequest{
		SpaceID: "space-1",
		Type:    SessionTypeChat,
		Title:   "help",
	})
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	var decoded CreateSessionRequest
	if err := req.DecodePayload(&decoded); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if decoded.Title != "help" {
		t.Fatalf("decoded request = %+v", decoded)
	}
	clone := req.Clone()
	clone.Payload[0] = '['
	if !strings.HasPrefix(string(req.Payload), "{") {
		t.Fatalf("request clone reused payload backing array: %s", req.Payload)
	}

	resp, err := OK(req.RequestID, SessionInfo{ID: "session-1", Type: SessionTypeChat})
	if err != nil {
		t.Fatalf("ok response: %v", err)
	}
	var session SessionInfo
	if err := resp.DecodePayload(&session); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if session.ID != "session-1" {
		t.Fatalf("decoded response = %+v", session)
	}
	respClone := resp.Clone()
	respClone.Payload[0] = '['
	if !strings.HasPrefix(string(resp.Payload), "{") {
		t.Fatalf("response clone reused payload backing array: %s", resp.Payload)
	}
}

func TestEnvelopeRejectsMalformedJSONAndErrorStatus(t *testing.T) {
	req := RequestEnvelope{Version: Version, RequestID: "req-1", Payload: json.RawMessage(`{`)}
	if err := req.Validate(); err == nil {
		t.Fatal("expected malformed request payload error")
	}
	resp := ResponseEnvelope{Version: Version, RequestID: "req-1", Status: "error"}
	if err := resp.Validate(); err == nil {
		t.Fatal("expected missing error payload")
	}
	errResp := Error("req-1", "permission", "denied")
	if err := errResp.DecodePayload(&struct{}{}); err == nil {
		t.Fatal("expected error response to decode as error")
	}
}
