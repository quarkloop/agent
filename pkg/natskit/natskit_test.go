package natskit

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
)

func TestRequestEnvelopeDoesNotDuplicateOperationRoute(t *testing.T) {
	req, err := NewRequest("call-1", "space-1", ActorRuntime, json.RawMessage(`{"value":"test"}`))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{`"service"`, `"function"`, `"subject"`} {
		if strings.Contains(string(data), prohibited) {
			t.Fatalf("request repeats routing identity %s: %s", prohibited, data)
		}
	}
}

func TestHostCallUsesSubjectAndResponderOwnedQueue(t *testing.T) {
	server := startNATS(t, false)
	host, err := newHost(context.Background(), Config{URL: server.ClientURL(), Name: "host-test"}, "q.echo.v1")
	if err != nil {
		t.Fatalf("new host: %v", err)
	}
	t.Cleanup(host.Close)
	operation, err := ServiceOperation("echo", "get")
	if err != nil {
		t.Fatal(err)
	}
	if err := host.RegisterUnary(operation, time.Second, func(_ context.Context, req RequestEnvelope) (ResponseEnvelope, error) {
		return OKResponse(req.ServiceCallID, json.RawMessage(`{"ok":true}`)), nil
	}); err != nil {
		t.Fatalf("register operation: %v", err)
	}
	if err := host.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	client, err := Connect(context.Background(), Config{URL: server.ClientURL(), Name: "caller-test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	req, _ := NewRequest("call-echo", "space-1", ActorRuntime, json.RawMessage(`{}`))
	resp, err := client.Call(context.Background(), operation, req)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if resp.ReferenceID != ReferenceIDForServiceCall("call-echo") || resp.Status != StatusOK {
		t.Fatalf("response = %+v", resp)
	}
}

func TestOperationRejectsMetadataThatDisagreesWithSubject(t *testing.T) {
	server := startNATS(t, false)
	host, err := newHost(context.Background(), Config{URL: server.ClientURL(), Name: "metadata-host"}, "q.echo.v1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(host.Close)
	err = host.RegisterUnary(Operation{Owner: "other", Function: "get", Subject: "svc.echo.v1.get"}, time.Second, func(_ context.Context, req RequestEnvelope) (ResponseEnvelope, error) {
		return OKResponse(req.ServiceCallID, json.RawMessage(`{}`)), nil
	})
	if err == nil {
		t.Fatal("registered operation with duplicated, conflicting route identity")
	}
}

func TestRPCDescriptorSubjectIsTheRegistrationAuthority(t *testing.T) {
	operation, err := operationForRPC(&servicev1.ServiceDescriptor{Name: "wrong-owner"}, &servicev1.RpcDescriptor{
		Owner:        "wrong-owner",
		FunctionName: "wrong_Function",
		Subject:      "svc.gateway.v1.embed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if operation.Subject != "svc.gateway.v1.embed" || operation.Owner != "gateway" || operation.Function != "embed" {
		t.Fatalf("operation = %+v", operation)
	}
}

func TestMustServiceRPCUsesCanonicalOperationAuthority(t *testing.T) {
	rpc := MustServiceRPC("devops", "repo_Status", "quark.devops.v1.RepoService", "Status", "request", "response", "description")
	if rpc.GetOwner() != "devops" || rpc.GetFunctionName() != "repo_Status" || rpc.GetSubject() != "svc.devops.v1.repo_status" {
		t.Fatalf("rpc descriptor = %+v", rpc)
	}
	stream := MustStreamingServiceRPC("workflow", "workflow_StreamEvents", "quark.workflow.v1.WorkflowService", "StreamEvents", "request", "response", "description")
	if !stream.GetStreaming() || stream.GetSubject() != "svc.workflow.v1.stream_events" {
		t.Fatalf("stream descriptor = %+v", stream)
	}
	queue, err := ServiceQueueGroup("workflow")
	if err != nil || queue != "q.service.v1.workflow" {
		t.Fatalf("service queue = %q, err = %v", queue, err)
	}
}

func TestServiceHostRejectsMixedOwnerBindings(t *testing.T) {
	_, err := serviceQueueForBindings([]Binding{
		{Descriptor: &servicev1.ServiceDescriptor{Rpcs: []*servicev1.RpcDescriptor{MustServiceRPC("gateway", "gateway_Embed", "gateway", "Embed", "request", "response", "description")}}},
		{Descriptor: &servicev1.ServiceDescriptor{Rpcs: []*servicev1.RpcDescriptor{MustServiceRPC("indexer", "indexer_QueryContext", "indexer", "QueryContext", "request", "response", "description")}}},
	})
	if err == nil || !strings.Contains(err.Error(), "cannot register operations") {
		t.Fatalf("mixed owners error = %v", err)
	}
}

func TestCallReportsNoResponder(t *testing.T) {
	server := startNATS(t, false)
	client, err := Connect(context.Background(), Config{URL: server.ClientURL(), Name: "no-responder-client"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	operation, _ := ServiceOperation("missing", "get")
	req, _ := NewRequest("call-missing", "space-1", ActorRuntime, json.RawMessage(`{}`))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := client.Call(ctx, operation, req); !errors.Is(err, natsgo.ErrNoResponders) {
		t.Fatalf("no responder error = %v", err)
	}
}

func TestSubscriptionWildcardsAreResponderOnly(t *testing.T) {
	server := startNATS(t, false)
	client, err := Connect(context.Background(), Config{URL: server.ClientURL(), Name: "wildcard-test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	sub, err := client.Respond("session.*.input", "q.runtime.sessions", time.Second, func(_ context.Context, _ Message) ([]byte, error) {
		return []byte(`{"status":"ok"}`), nil
	})
	if err != nil {
		t.Fatalf("register wildcard responder: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	if err := client.Publish(context.Background(), "session.*.input", []byte(`{}`), nil); err == nil {
		t.Fatal("publisher accepted wildcard destination")
	}
}

func TestApplicationHostRegistersRequestReplyAndPublishesLiveEvent(t *testing.T) {
	server := startNATS(t, false)
	host, err := NewApplicationHost(context.Background(), Config{URL: server.ClientURL(), Name: "runtime-host"}, "q.runtime.test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(host.Close)
	input, err := NewApplicationOperation("session.input", "session.*.input")
	if err != nil {
		t.Fatal(err)
	}
	if err := host.Register(input, func(_ context.Context, msg Message) ([]byte, error) {
		return append([]byte("accepted:"), msg.Data...), nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := host.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	client, err := Connect(context.Background(), Config{URL: server.ClientURL(), Name: "session-client"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	reply, err := client.Request(context.Background(), "session.chat.input", []byte("hello"), nil)
	if err != nil || string(reply) != "accepted:hello" {
		t.Fatalf("reply = %q, err = %v", reply, err)
	}

	received := make(chan Message, 1)
	sub, err := client.Subscribe("session.chat.events", func(msg Message) { received <- msg })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	if err := client.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	event, err := NewApplicationEvent("session.events", "session.chat.events")
	if err != nil {
		t.Fatal(err)
	}
	if err := host.Publish(context.Background(), event, []byte("token"), map[string]string{HeaderSessionID: "chat"}); err != nil {
		t.Fatal(err)
	}
	select {
	case msg := <-received:
		if string(msg.Data) != "token" || msg.Headers[HeaderSessionID] != "chat" {
			t.Fatalf("event = %+v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for application event")
	}
	host.Close()
	requestCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if reply, err := client.Request(requestCtx, "session.chat.input", []byte("again"), nil); err == nil {
		t.Fatalf("request was processed after host drain: %q", reply)
	}
}

func TestApplicationHostRejectsWildcardEventDestination(t *testing.T) {
	if _, err := NewApplicationEvent("session.events", "session.*.events"); err == nil {
		t.Fatal("application event accepted wildcard destination")
	}
}

func TestApplicationHostDurableEventWaitsForJetStreamStorage(t *testing.T) {
	server := startNATS(t, true)
	conn, err := natsgo.Connect(server.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(conn.Close)
	js, err := conn.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := js.AddStream(&natsgo.StreamConfig{Name: "SESSIONS", Subjects: []string{"session.*.events"}}); err != nil {
		t.Fatal(err)
	}
	host, err := NewApplicationHost(context.Background(), Config{URL: server.ClientURL(), Name: "durable-runtime"}, "q.runtime.test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(host.Close)
	event, err := NewDurableApplicationEvent("session.events", "session.chat.events")
	if err != nil {
		t.Fatal(err)
	}
	if err := host.Publish(context.Background(), event, []byte("stored"), nil); err != nil {
		t.Fatal(err)
	}
	msg, err := js.GetLastMsg("SESSIONS", "session.chat.events")
	if err != nil || string(msg.Data) != "stored" {
		t.Fatalf("stored event = %q, err = %v", msg.Data, err)
	}
}

func TestApplicationHostRestoresResponderAfterReconnect(t *testing.T) {
	server := startNATS(t, false)
	port := server.Addr().(*net.TCPAddr).Port
	config := Config{
		URL:           server.ClientURL(),
		Name:          "reconnect-runtime",
		ReconnectWait: 10 * time.Millisecond,
		MaxReconnects: -1,
		Timeout:       time.Second,
	}
	host, err := NewApplicationHost(context.Background(), config, "q.runtime.reconnect")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(host.Close)
	operation, _ := NewApplicationOperation("runtime.info.get", "runtime.info.v1.get")
	if err := host.Register(operation, func(_ context.Context, _ Message) ([]byte, error) {
		return []byte("ready"), nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := host.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	clientConfig := config
	clientConfig.Name = "reconnect-client"
	client, err := Connect(context.Background(), clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)

	server.Shutdown()
	server.WaitForShutdown()
	replacement, err := natsserver.NewServer(&natsserver.Options{Host: "127.0.0.1", Port: port, NoLog: true, NoSigs: true})
	if err != nil {
		t.Fatal(err)
	}
	replacement.Start()
	if !replacement.ReadyForConnections(time.Second) {
		t.Fatal("replacement nats server not ready")
	}
	t.Cleanup(replacement.Shutdown)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		reply, requestErr := client.Request(ctx, operation.Subject, []byte("{}"), nil)
		cancel()
		if requestErr == nil && string(reply) == "ready" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("application responder did not recover after reconnect")
}

func TestHostReturnsStructuredErrorForMalformedRequest(t *testing.T) {
	server := startNATS(t, false)
	host, err := newHost(context.Background(), Config{URL: server.ClientURL(), Name: "malformed-host"}, "q.malformed.v1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(host.Close)
	operation, _ := ServiceOperation("document", "parse")
	called := false
	if err := host.RegisterUnary(operation, time.Second, func(_ context.Context, req RequestEnvelope) (ResponseEnvelope, error) {
		called = true
		return OKResponse(req.ServiceCallID, json.RawMessage(`{}`)), nil
	}); err != nil {
		t.Fatal(err)
	}
	_ = host.Ready(context.Background())
	client, _ := Connect(context.Background(), Config{URL: server.ClientURL(), Name: "malformed-client"})
	t.Cleanup(client.Close)
	data, err := client.Request(context.Background(), operation.Subject, []byte(`{invalid`), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := DecodeServiceResponse(data)
	if err != nil || resp.Status != StatusError || called {
		t.Fatalf("malformed response = %+v, called = %v, err = %v", resp, called, err)
	}
}

func TestCorePublishSubscribeDoesNotReplayMessagesBeforeSubscription(t *testing.T) {
	server := startNATS(t, false)
	client, err := Connect(context.Background(), Config{URL: server.ClientURL(), Name: "pubsub-client"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	if err := client.Publish(context.Background(), "session.space_1.events", []byte("before"), nil); err != nil {
		t.Fatal(err)
	}
	received := make(chan string, 1)
	sub, err := client.Subscribe("session.space_1.events", func(msg Message) {
		received <- string(msg.Data)
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	if err := client.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := client.Publish(context.Background(), "session.space_1.events", []byte("after"), nil); err != nil {
		t.Fatal(err)
	}
	select {
	case value := <-received:
		if value != "after" {
			t.Fatalf("received replayed Core NATS value %q", value)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for live published message")
	}
}

func TestStreamingServiceMarksOnlyTerminalResponseFinal(t *testing.T) {
	server := startNATS(t, false)
	host, err := newHost(context.Background(), Config{URL: server.ClientURL(), Name: "stream-host"}, "q.stream.v1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(host.Close)
	operation, _ := ServiceOperation("gateway", "stream_generate")
	if err := host.RegisterStream(operation, time.Second, func(_ context.Context, req RequestEnvelope, publish func(ResponseEnvelope) error) (ResponseEnvelope, error) {
		if err := publish(OKResponse(req.ServiceCallID, json.RawMessage(`{"delta":"one"}`))); err != nil {
			return ResponseEnvelope{}, err
		}
		return OKResponse(req.ServiceCallID, json.RawMessage(`{"done":true}`)), nil
	}); err != nil {
		t.Fatal(err)
	}
	_ = host.Ready(context.Background())
	client, _ := Connect(context.Background(), Config{URL: server.ClientURL(), Name: "stream-client"})
	t.Cleanup(client.Close)
	req, _ := NewRequest("call-stream", "space-1", ActorRuntime, json.RawMessage(`{}`))
	stream, err := client.OpenServiceStream(context.Background(), operation, req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stream.Close() })
	firstData, _ := stream.Next(context.Background())
	first, err := DecodeServiceResponse(firstData)
	if err != nil || first.Final {
		t.Fatalf("first stream response = %+v, err = %v", first, err)
	}
	finalData, _ := stream.Next(context.Background())
	final, err := DecodeServiceResponse(finalData)
	if err != nil || !final.Final {
		t.Fatalf("terminal stream response = %+v, err = %v", final, err)
	}
}

func TestStreamingServiceRefreshesIdleTimeoutOnProgress(t *testing.T) {
	server := startNATS(t, false)
	host, err := newHost(context.Background(), Config{URL: server.ClientURL(), Name: "stream-idle-host"}, "q.stream.v1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(host.Close)
	operation, _ := ServiceOperation("gateway", "stream_generate")
	const idleTimeout = 35 * time.Millisecond
	if err := host.RegisterStream(operation, idleTimeout, func(ctx context.Context, req RequestEnvelope, publish func(ResponseEnvelope) error) (ResponseEnvelope, error) {
		for i := 0; i < 4; i++ {
			select {
			case <-time.After(20 * time.Millisecond):
			case <-ctx.Done():
				return ResponseEnvelope{}, ctx.Err()
			}
			if err := publish(OKResponse(req.ServiceCallID, json.RawMessage(`{"delta":"progress"}`))); err != nil {
				return ResponseEnvelope{}, err
			}
		}
		return OKResponse(req.ServiceCallID, json.RawMessage(`{"done":true}`)), nil
	}); err != nil {
		t.Fatal(err)
	}
	_ = host.Ready(context.Background())
	client, _ := Connect(context.Background(), Config{URL: server.ClientURL(), Name: "stream-idle-client"})
	t.Cleanup(client.Close)
	req, _ := NewRequest("call-stream-idle", "space-1", ActorRuntime, json.RawMessage(`{}`))
	stream, err := client.OpenServiceStream(context.Background(), operation, req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stream.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for i := 0; i < 4; i++ {
		data, err := stream.Next(ctx)
		if err != nil {
			t.Fatalf("progress response %d: %v", i, err)
		}
		resp, err := DecodeServiceResponse(data)
		if err != nil || resp.Status != StatusOK || resp.Final {
			t.Fatalf("progress response %d = %+v, err = %v", i, resp, err)
		}
	}
	data, err := stream.Next(ctx)
	if err != nil {
		t.Fatal(err)
	}
	final, err := DecodeServiceResponse(data)
	if err != nil || final.Status != StatusOK || !final.Final {
		t.Fatalf("terminal response = %+v, err = %v", final, err)
	}
}

func TestAuditWritesAcknowledgedRecordWithoutContent(t *testing.T) {
	server := startNATS(t, true)
	conn, err := natsgo.Connect(server.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	js, _ := conn.JetStream()
	if _, err := js.AddStream(&natsgo.StreamConfig{Name: "AUDIT", Subjects: []string{"audit.>"}}); err != nil {
		t.Fatal(err)
	}
	host, err := newHost(context.Background(), Config{
		URL:         server.ClientURL(),
		Name:        "audit-host",
		AuditPrefix: "audit",
		AuditPolicy: AuditPolicy{SnapshotPolicy: SnapshotPolicyMetadata},
	}, "q.audit.v1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(host.Close)
	operation, _ := ServiceOperation("audit", "write")
	if err := host.RegisterUnary(operation, time.Second, func(_ context.Context, req RequestEnvelope) (ResponseEnvelope, error) {
		return OKResponse(req.ServiceCallID, json.RawMessage(`{"secret":"output"}`)), nil
	}); err != nil {
		t.Fatal(err)
	}
	_ = host.Ready(context.Background())
	client, _ := Connect(context.Background(), Config{URL: server.ClientURL(), Name: "audit-client"})
	t.Cleanup(client.Close)
	req, _ := NewRequest("call-audit", "space-1", ActorRuntime, json.RawMessage(`{"secret":"input"}`))
	resp, err := client.Call(context.Background(), operation, req)
	if err != nil {
		t.Fatal(err)
	}
	msg, err := js.GetMsg("AUDIT", 1)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(msg.Data), "input") || strings.Contains(string(msg.Data), "output") || !strings.Contains(string(msg.Data), `"content_stored":false`) {
		t.Fatalf("audit data contains content or lacks metadata: %s", msg.Data)
	}
	if resp.ReferenceID == "" {
		t.Fatal("missing reference")
	}
}

func TestAuditRecordCollectionSubjectPreservesWildcardSemantics(t *testing.T) {
	if got, want := ServiceCallRecordsSubject("audit", "Space One"), "audit.space_one.service_calls.>"; got != want {
		t.Fatalf("collection subject = %q, want %q", got, want)
	}
	if got, want := ServiceCallRecordSubject("audit", "Space One", "Ref One"), "audit.space_one.service_calls.ref_one"; got != want {
		t.Fatalf("record subject = %q, want %q", got, want)
	}
}

func TestKeyValueWrapsRevisionAndConflictSemantics(t *testing.T) {
	server := startNATS(t, true)
	client, err := Connect(context.Background(), Config{URL: server.ClientURL(), Name: "kv-client"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	kv, err := client.EnsureKeyValue(KeyValueConfig{Bucket: "leases", TTL: time.Minute, History: 1})
	if err != nil {
		t.Fatal(err)
	}
	revision, err := kv.Create("space", []byte("runtime-1"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := kv.Create("space", []byte("runtime-2")); !errors.Is(err, ErrKeyExists) {
		t.Fatalf("duplicate create error = %v", err)
	}
	entry, err := kv.Get("space")
	if err != nil || entry.Revision() != revision || string(entry.Value()) != "runtime-1" {
		t.Fatalf("entry = %+v, err = %v", entry, err)
	}
	if err := kv.DeleteRevision("space", revision); err != nil {
		t.Fatal(err)
	}
	if _, err := kv.Get("space"); !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("deleted entry error = %v", err)
	}
}

func startNATS(t *testing.T, jetStream bool) *natsserver.Server {
	t.Helper()
	cfg := &natsserver.Options{Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true, JetStream: jetStream}
	if jetStream {
		cfg.StoreDir = t.TempDir()
	}
	server, err := natsserver.NewServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	go server.Start()
	if !server.ReadyForConnections(time.Second) {
		t.Fatal("nats server not ready")
	}
	t.Cleanup(server.Shutdown)
	return server
}
