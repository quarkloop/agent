package natshub

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/natskit"
)

func TestHubStartsAcceptsConnectionAndStops(t *testing.T) {
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "nats"))
	cfg.Client.Port = natsserver.RANDOM_PORT
	cfg.WebSocket.Port = natsserver.RANDOM_PORT
	cfg.Monitoring.Port = natsserver.RANDOM_PORT
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
	for _, stream := range []string{StreamAudit, StreamTelemetry, StreamCatalog} {
		info, err := js.StreamInfo(stream)
		if err != nil {
			t.Fatalf("stream %s missing: %v", stream, err)
		}
		if len(info.Config.Subjects) == 0 || info.Config.Storage != nats.FileStorage {
			t.Fatalf("stream %s config incomplete: %+v", stream, info.Config)
		}
	}
	for _, bucket := range []string{KVRunStateLeases} {
		kv, err := js.KeyValue(bucket)
		if err != nil {
			t.Fatalf("kv bucket %s missing: %v", bucket, err)
		}
		if _, err := kv.Put("probe", []byte("ok")); err != nil {
			t.Fatalf("write kv bucket %s: %v", bucket, err)
		}
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
	if !info.Config.AllowDirect {
		t.Fatal("audit stream does not permit indexed direct retrieval")
	}
}

func TestHubProvisionsReplayableRuntimeStreamsInsideSpaceAccount(t *testing.T) {
	hub := startTestHub(t)
	space, err := hub.ProvisionSpace("docs")
	if err != nil {
		t.Fatalf("provision space: %v", err)
	}
	nc := connectWithCredential(t, hub, space.Runtime)
	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("jetstream: %v", err)
	}
	for _, stream := range []string{StreamSessionEvents, StreamRuntimeActivity} {
		info, err := js.StreamInfo(stream)
		if err != nil {
			t.Fatalf("space stream %s missing: %v", stream, err)
		}
		if stream == StreamRuntimeActivity && containsSubject(info.Config.Subjects, "runtime.activity.v1.>") {
			t.Fatal("runtime activity stream captures request/reply operations")
		}
	}
	kv, err := js.KeyValue(KVRuntimeSpaceLeases)
	if err != nil {
		t.Fatalf("space runtime lease bucket missing: %v", err)
	}
	if _, err := kv.Create("docs", []byte("runtime-a")); err != nil {
		t.Fatalf("claim space runtime lease: %v", err)
	}
	if _, err := js.Publish("session.chat.events", []byte(`{"type":"token"}`)); err != nil {
		t.Fatalf("persist session event: %v", err)
	}
	stored, err := js.GetLastMsg(StreamSessionEvents, "session.chat.events")
	if err != nil || len(stored.Data) == 0 {
		t.Fatalf("stored event = %+v, err = %v", stored, err)
	}
	control, err := hub.ControlCredential()
	if err != nil {
		t.Fatal(err)
	}
	controlJS, err := connectWithCredential(t, hub, control).JetStream()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controlJS.StreamInfo(StreamSessionEvents); err == nil {
		t.Fatal("control account unexpectedly owns a space-scoped runtime event stream")
	}
	if _, err := controlJS.KeyValue(KVRuntimeSpaceLeases); err == nil {
		t.Fatal("control account unexpectedly owns a space-scoped runtime lease bucket")
	}
}

func TestRunStateServiceCredentialCanUseItsOwnedControlLeaseBucket(t *testing.T) {
	hub := startTestHub(t)
	route, err := NewServiceFunctionRoute("runstate", "v1", "acquire_lease")
	if err != nil {
		t.Fatal(err)
	}
	credential, err := hub.IssueServiceCredential("runstate", []ServiceFunctionRoute{route})
	if err != nil {
		t.Fatal(err)
	}
	js, err := connectWithCredential(t, hub, credential).JetStream()
	if err != nil {
		t.Fatal(err)
	}
	kv, err := js.KeyValue(KVRunStateLeases)
	if err != nil {
		t.Fatalf("runstate cannot open owned lease bucket: %v", err)
	}
	if _, err := kv.Create("run-1", []byte("owner")); err != nil {
		t.Fatalf("runstate cannot write owned lease: %v", err)
	}
}

func containsSubject(subjects []string, want string) bool {
	for _, subject := range subjects {
		if subject == want {
			return true
		}
	}
	return false
}

func TestHubRetrievesAndFiltersAuditRecords(t *testing.T) {
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
	for _, event := range []natskit.ServiceCallEvent{
		{Type: "service_call", ReferenceID: "ref-one", AuditRef: "ref-one", ServiceCallID: "ref-one", SpaceID: "docs", SessionID: "chat-1", RunID: "run-1", Service: "indexer", Function: "get_context", Status: "ok"},
		{Type: "service_call", ReferenceID: "ref-two", AuditRef: "ref-two", ServiceCallID: "ref-two", SpaceID: "docs", SessionID: "chat-2", RunID: "run-2", Service: "gateway", Function: "generate", Status: "ok"},
	} {
		data, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal event: %v", err)
		}
		if _, err := js.Publish(natskit.ServiceCallRecordSubject("audit", event.SpaceID, event.ReferenceID), data); err != nil {
			t.Fatalf("publish event: %v", err)
		}
	}

	record, err := hub.GetAuditRecord(context.Background(), "docs", "ref-one")
	if err != nil {
		t.Fatalf("get audit record: %v", err)
	}
	if record.ReferenceID != "ref-one" || record.Service != "indexer" {
		t.Fatalf("record = %+v", record)
	}
	page, err := hub.ListAuditRecords(context.Background(), AuditFilter{SpaceID: "docs", Service: "gateway", Limit: 1})
	if err != nil {
		t.Fatalf("list audit records: %v", err)
	}
	if len(page.Records) != 1 || page.Records[0].ReferenceID != "ref-two" || page.NextCursor == 0 {
		t.Fatalf("filtered page = %+v", page)
	}
	secondPage, err := hub.ListAuditRecords(context.Background(), AuditFilter{SpaceID: "docs", Cursor: page.NextCursor, Limit: 1})
	if err != nil {
		t.Fatalf("list next page: %v", err)
	}
	if len(secondPage.Records) != 0 {
		t.Fatalf("next page = %+v", secondPage)
	}
	if _, err := hub.GetAuditRecord(context.Background(), "docs", "missing"); !errors.Is(err, ErrAuditRecordNotFound) {
		t.Fatalf("missing record error = %v", err)
	}
}

func TestHubStartFailsWhenStateDirIsFile(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "nats-state")
	if err := os.WriteFile(statePath, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("write state file: %v", err)
	}
	cfg := DefaultConfig(statePath)
	cfg.Client.Port = natsserver.RANDOM_PORT
	cfg.WebSocket.Port = natsserver.RANDOM_PORT
	cfg.Monitoring.Port = natsserver.RANDOM_PORT
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
	cfg.Client.Port = natsserver.RANDOM_PORT
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
