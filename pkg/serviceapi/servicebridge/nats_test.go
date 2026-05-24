package servicebridge

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	embeddingv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/embedding/v1"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/pkg/serviceapi/observability"
	"github.com/quarkloop/pkg/serviceapi/servicefunction"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestNATSServiceDispatchesUnaryServiceFunction(t *testing.T) {
	ns := startBridgeNATS(t)
	server := NewNATSService(NATSConfig{URL: ns.ClientURL(), Queue: "q.embedding.test", Name: "embedding-test"})
	impl := embeddingBridgeServer{}
	desc := &servicev1.ServiceDescriptor{
		Name:    "embedding",
		Version: "1.0.0",
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:      "quark.embedding.v1.EmbeddingService",
			Method:       "Embed",
			Request:      "quark.embedding.v1.EmbedRequest",
			Response:     "quark.embedding.v1.EmbedResponse",
			FunctionName: "embedding_Embed",
		}},
	}
	if err := server.Start(context.Background(), Binding{
		Descriptor: desc,
		Services: []RPCService{{
			Service:        "quark.embedding.v1.EmbeddingService",
			Implementation: impl,
		}},
	}); err != nil {
		t.Fatalf("start service bridge: %v", err)
	}
	t.Cleanup(server.Close)

	conn, err := natsgo.Connect(ns.ClientURL())
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer conn.Close()

	payload, err := protojson.Marshal(&embeddingv1.EmbedRequest{Input: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	request := servicefunction.RequestEnvelope{
		Version:  servicefunction.EnvelopeVersion,
		CallID:   "call-1",
		SpaceID:  "space-1",
		Actor:    servicefunction.ActorRuntime,
		Service:  "embedding",
		Function: "embed",
		Subject:  "svc.embedding.v1.embed",
		Payload:  payload,
	}
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := conn.Request(request.Subject, data, time.Second)
	if err != nil {
		t.Fatalf("request service function: %v", err)
	}
	var response servicefunction.ResponseEnvelope
	if err := json.Unmarshal(reply.Data, &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Status != servicefunction.StatusOK {
		t.Fatalf("response = %+v", response)
	}
	var embed embeddingv1.EmbedResponse
	if err := protojson.Unmarshal(response.Payload, &embed); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if embed.GetModel() != "bridge-test" || len(embed.GetVector()) != 2 {
		t.Fatalf("embed = %+v", &embed)
	}
}

func TestNATSServicePublishesRedactedAuditAndTelemetryEvents(t *testing.T) {
	ns := startBridgeNATS(t)
	server := NewNATSService(NATSConfig{
		URL:             ns.ClientURL(),
		Queue:           "q.embedding.events.test",
		Name:            "embedding-events-test",
		AuditPrefix:     "audit",
		TelemetryPrefix: "telemetry",
	})
	impl := embeddingBridgeServer{}
	desc := &servicev1.ServiceDescriptor{
		Name:    "embedding",
		Version: "1.0.0",
		Rpcs: []*servicev1.RpcDescriptor{{
			Service:      "quark.embedding.v1.EmbeddingService",
			Method:       "Embed",
			Request:      "quark.embedding.v1.EmbedRequest",
			Response:     "quark.embedding.v1.EmbedResponse",
			FunctionName: "embedding_Embed",
		}},
	}
	if err := server.Start(context.Background(), Binding{
		Descriptor: desc,
		Services: []RPCService{{
			Service:        "quark.embedding.v1.EmbeddingService",
			Implementation: impl,
		}},
	}); err != nil {
		t.Fatalf("start service bridge: %v", err)
	}
	t.Cleanup(server.Close)

	conn, err := natsgo.Connect(ns.ClientURL())
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer conn.Close()
	auditSub, err := conn.SubscribeSync("audit.space_1.service_calls")
	if err != nil {
		t.Fatalf("subscribe audit: %v", err)
	}
	telemetrySub, err := conn.SubscribeSync("telemetry.space_1.service_calls")
	if err != nil {
		t.Fatalf("subscribe telemetry: %v", err)
	}
	if err := conn.FlushTimeout(time.Second); err != nil {
		t.Fatalf("flush subscriptions: %v", err)
	}

	payload, err := protojson.Marshal(&embeddingv1.EmbedRequest{Input: "secret text"})
	if err != nil {
		t.Fatal(err)
	}
	request := servicefunction.RequestEnvelope{
		Version:     servicefunction.EnvelopeVersion,
		CallID:      "call-2",
		SpaceID:     "space-1",
		SessionID:   "session-1",
		RunID:       "run-1",
		Actor:       servicefunction.ActorRuntime,
		Service:     "embedding",
		Function:    "embed",
		Subject:     "svc.embedding.v1.embed",
		Payload:     payload,
		TraceParent: "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01",
		TraceState:  "vendor=value",
	}
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Request(request.Subject, data, time.Second); err != nil {
		t.Fatalf("request service function: %v", err)
	}
	assertServiceCallEvent(t, auditSub, "call-2")
	assertServiceCallEvent(t, telemetrySub, "call-2")
}

type embeddingBridgeServer struct {
}

func (embeddingBridgeServer) Embed(context.Context, *embeddingv1.EmbedRequest) (*embeddingv1.EmbedResponse, error) {
	return &embeddingv1.EmbedResponse{
		Vector:      []float32{0.1, 0.2},
		Model:       "bridge-test",
		Dimensions:  2,
		Provider:    "test",
		ContentHash: "hash",
	}, nil
}

func startBridgeNATS(t *testing.T) *natsserver.Server {
	t.Helper()
	ns, err := natsserver.NewServer(&natsserver.Options{Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true})
	if err != nil {
		t.Fatalf("new nats server: %v", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(time.Second) {
		ns.Shutdown()
		t.Fatal("nats server did not become ready")
	}
	t.Cleanup(ns.Shutdown)
	return ns
}

func assertServiceCallEvent(t *testing.T, sub *natsgo.Subscription, callID string) {
	t.Helper()
	msg, err := sub.NextMsg(time.Second)
	if err != nil {
		t.Fatalf("next event: %v", err)
	}
	var event observability.ServiceCallEvent
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		t.Fatalf("decode event: %v", err)
	}
	if event.CallID != callID || event.Service != "embedding" || event.Function != "embed" || event.Subject != "svc.embedding.v1.embed" {
		t.Fatalf("event = %+v", event)
	}
	if event.Status != string(servicefunction.StatusOK) {
		t.Fatalf("event status = %q", event.Status)
	}
	if event.TraceParent == "" || event.TraceState == "" {
		t.Fatalf("event trace fields missing: %+v", event)
	}
	if string(msg.Data) == "secret text" {
		t.Fatalf("event leaked request payload: %s", msg.Data)
	}
}
