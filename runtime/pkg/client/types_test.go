package agentclient

import (
	"encoding/json"
	"testing"
)

func TestClientActivityRecordOwnsDecodedRawMessage(t *testing.T) {
	raw := []byte(`{"id":"a1","type":"tool_result","data":{"name":"fs"}}`)
	var record ActivityRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		t.Fatalf("decode activity record: %v", err)
	}
	raw[len(raw)-2] = 'x'
	if string(record.Data) != `{"name":"fs"}` {
		t.Fatalf("activity record reused response backing bytes: %s", record.Data)
	}
}

func TestRuntimeClientPathJoinIsClientOwned(t *testing.T) {
	if got := joinPath("/api/v1/runtime/", "/health"); got != "/api/v1/runtime/health" {
		t.Fatalf("join path = %q", got)
	}
	if got := joinPath("", "/v1/activity"); got != "/v1/activity" {
		t.Fatalf("empty-base join path = %q", got)
	}
}
