package natshub

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

func TestHubStartsAcceptsConnectionAndStops(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "nats"))
	cfg.Client.Port = 0
	cfg.WebSocket.Port = 0
	cfg.Monitoring.Port = reservePort(t)
	cfg.ReadyTimeout = 5 * time.Second
	cfg.NoLog = true

	hub, err := New(cfg)
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}
	if err := hub.Start(context.Background()); err != nil {
		t.Fatalf("start hub: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hub.Stop(stopCtx)
	})
	endpoints := hub.Endpoints()
	if endpoints.ClientURL == "" || !strings.HasPrefix(endpoints.ClientURL, "nats://") {
		t.Fatalf("client url = %q", endpoints.ClientURL)
	}
	if endpoints.WebSocketURL == "" || !strings.HasPrefix(endpoints.WebSocketURL, "ws://") {
		t.Fatalf("websocket url = %q", endpoints.WebSocketURL)
	}
	if endpoints.MonitoringURL == "" || !strings.HasPrefix(endpoints.MonitoringURL, "http://") {
		t.Fatalf("monitoring url = %q", endpoints.MonitoringURL)
	}
	if endpoints.JetStreamDir != filepath.Join(cfg.StateDir, "jetstream") {
		t.Fatalf("jetstream dir = %q", endpoints.JetStreamDir)
	}
	if hub.ServerForTest() == nil || !hub.ServerForTest().JetStreamEnabled() {
		t.Fatal("embedded server did not enable jetstream")
	}
	nc, err := nats.Connect(
		endpoints.ClientURL,
		nats.UserInfo(DefaultControlUser, DefaultControlPassword),
		nats.Timeout(2*time.Second),
	)
	if err != nil {
		t.Fatalf("connect to embedded nats: %v", err)
	}
	nc.Close()

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	if err := hub.Stop(stopCtx); err != nil {
		t.Fatalf("stop hub: %v", err)
	}
	if hub.ServerForTest() != nil {
		t.Fatal("server pointer was not cleared")
	}
}

func TestHubProvisionsControlStreamsAndCoordinationBuckets(t *testing.T) {
	hub := startTestHub(t)
	control, err := hub.ControlCredential()
	if err != nil {
		t.Fatalf("control credential: %v", err)
	}
	nc := connectWithCredential(t, hub, control)
	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("jetstream: %v", err)
	}
	for _, stream := range []string{StreamAudit, StreamTelemetry, StreamSessionEvents, StreamRuntimeActivity, StreamWorkflowEvents, StreamCatalog, StreamJobs} {
		info, err := js.StreamInfo(stream)
		if err != nil {
			t.Fatalf("stream %s missing: %v", stream, err)
		}
		if len(info.Config.Subjects) == 0 || info.Config.Storage != nats.FileStorage {
			t.Fatalf("stream %s config incomplete: %+v", stream, info.Config)
		}
	}
	for _, bucket := range []string{KVRuntimeSpaceLeases, KVServiceLeases, KVRunStateLeases, KVAccountMetadata, KVCatalogCursors} {
		kv, err := js.KeyValue(bucket)
		if err != nil {
			t.Fatalf("kv bucket %s missing: %v", bucket, err)
		}
		if _, err := kv.Put("probe", []byte("ok")); err != nil {
			t.Fatalf("write kv bucket %s: %v", bucket, err)
		}
	}
	objects, err := js.ObjectStore(ObjectArtifactHandoff)
	if err != nil {
		t.Fatalf("artifact handoff object store missing: %v", err)
	}
	objectStream, err := js.StreamInfo("OBJ_" + ObjectArtifactHandoff)
	if err != nil {
		t.Fatalf("artifact handoff backing stream missing: %v", err)
	}
	if objectStream.Config.MaxBytes != defaultArtifactHandoffMaxBytes {
		t.Fatalf("artifact handoff max bytes = %d", objectStream.Config.MaxBytes)
	}
	if _, err := objects.PutBytes("probe", []byte("large-payload-ref")); err != nil {
		t.Fatalf("write artifact handoff object: %v", err)
	}
	if _, err := js.Publish("audit.space_1.service_calls.svc_ref_probe", []byte(`{"type":"service_call"}`)); err != nil {
		t.Fatalf("publish audit event: %v", err)
	}
	replay, err := js.SubscribeSync("audit.space_1.service_calls.>", nats.BindStream(StreamAudit), nats.DeliverAll())
	if err != nil {
		t.Fatalf("subscribe audit replay: %v", err)
	}
	t.Cleanup(func() { _ = replay.Unsubscribe() })
	msg, err := replay.NextMsg(time.Second)
	if err != nil {
		t.Fatalf("replay audit event: %v", err)
	}
	if string(msg.Data) == "" {
		t.Fatal("replayed audit event was empty")
	}
	info, err := js.StreamInfo(StreamAudit)
	if err != nil {
		t.Fatalf("audit stream info: %v", err)
	}
	if info.State.Msgs == 0 {
		t.Fatal("audit stream did not store published event")
	}
}

func TestHubStartFailsWhenStateDirIsFile(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "nats-state")
	if err := os.WriteFile(statePath, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("write state file: %v", err)
	}
	cfg := DefaultConfig(statePath)
	cfg.Client.Port = 0
	cfg.WebSocket.Port = 0
	cfg.Monitoring.Port = 0
	cfg.NoLog = true
	hub, err := New(cfg)
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}
	if err := hub.Start(context.Background()); err == nil {
		t.Fatal("expected state dir startup failure")
	}
}

func TestExternalModeDoesNotStartEmbeddedServer(t *testing.T) {
	hub, err := New(Config{
		Mode:        ModeExternal,
		ExternalURL: "nats://example.invalid:4222",
		Accounts:    DefaultAccounts(),
	})
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}
	if err := hub.Start(context.Background()); err != nil {
		t.Fatalf("start external hub: %v", err)
	}
	endpoints := hub.Endpoints()
	if endpoints.ClientURL != "nats://example.invalid:4222" {
		t.Fatalf("client url = %q", endpoints.ClientURL)
	}
	if hub.ServerForTest() != nil {
		t.Fatal("external mode started embedded server")
	}
	if err := hub.Stop(context.Background()); err != nil {
		t.Fatalf("stop external hub: %v", err)
	}
}

func TestHubRejectsDoubleStart(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "nats"))
	cfg.Client.Port = 0
	cfg.WebSocket.Enabled = false
	cfg.Monitoring.Enabled = false
	cfg.NoLog = true
	hub, err := New(cfg)
	if err != nil {
		t.Fatalf("new hub: %v", err)
	}
	if err := hub.Start(context.Background()); err != nil {
		t.Fatalf("start hub: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hub.Stop(ctx)
	})
	if err := hub.Start(context.Background()); err == nil {
		t.Fatal("expected double start error")
	}
}

func reservePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}
