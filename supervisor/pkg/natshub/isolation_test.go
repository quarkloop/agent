package natshub

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

func TestSpaceAccountsIsolateMatchingSubjects(t *testing.T) {
	hub := startTestHub(t)
	spaceA, err := hub.ProvisionSpace("space-a")
	if err != nil {
		t.Fatalf("provision space a: %v", err)
	}
	spaceB, err := hub.ProvisionSpace("space-b")
	if err != nil {
		t.Fatalf("provision space b: %v", err)
	}
	sessionA, err := hub.IssueSessionCredential("space-a", "shared")
	if err != nil {
		t.Fatalf("issue session credential: %v", err)
	}

	events := make(chan string, 2)
	clientA := connectWithCredential(t, hub, sessionA)
	subscribe(t, clientA, "session.shared.events", events)

	runtimeB := connectWithCredential(t, hub, spaceB.Runtime)
	publish(t, runtimeB, "session.shared.events", "from-b")
	assertNoMessage(t, events)

	runtimeA := connectWithCredential(t, hub, spaceA.Runtime)
	publish(t, runtimeA, "session.shared.events", "from-a")
	assertMessage(t, events, "from-a")
}

func TestSessionCredentialCannotReadAnotherSession(t *testing.T) {
	hub := startTestHub(t)
	space, err := hub.ProvisionSpace("docs")
	if err != nil {
		t.Fatalf("provision space: %v", err)
	}
	sessionOne, err := hub.IssueSessionCredential("docs", "one")
	if err != nil {
		t.Fatalf("issue session one credential: %v", err)
	}
	sessionTwo, err := hub.IssueSessionCredential("docs", "two")
	if err != nil {
		t.Fatalf("issue session two credential: %v", err)
	}

	permissionErrors := make(chan error, 1)
	clientOne := connectWithCredential(
		t,
		hub,
		sessionOne,
		nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
			permissionErrors <- err
		}),
	)
	if _, err := clientOne.Subscribe("session.two.events", func(*nats.Msg) {}); err != nil {
		t.Fatalf("subscribe with session one credential: %v", err)
	}
	clientOne.Flush()
	assertPermissionError(t, permissionErrors)

	events := make(chan string, 1)
	clientTwo := connectWithCredential(t, hub, sessionTwo)
	subscribe(t, clientTwo, "session.two.events", events)
	runtime := connectWithCredential(t, hub, space.Runtime)
	publish(t, runtime, "session.two.events", "visible")
	assertMessage(t, events, "visible")
}

func TestRuntimeCanCallOnlyImportedServiceFunctions(t *testing.T) {
	hub := startTestHub(t)
	space, err := hub.ProvisionSpace("knowledge")
	if err != nil {
		t.Fatalf("provision space: %v", err)
	}
	route, err := NewServiceFunctionRoute("io", "v1", "read_file")
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	routes := []ServiceFunctionRoute{route}
	serviceCredential, err := hub.IssueServiceCredential("io", routes)
	if err != nil {
		t.Fatalf("issue service credential: %v", err)
	}
	if err := hub.ImportServiceFunctions("knowledge", routes); err != nil {
		t.Fatalf("import service function: %v", err)
	}

	service := connectWithCredential(t, hub, serviceCredential)
	if _, err := service.Subscribe(route.ExportSubject, func(msg *nats.Msg) {
		if err := msg.Respond([]byte("ok:" + string(msg.Data))); err != nil {
			t.Errorf("respond: %v", err)
		}
	}); err != nil {
		t.Fatalf("service subscribe: %v", err)
	}
	service.Flush()

	runtime := connectWithCredential(t, hub, space.Runtime)
	reply, err := runtime.Request(route.ImportSubject, []byte("payload"), time.Second)
	if err != nil {
		t.Fatalf("request imported service: %v", err)
	}
	if got := string(reply.Data); got != "ok:payload" {
		t.Fatalf("reply = %q", got)
	}
	if _, err := runtime.Request("svc.io.v1.write_file", []byte("payload"), 100*time.Millisecond); err == nil {
		t.Fatal("expected unimported service function request to fail")
	} else if !errors.Is(err, nats.ErrNoResponders) && !errors.Is(err, nats.ErrTimeout) {
		t.Fatalf("unimported service function error = %v", err)
	}
}

