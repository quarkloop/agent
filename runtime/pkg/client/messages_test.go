package rtclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPostSessionMessageStreamsTypedEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/sessions/chat-1/messages" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		var req postSessionMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Content != "hello" {
			t.Fatalf("content = %q, want hello", req.Content)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: token\ndata: \"hi\"\n\n"))
		_, _ = w.Write([]byte("event: tool_start\ndata: {\"name\":\"fs\"}\n\n"))
	}))
	defer server.Close()

	client := New(server.URL, WithTransport(NewTransport(server.URL, WithHTTPClient(server.Client()))))
	var events []SSEEvent
	err := client.PostSessionMessage(context.Background(), "chat-1", "hello", func(event SSEEvent) error {
		events = append(events, event)
		return nil
	})
	if err != nil {
		t.Fatalf("PostSessionMessage returned error: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2", len(events))
	}
	if events[0].Type != "token" || string(events[0].Data) != `"hi"` {
		t.Fatalf("unexpected token event: %+v", events[0])
	}
	if events[1].Type != "tool_start" || string(events[1].Data) != `{"name":"fs"}` {
		t.Fatalf("unexpected tool event: %+v", events[1])
	}
}

func TestListSessionMessagesMapsNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
	}))
	defer server.Close()

	client := New(server.URL, WithTransport(NewTransport(server.URL, WithHTTPClient(server.Client()))))
	_, err := client.ListSessionMessages(context.Background(), "missing")
	if !IsNotFound(err) {
		t.Fatalf("IsNotFound = false for %v", err)
	}
}
