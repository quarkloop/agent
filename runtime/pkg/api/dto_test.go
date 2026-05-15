package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/quarkloop/runtime/pkg/activity"
	"github.com/quarkloop/runtime/pkg/message"
)

func TestMapMessageResponsesCopiesBoundaryShape(t *testing.T) {
	in := []message.Message{{
		ID:        "m1",
		Role:      "assistant",
		Content:   "hello",
		Timestamp: "2026-05-14T00:00:00Z",
	}}

	out := mapMessageResponses(in)
	if len(out) != 1 {
		t.Fatalf("len(out) = %d, want 1", len(out))
	}
	if out[0].ID != "m1" || out[0].Role != "assistant" || out[0].Content != "hello" || out[0].Timestamp == "" {
		t.Fatalf("unexpected mapped message: %+v", out[0])
	}

	in[0].Content = "mutated"
	if out[0].Content != "hello" {
		t.Fatalf("response reused input backing data: %+v", out[0])
	}
}

func TestMapSendMessageRequestMapsIngressDTOToRuntimeRequest(t *testing.T) {
	got := mapSendMessageRequest("session-1", sendMessageRequest{Content: "hello"})
	if got.SessionID != "session-1" || got.Content != "hello" {
		t.Fatalf("post request = %+v", got)
	}
}

func TestMapActivityResponseCopiesBoundaryShape(t *testing.T) {
	in := activity.Record{
		ID:        "a1",
		SessionID: "s1",
		Type:      "tool_start",
		Timestamp: time.Now().UTC(),
		Data:      json.RawMessage(`{"name":"fs"}`),
	}

	out := mapActivityResponse(in)
	if out.ID != "a1" || out.SessionID != "s1" || out.Type != "tool_start" || string(out.Data) != `{"name":"fs"}` {
		t.Fatalf("unexpected mapped activity: %+v", out)
	}

	in.Data[0] = '['
	if string(out.Data) != `{"name":"fs"}` {
		t.Fatalf("response reused input backing data: %+v", out)
	}
}