func TestRuntimeCanRequestSupervisorCatalogSnapshot(t *testing.T) {
	hub := startTestHub(t)
	space, err := hub.ProvisionSpace("knowledge")
	if err != nil {
		t.Fatalf("provision space: %v", err)
	}
	controlCredential, err := hub.ControlCredential()
	if err != nil {
		t.Fatalf("control credential: %v", err)
	}
	control := connectWithCredential(t, hub, controlCredential)
	if _, err := control.Subscribe(catalogRuntimeGetSubject, func(msg *nats.Msg) {
		if err := msg.Respond([]byte("catalog:" + string(msg.Data))); err != nil {
			t.Errorf("respond: %v", err)
		}
	}); err != nil {
		t.Fatalf("catalog subscribe: %v", err)
	}
	control.Flush()

	runtime := connectWithCredential(t, hub, space.Runtime)
	reply, err := runtime.Request(catalogRuntimeGetSubject, []byte("payload"), time.Second)
	if err != nil {
		t.Fatalf("request catalog snapshot: %v", err)
	}
	if got := string(reply.Data); got != "catalog:payload" {
		t.Fatalf("reply = %q", got)
	}

	events := make(chan string, 1)
	subscribe(t, runtime, catalogRuntimeEventsSubject, events)
	publish(t, control, catalogRuntimeEventsSubject, "changed")
	assertMessage(t, events, "changed")
}

func TestIssuedCredentialsSurviveEmbeddedHubRestart(t *testing.T) {
	hub := startTestHub(t)
	session, err := hub.IssueSessionCredential("restart-space", "chat")
	if err != nil {
		t.Fatalf("issue session credential: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := hub.Stop(ctx); err != nil {
		t.Fatalf("stop hub: %v", err)
	}
	if err := hub.Start(context.Background()); err != nil {
		t.Fatalf("restart hub: %v", err)
	}
	nc := connectWithCredential(t, hub, session)
	if err := nc.Flush(); err != nil {
		t.Fatalf("flush restarted credential: %v", err)
	}
}

func startTestHub(t *testing.T) *Hub {
	t.Helper()
	cfg := DefaultConfig(filepath.Join(t.TempDir(), "nats"))
	cfg.Client.Port = 0
	cfg.WebSocket.Enabled = false
	cfg.Monitoring.Enabled = false
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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hub.Stop(ctx)
	})
	return hub
}

func connectWithCredential(t *testing.T, hub *Hub, credential Credential, opts ...nats.Option) *nats.Conn {
	t.Helper()
	options := []nats.Option{
		nats.UserInfo(credential.Username, credential.Password),
		nats.Timeout(time.Second),
	}
	options = append(options, opts...)
	nc, err := nats.Connect(hub.Endpoints().ClientURL, options...)
	if err != nil {
		t.Fatalf("connect %s credential: %v", credential.Role, err)
	}
	t.Cleanup(nc.Close)
	return nc
}

func subscribe(t *testing.T, nc *nats.Conn, subject string, out chan<- string) {
	t.Helper()
	if _, err := nc.Subscribe(subject, func(msg *nats.Msg) {
		out <- string(msg.Data)
	}); err != nil {
		t.Fatalf("subscribe %q: %v", subject, err)
	}
	nc.Flush()
	if err := nc.LastError(); err != nil {
		t.Fatalf("flush subscription %q: %v", subject, err)
	}
}

func publish(t *testing.T, nc *nats.Conn, subject, data string) {
	t.Helper()
	if err := nc.Publish(subject, []byte(data)); err != nil {
		t.Fatalf("publish %q: %v", subject, err)
	}
	nc.Flush()
	if err := nc.LastError(); err != nil {
		t.Fatalf("flush publish %q: %v", subject, err)
	}
}

func assertNoMessage(t *testing.T, messages <-chan string) {
	t.Helper()
	select {
	case msg := <-messages:
		t.Fatalf("unexpected message %q", msg)
	case <-time.After(150 * time.Millisecond):
	}
}

func assertMessage(t *testing.T, messages <-chan string, want string) {
	t.Helper()
	select {
	case got := <-messages:
		if got != want {
			t.Fatalf("message = %q, want %q", got, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %q", want)
	}
}

func assertPermissionError(t *testing.T, errors <-chan error) {
	t.Helper()
	select {
	case err := <-errors:
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "permissions violation") {
			t.Fatalf("permission error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for permission error")
	}
}
