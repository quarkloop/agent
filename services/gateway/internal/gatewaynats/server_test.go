package gatewaynats

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	gatewayv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/gateway/v1"
	"github.com/quarkloop/pkg/serviceapi/servicefunction"
	"github.com/quarkloop/services/gateway/internal/gatewaysvc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestGatewayNATSGenerateAndUsageSummary(t *testing.T) {
	ns := startTestNATS(t)
	srv, err := gatewaysvc.NewServer(gatewaysvc.Config{
		Providers: []gatewaysvc.ProviderConfig{{ID: "local", Kind: "local", Model: "local/noop", Enabled: true}},
	})
	if err != nil {
		t.Fatalf("new model server: %v", err)
	}
	gateway := New(Config{
		URL:      ns.ClientURL(),
		Username: "quark-control",
		Password: "secret",
		Queue:    "q.gateway.test",
	}, srv)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := gateway.Start(ctx); err != nil {
		t.Fatalf("start gateway: %v", err)
	}
	defer gateway.Close()

	conn := connectTestNATS(t, ns.ClientURL())
	defer conn.Close()

	var generated gatewayv1.GenerateResponse
	callGateway(t, conn, "generate", &gatewayv1.GenerateRequest{
		Provider: "local",
		Model:    "local/noop",
		Messages: []*gatewayv1.ModelMessage{{Role: "user", Content: "say hello"}},
	}, &generated)
	if generated.GetText() == "" || generated.GetUsage().GetProvider() != "local" {
		t.Fatalf("generate response = %+v", &generated)
	}

	var summary gatewayv1.UsageSummaryResponse
	callGateway(t, conn, "usage_summary", &gatewayv1.UsageSummaryRequest{}, &summary)
	if len(summary.GetUsage()) != 1 || summary.GetUsage()[0].GetProvider() != "local" || summary.GetUsage()[0].GetRequests() == 0 {
		t.Fatalf("usage summary = %+v", summary.GetUsage())
	}
}

func TestGatewayNATSReloadConfig(t *testing.T) {
	ns := startTestNATS(t)
	srv, err := gatewaysvc.NewServer(gatewaysvc.Config{
		Providers: []gatewaysvc.ProviderConfig{{ID: "local", Kind: "local", Model: "local/noop", Enabled: true}},
	})
	if err != nil {
		t.Fatalf("new model server: %v", err)
	}
	gateway := New(Config{URL: ns.ClientURL(), Username: "quark-control", Password: "secret", Queue: "q.gateway.test"}, srv)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := gateway.Start(ctx); err != nil {
		t.Fatalf("start gateway: %v", err)
	}
	defer gateway.Close()

	conn := connectTestNATS(t, ns.ClientURL())
	defer conn.Close()
	var reloaded gatewayv1.ReloadConfigResponse
	callGateway(t, conn, "reload_config", &gatewayv1.ReloadConfigRequest{
		Providers: []*gatewayv1.GatewayProviderConfig{{
			Id:      "local",
			Kind:    "local",
			Model:   "local/reloaded",
			Enabled: true,
		}},
	}, &reloaded)
	if !reloaded.GetReloaded() || len(reloaded.GetProviders()) != 1 || reloaded.GetProviders()[0] != "local" {
		t.Fatalf("reload response = %+v", &reloaded)
	}

	var models gatewayv1.ListModelsResponse
	callGateway(t, conn, "list_models", &gatewayv1.ListModelsRequest{Provider: "local"}, &models)
	if len(models.GetModels()) != 1 || models.GetModels()[0].GetId() != "local/reloaded" {
		t.Fatalf("models after reload = %+v", models.GetModels())
	}
}

