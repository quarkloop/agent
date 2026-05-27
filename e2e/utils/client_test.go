//go:build e2e

package utils

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadMessageTraceParsesTokensAndTools(t *testing.T) {
	stream := strings.NewReader(strings.Join([]string{
		"event: token",
		`data: "hello"`,
		"",
		"event: tool_start",
		`data: {"id":"call-1","name":"gateway_Embed","arguments":"{}"}`,
		"",
		"event: tool_result",
		`data: {"id":"call-1","name":"gateway_Embed","result":"{\"embeddingRef\":\"ref\"}","error":false}`,
		"",
		"event: progress",
		`data: {"phase":"tool_call_validation","state":"rejected","function":"indexer_UpsertChunk","reason":"incomplete_canonical_record","diagnostic":"missing facts"}`,
		"",
	}, "\n"))

	trace, err := readMessageTrace(context.Background(), stream, MessageTraceOptions{
		Label:       "unit trace",
		IdleTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	if !trace.Completed {
		t.Fatal("trace did not complete")
	}
	if trace.Text != "hello" {
		t.Fatalf("text = %q, want hello", trace.Text)
	}
	if len(trace.ToolStarts) != 1 || trace.ToolStarts[0] != "gateway_Embed" {
		t.Fatalf("tool starts = %v", trace.ToolStarts)
	}
	if trace.ToolStartEvents[0].CallID != "call-1" {
		t.Fatalf("tool start call id = %q", trace.ToolStartEvents[0].CallID)
	}
	if len(trace.ToolResults) != 1 || trace.ToolResults[0] != "gateway_Embed" {
		t.Fatalf("tool results = %v", trace.ToolResults)
	}
	if trace.ToolResultEvents[0].CallID != "call-1" || trace.ToolResultEvents[0].Error {
		t.Fatalf("tool result event = %+v", trace.ToolResultEvents[0])
	}
	if len(trace.ProgressEvents) != 1 || trace.ProgressEvents[0].Reason != "incomplete_canonical_record" ||
		trace.ProgressEvents[0].Diagnostic != "missing facts" {
		t.Fatalf("progress events = %+v", trace.ProgressEvents)
	}
}

func TestReadMessageTraceIdleTimeoutIncludesDiagnostics(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()

	_, err := readMessageTrace(context.Background(), reader, MessageTraceOptions{
		Label:       "idle unit trace",
		Prompt:      "index these PDFs",
		IdleTimeout: 20 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected idle timeout")
	}
	msg := err.Error()
	for _, want := range []string{"idle timeout", "idle unit trace", "index these PDFs"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
}

func TestWriteMessageFailureTraceArtifactPreservesProgress(t *testing.T) {
	dir := t.TempDir()
	writeMessageFailureTraceArtifact(t, dir, MessageTrace{
		SessionID: "session-1",
		ProgressEvents: []ProgressEvent{{
			Phase: "tool_call_validation", State: "rejected", Reason: "invalid_arguments",
		}},
	}, MessageTraceOptions{Label: "index", Prompt: "index these files"}, io.EOF)
	data, err := os.ReadFile(filepath.Join(dir, "message-failure-trace.json"))
	if err != nil {
		t.Fatalf("read failure trace: %v", err)
	}
	if !strings.Contains(string(data), "invalid_arguments") || !strings.Contains(string(data), "session-1") {
		t.Fatalf("failure trace missing diagnostics: %s", data)
	}
}
