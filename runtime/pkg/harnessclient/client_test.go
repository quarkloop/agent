package harnessclient

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/pkg/plugin"
	harnessv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/harness/v1"
	"github.com/quarkloop/runtime/pkg/runcontext"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestRustHarnessConformsToNATSKitUnaryAndStreamingContracts(t *testing.T) {
	ns := startHarnessTestNATS(t)
	binary := buildRustHarness(t)
	var processLog bytes.Buffer
	cmd := exec.Command(binary, "--root", t.TempDir(), "--nats-url", ns.ClientURL())
	cmd.Stdout = &processLog
	cmd.Stderr = &processLog
	if err := cmd.Start(); err != nil {
		t.Fatalf("start Rust Harness: %v", err)
	}
	running := true
	t.Cleanup(func() {
		if running {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	ctx := runcontext.WithSessionID(runcontext.WithSpaceID(context.Background(), "space-a"), "session-a")
	client := New(Config{URL: ns.ClientURL(), SpaceID: "space-a", Timeout: 250 * time.Millisecond})
	t.Cleanup(client.Close)
	var messages []plugin.Message
	deadline := time.Now().Add(10 * time.Second)
	for {
		var err error
		messages, err = client.Compose(ctx, Input{
			Materials: []Material{
				{SourceID: "plugin.agent.main.system", SourceKind: "agent", Content: "Use evidence.", Required: true},
				{SourceID: "mem-preference", SourceKind: "memory", Content: "Use short paragraphs."},
			},
			History: []plugin.Message{{Role: "user", Content: "Summarize this."}},
		})
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("Rust Harness did not become responsive: %v\n%s", err, processLog.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(messages) != 2 || messages[0].Content != "Use evidence.\n\nUse short paragraphs." || messages[1].Content != "Summarize this." {
		t.Fatalf("composed messages = %+v", messages)
	}

	nc, err := natskit.Connect(context.Background(), natskit.Config{URL: ns.ClientURL(), Name: "harness-go-conformance"})
	if err != nil {
		t.Fatalf("connect Go NATSKit: %v", err)
	}
	t.Cleanup(nc.Close)
	streamPayload, err := protojson.Marshal(&harnessv1.StreamContextReportsRequest{Space: "space-a", SessionId: "session-a", Limit: 5})
	if err != nil {
		t.Fatalf("marshal stream payload: %v", err)
	}
	streamReq, err := natskit.NewRequest("call-harness-stream", "space-a", natskit.ActorRuntime, json.RawMessage(streamPayload))
	if err != nil {
		t.Fatalf("new stream request: %v", err)
	}
	streamOp, _ := natskit.ServiceOperation("harness", "stream_context_reports")
	stream, err := nc.OpenServiceStream(context.Background(), streamOp, streamReq)
	if err != nil {
		t.Fatalf("open Harness stream: %v", err)
	}
	t.Cleanup(func() { _ = stream.Close() })
	eventData, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("read context report: %v", err)
	}
	event, err := natskit.DecodeServiceResponse(eventData)
	if err != nil {
		t.Fatalf("decode context report envelope: %v", err)
	}
	if event.Final || event.ReferenceID != natskit.ReferenceIDForServiceCall("call-harness-stream") {
		t.Fatalf("context report envelope = %+v", event)
	}
	var report harnessv1.ContextReport
	if err := protojson.Unmarshal(event.Payload, &report); err != nil || report.GetSessionId() != "session-a" ||
		len(report.GetIncludedMemoryIds()) != 1 || report.GetIncludedMemoryIds()[0] != "mem-preference" {
		t.Fatalf("context report payload = %+v, error = %v", &report, err)
	}
	terminalData, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("read terminal stream envelope: %v", err)
	}
	terminal, err := natskit.DecodeServiceResponse(terminalData)
	if err != nil || !terminal.Final || terminal.AuditRef != natskit.AuditRefForReference(terminal.ReferenceID) {
		t.Fatalf("terminal envelope = %+v, error = %v", terminal, err)
	}

	missingPayload, _ := protojson.Marshal(&harnessv1.GetMemoryRequest{Space: "space-a", Scope: "session", Key: "missing"})
	missingReq, _ := natskit.NewRequest("call-harness-error", "space-a", natskit.ActorRuntime, json.RawMessage(missingPayload))
	missingOp, _ := natskit.ServiceOperation("harness", "get_memory")
	missing, err := nc.Call(context.Background(), missingOp, missingReq)
	if err != nil {
		t.Fatalf("call missing memory: %v", err)
	}
	if missing.Status != natskit.StatusError || missing.Error == nil || missing.Error.Category != "invalid_argument" {
		t.Fatalf("error envelope = %+v", missing)
	}

	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatalf("interrupt Rust Harness: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("Rust Harness graceful shutdown: %v\n%s", err, processLog.String())
	}
	running = false
}

func TestMessagesPreserveStructuredToolExecutionHistory(t *testing.T) {
	input := []plugin.Message{
		{
			Role: "assistant",
			ToolCalls: []plugin.ToolCall{{
				Index: 2,
				ID:    "call-embed",
				Type:  "function",
				Function: plugin.ToolCallFunction{
					Name:      "gateway_Embed",
					Arguments: `{"inputs":[{"content":"source text"}]}`,
				},
			}},
		},
		{Role: "tool", Content: `{"embeddingRef":"embedding:1"}`, ToolCallID: "call-embed"},
	}

	roundTrip := messagesFromProto(messagesToProto(input))
	if len(roundTrip) != len(input) {
		t.Fatalf("messages = %d, want %d", len(roundTrip), len(input))
	}
	gotCall := roundTrip[0].ToolCalls
	if len(gotCall) != 1 || gotCall[0].ID != "call-embed" ||
		gotCall[0].Index != 2 || gotCall[0].Type != "function" ||
		gotCall[0].Function.Name != "gateway_Embed" ||
		gotCall[0].Function.Arguments != `{"inputs":[{"content":"source text"}]}` {
		t.Fatalf("assistant tool calls = %+v", gotCall)
	}
	if roundTrip[1].ToolCallID != "call-embed" {
		t.Fatalf("tool call id = %q, want %q", roundTrip[1].ToolCallID, "call-embed")
	}
}

func buildRustHarness(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	manifest := filepath.Join(root, "services", "harness", "Cargo.toml")
	cmd := exec.Command("cargo", "build", "--quiet", "--manifest-path", manifest)
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build Rust Harness: %v\n%s", err, output)
	}
	return filepath.Join(root, "services", "harness", "target", "debug", "quark-harness")
}

func startHarnessTestNATS(t *testing.T) *natsserver.Server {
	t.Helper()
	ns, err := natsserver.NewServer(&natsserver.Options{Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true})
	if err != nil {
		t.Fatalf("new NATS server: %v", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(time.Second) {
		ns.Shutdown()
		t.Fatal("NATS server did not become ready")
	}
	t.Cleanup(ns.Shutdown)
	return ns
}
