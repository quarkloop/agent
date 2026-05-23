package observability

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestServiceCallSubjectScopesBySanitizedSpace(t *testing.T) {
	got := ServiceCallSubject("audit.", "Space.One/Two")
	if got != "audit.space_one_two.service_calls" {
		t.Fatalf("subject = %q", got)
	}
	if got := ServiceCallSubject("telemetry", ""); got != "telemetry.unknown.service_calls" {
		t.Fatalf("empty space subject = %q", got)
	}
}

func TestMarshalServiceCallEventRequiresSchemaFields(t *testing.T) {
	_, err := MarshalServiceCallEvent(ServiceCallEvent{
		Service:        "indexer",
		Function:       "query",
		Subject:        "svc.indexer.v1.query",
		Status:         "ok",
		DurationMillis: 1,
		RecordedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("valid event rejected: %v", err)
	}

	_, err = MarshalServiceCallEvent(ServiceCallEvent{
		Function:   "query",
		Subject:    "svc.indexer.v1.query",
		Status:     "ok",
		RecordedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err == nil || !strings.Contains(err.Error(), "service") {
		t.Fatalf("expected service validation error, got %v", err)
	}
}

func TestServiceCallEventDoesNotExposePayloadFields(t *testing.T) {
	data, err := MarshalServiceCallEvent(ServiceCallEvent{
		CallID:         "call-1",
		SpaceID:        "space-1",
		Service:        "embedding",
		Function:       "embed",
		Subject:        "svc.embedding.v1.embed",
		Status:         "ok",
		DurationMillis: 2,
		TraceParent:    "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01",
		TraceState:     "vendor=value",
		RecordedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode event: %v", err)
	}
	for _, forbidden := range []string{"payload", "request", "response", "arguments", "input"} {
		if _, ok := payload[forbidden]; ok {
			t.Fatalf("event exposes forbidden field %q: %s", forbidden, data)
		}
	}
	if payload["type"] != ServiceCallEventType {
		t.Fatalf("event type missing: %s", data)
	}
}
