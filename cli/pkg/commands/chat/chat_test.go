package chatcmd

import (
	"bytes"
	"testing"

	"github.com/quarkloop/pkg/serviceapi/clientcontract"
)

func TestPrintEventWritesTokensAndToolProgressSeparately(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := printEvent(&stdout, &stderr, clientcontract.SessionEvent{Type: "token", Payload: []byte(`"hello"`)}, true); err != nil {
		t.Fatalf("token event returned error: %v", err)
	}
	if err := printEvent(&stdout, &stderr, clientcontract.SessionEvent{Type: "tool_start", Payload: []byte(`{"name":"fs"}`)}, true); err != nil {
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

	err := printEvent(&stdout, &stderr, clientcontract.SessionEvent{Type: "error", Payload: []byte(`"boom"`)}, true)
	if err == nil {
		t.Fatal("expected agent error")
	}
}