func TestGatewayNATSStreamGenerate(t *testing.T) {
	ns := startTestNATS(t)
	srv, err := gatewaysvc.NewServer(gatewaysvc.Config{
		Providers: []gatewaysvc.ProviderConfig{{ID: "local", Kind: "local", Model: "local/noop", Enabled: true}},
	})
	if err != nil {
		t.Fatalf("new model server: %v", err)
	}
	gateway := New(Config{URL: ns.ClientURL(), Username: "quark-control", Password: "secret", Queue: "q.gateway.test"}, srv)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := gateway.Start(ctx); err != nil {
		t.Fatalf("start gateway: %v", err)
	}
	defer gateway.Close()

	conn := connectTestNATS(t, ns.ClientURL())
	defer conn.Close()
	subject := subjectFor(t, "stream_generate")
	inbox := natsgo.NewInbox()
	sub, err := conn.SubscribeSync(inbox)
	if err != nil {
		t.Fatalf("subscribe inbox: %v", err)
	}
	defer sub.Unsubscribe()
	payload, err := protojson.Marshal(&gatewayv1.StreamGenerateRequest{
		Provider: "local",
		Model:    "local/noop",
		Messages: []*gatewayv1.ModelMessage{{Role: "user", Content: "stream this"}},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	req := requestEnvelope(t, "stream_generate", payload)
	data, _ := json.Marshal(req)
	if err := conn.PublishRequest(subject, inbox, data); err != nil {
		t.Fatalf("publish request: %v", err)
	}
	var sawDone bool
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		msg, err := sub.NextMsg(time.Second)
		if err != nil {
			t.Fatalf("next stream message: %v", err)
		}
		var envelope servicefunction.ResponseEnvelope
		if err := json.Unmarshal(msg.Data, &envelope); err != nil {
			t.Fatalf("decode envelope: %v", err)
		}
		if envelope.Status != servicefunction.StatusOK {
			t.Fatalf("stream error = %+v", envelope.Error)
		}
		var chunk gatewayv1.StreamGenerateResponse
		if err := protojson.Unmarshal(envelope.Payload, &chunk); err != nil {
			t.Fatalf("decode chunk: %v", err)
		}
		if chunk.GetDone() {
			if chunk.GetUsage().GetProvider() != "local" {
				t.Fatalf("done usage = %+v", chunk.GetUsage())
			}
			sawDone = true
			break
		}
	}
	if !sawDone {
		t.Fatal("stream did not finish")
	}
}

func startTestNATS(t *testing.T) *natsserver.Server {
	t.Helper()
	ns, err := natsserver.NewServer(&natsserver.Options{
		Host:     "127.0.0.1",
		Port:     -1,
		Username: "quark-control",
		Password: "secret",
		NoLog:    true,
		NoSigs:   true,
	})
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

func connectTestNATS(t *testing.T, url string) *natsgo.Conn {
	t.Helper()
	conn, err := natsgo.Connect(url, natsgo.UserInfo("quark-control", "secret"), natsgo.Timeout(time.Second))
	if err != nil {
		t.Fatalf("connect nats: %v", err)
	}
	return conn
}

func callGateway(t *testing.T, conn *natsgo.Conn, function string, request proto.Message, response proto.Message) {
	t.Helper()
	payload, err := protojson.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	envelope := callGatewayRaw(t, conn, function, payload)
	if err := protojson.Unmarshal(envelope.Payload, response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func callGatewayRaw(t *testing.T, conn *natsgo.Conn, function string, payload json.RawMessage) servicefunction.ResponseEnvelope {
	t.Helper()
	req := requestEnvelope(t, function, payload)
	data, _ := json.Marshal(req)
	msg, err := conn.Request(subjectFor(t, function), data, time.Second)
	if err != nil {
		t.Fatalf("request gateway %s: %v", function, err)
	}
	var resp servicefunction.ResponseEnvelope
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		t.Fatalf("decode response envelope: %v", err)
	}
	if resp.Status != servicefunction.StatusOK {
		t.Fatalf("gateway %s failed: %+v", function, resp.Error)
	}
	return resp
}

func requestEnvelope(t *testing.T, function string, payload json.RawMessage) servicefunction.RequestEnvelope {
	t.Helper()
	subject := subjectFor(t, function)
	req, err := servicefunction.NewRequest("call-"+function, "space-1", servicefunction.ActorRuntime, servicefunction.Descriptor{
		Version:       servicefunction.DescriptorVersion,
		Service:       "gateway",
		Function:      function,
		Subject:       subject,
		InputSchema:   json.RawMessage(`{"type":"object"}`),
		OutputSchema:  json.RawMessage(`{"type":"object"}`),
		Risk:          servicefunction.RiskRead,
		TimeoutMillis: 1000,
	}, payload)
	if err != nil {
		t.Fatalf("new request envelope: %v", err)
	}
	return req
}

func subjectFor(t *testing.T, function string) string {
	t.Helper()
	subject, err := servicefunction.Subject("gateway", servicefunction.DefaultVersion, function)
	if err != nil {
		t.Fatalf("subject: %v", err)
	}
	return subject
}
