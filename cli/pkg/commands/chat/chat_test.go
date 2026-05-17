package chatcmd

import (
	"bytes"
	"testing"

	rtclient "github.com/quarkloop/runtime/pkg/client"
)

func TestPrintEventWritesTokensAndToolProgressSeparately(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := printEvent(&stdout, &stderr, rtclient.SSEEvent{Type: "token", Data: []byte(`"hello"`)}, true); err != nil {
		t.Fatalf("token event returned error: %v", err)
	}
	if err := printEvent(&stdout, &stderr, rtclient.SSEEvent{Type: "tool_start", Data: []byte(`{"name":"fs"}`)}, true); err != nil {
		t.Fatalf("tool event returned error: %v", err)
	}

	if stdout.String() != "hello" {
		t.Fatalf("stdout = %q, want hello", stdout.String())
	}
	if stderr.String() != "tool start: fs\n" {
		t.Fatalf("stderr = %q, want tool progress", stderr.String())
	}
}

func TestPrintEventReturnsAgentError(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	err := printEvent(&stdout, &stderr, rtclient.SSEEvent{Type: "error", Data: []byte(`"boom"`)}, true)
	if err == nil {
		t.Fatal("expected agent error")
	}
}
