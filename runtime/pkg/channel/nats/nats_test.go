package nats_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	natschannel "github.com/quarkloop/runtime/pkg/channel/nats"
	"github.com/quarkloop/runtime/pkg/message"
	"github.com/quarkloop/runtime/pkg/session"
)

func TestChannelAcceptsSessionInputAndPublishesEvents(t *testing.T) {
	serverURL := startServer(t)

	poster := &fakePoster{requests: make(chan message.PostRequest, 1)}
	channel := natschannel.New(natschannel.Config{
		URL:   serverURL,
		Name:  "runtime-channel-test",
		Queue: "q.runtime.test",
	}, poster, session.NewRegistry())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := channel.Start(ctx); err != nil {
		t.Fatalf("start channel: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		_ = channel.Stop(stopCtx)
	})

	client := connectNATS(t, serverURL)
	eventsSubject, err := clientcontract.SessionEventsSubject("chat")
	if err != nil {
		t.Fatalf("events subject: %v", err)
	}
	events := make(chan clientcontract.SessionEvent, 4)
	sub, err := client.Subscribe(eventsSubject, func(msg *natsgo.Msg) {
		var event clientcontract.SessionEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			t.Errorf("decode event: %v", err)
			return
		}
		events <- event
	})
	if err != nil {
		t.Fatalf("subscribe events: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	if err := client.FlushTimeout(time.Second); err != nil {
		t.Fatalf("flush events subscription: %v", err)
	}

	req, err := clientcontract.NewRequest("req-chat", "docs", clientcontract.SendMessageRequest{
		SpaceID:   "docs",
		SessionID: "chat",
		Content:   "hello",
	})
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	inputSubject, err := clientcontract.SessionInputSubject("chat")
	if err != nil {
		t.Fatalf("input subject: %v", err)
	}
	reply, err := client.Request(inputSubject, data, time.Second)
	if err != nil {
		t.Fatalf("request input: %v", err)
	}
	var response clientcontract.ResponseEnvelope
	if err := json.Unmarshal(reply.Data, &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != "ok" {
		t.Fatalf("response = %#v", response)
	}

	select {
	case got := <-poster.requests:
		if got.SessionID != "chat" || got.Content != "hello" {
			t.Fatalf("post request = %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for poster request")
	}
	assertSessionEvent(t, events, "token")
	assertSessionEvent(t, events, "done")
}

type fakePoster struct {
	requests chan message.PostRequest
}

func (p *fakePoster) Post(_ context.Context, request message.PostRequest, resp chan message.StreamMessage) {
	p.requests <- request
	go func() {
		defer close(resp)
		resp <- message.StreamMessage{Type: "token", Data: "hello"}
	}()
}

func startServer(t *testing.T) string {
	t.Helper()
	server, err := natsserver.NewServer(&natsserver.Options{
		Host:   "127.0.0.1",
		Port:   -1,
		NoSigs: true,
		NoLog:  true,
	})
	if err != nil {
		t.Fatalf("new nats server: %v", err)
	}
	server.Start()
	if !server.ReadyForConnections(5 * time.Second) {
		server.Shutdown()
		server.WaitForShutdown()
		t.Fatal("nats server did not become ready")
	}
	t.Cleanup(func() {
		server.Shutdown()
		server.WaitForShutdown()
	})
	return server.ClientURL()
}

func connectNATS(t *testing.T, url string) *natsgo.Conn {
	t.Helper()
	conn, err := natsgo.Connect(
		url,
		natsgo.Timeout(time.Second),
	)
	if err != nil {
		t.Fatalf("connect nats: %v", err)
	}
	t.Cleanup(conn.Close)
	return conn
}

func assertSessionEvent(t *testing.T, events <-chan clientcontract.SessionEvent, want string) {
	t.Helper()
	select {
	case got := <-events:
		if got.Type != want {
			t.Fatalf("event type = %q, want %q", got.Type, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s event", want)
	}
}
