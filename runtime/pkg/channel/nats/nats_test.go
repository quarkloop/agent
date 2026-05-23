package nats_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	"github.com/quarkloop/runtime/pkg/activity"
	natschannel "github.com/quarkloop/runtime/pkg/channel/nats"
	"github.com/quarkloop/runtime/pkg/message"
	"github.com/quarkloop/runtime/pkg/plan"
	"github.com/quarkloop/runtime/pkg/session"
)

func TestChannelAcceptsSessionInputAndPublishesEvents(t *testing.T) {
	serverURL := startServer(t)

	poster := &fakePoster{requests: make(chan message.PostRequest, 1)}
	activityStore := activity.NewStore(10)
	channel := natschannel.New(natschannel.Config{
		URL:   serverURL,
		Name:  "runtime-channel-test",
		Queue: "q.runtime.test",
	}, poster, session.NewRegistry(), natschannel.WithPlan(plan.New()), natschannel.WithActivity(activityStore))

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
	activityEvents := subscribeActivity(t, client)
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

	infoResp := requestPayload[clientcontract.RuntimeInfoResponse](t, client, clientcontract.SubjectRuntimeInfoGet, clientcontract.RuntimeInfoRequest{SpaceID: "docs"})
	if infoResp.Sessions != 1 {
		t.Fatalf("info response = %#v", infoResp)
	}
	sessionResp := requestPayload[clientcontract.RuntimeSessionResponse](t, client, clientcontract.SubjectRuntimeSessionGet, clientcontract.RuntimeSessionRequest{
		SpaceID:   "docs",
		SessionID: "chat",
	})
	if !sessionResp.Found {
		t.Fatalf("session response = %#v", sessionResp)
	}

	activityStore.Add("chat", "message.user", map[string]string{"source": "test"})
	assertActivityEvent(t, activityEvents, "message.user")

	planResp := requestPayload[clientcontract.RuntimePlanResponse](t, client, clientcontract.SubjectRuntimePlanGet, clientcontract.RuntimePlanRequest{SpaceID: "docs"})
	if planResp.Status != "idle" {
		t.Fatalf("plan response = %#v", planResp)
	}
	activityResp := requestPayload[clientcontract.RuntimeActivityListResponse](t, client, clientcontract.SubjectRuntimeActivityList, clientcontract.RuntimeActivityListRequest{
		SpaceID: "docs",
		Limit:   10,
	})
	if len(activityResp.Records) != 1 || activityResp.Records[0].Type != "message.user" {
		t.Fatalf("activity response = %#v", activityResp)
	}
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

func subscribeActivity(t *testing.T, conn *natsgo.Conn) <-chan clientcontract.RuntimeActivityRecord {
	t.Helper()
	records := make(chan clientcontract.RuntimeActivityRecord, 4)
	sub, err := conn.Subscribe(clientcontract.SubjectRuntimeActivityFeed, func(msg *natsgo.Msg) {
		var record clientcontract.RuntimeActivityRecord
		if err := json.Unmarshal(msg.Data, &record); err != nil {
			t.Errorf("decode activity: %v", err)
			return
		}
		records <- record
	})
	if err != nil {
		t.Fatalf("subscribe activity: %v", err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	if err := conn.FlushTimeout(time.Second); err != nil {
		t.Fatalf("flush activity subscription: %v", err)
	}
	return records
}

func assertActivityEvent(t *testing.T, records <-chan clientcontract.RuntimeActivityRecord, want string) {
	t.Helper()
	select {
	case got := <-records:
		if got.Type != want {
			t.Fatalf("activity type = %q, want %q", got.Type, want)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for %s activity", want)
	}
}

func requestPayload[T any](t *testing.T, conn *natsgo.Conn, subject string, payload any) T {
	t.Helper()
	req, err := clientcontract.NewRequest("req-"+subject, "docs", payload)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	reply, err := conn.Request(subject, data, time.Second)
	if err != nil {
		t.Fatalf("request %s: %v", subject, err)
	}
	var resp clientcontract.ResponseEnvelope
	if err := json.Unmarshal(reply.Data, &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != "ok" {
		t.Fatalf("response = %#v", resp)
	}
	var out T
	if err := resp.DecodePayload(&out); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	return out
}
