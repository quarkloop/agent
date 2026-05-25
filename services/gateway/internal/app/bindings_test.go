package app

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/quarkloop/pkg/natskit"
	gatewayv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/gateway/v1"
	"github.com/quarkloop/services/gateway/internal/gatewaysvc"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestGatewayBindingDispatchesUnaryOperation(t *testing.T) {
	broker := startGatewayNATS(t)
	server := gatewayServer(t)
	host, err := natskit.StartRPCService(context.Background(), natskit.Config{URL: broker.ClientURL(), Name: "gateway-host"}, "q.gateway.test", gatewayBinding("svc.gateway.v1", nil, server))
	if err != nil {
		t.Fatalf("start gateway binding: %v", err)
	}
	t.Cleanup(host.Close)
	client := gatewayClient(t, broker.ClientURL())
	payload, _ := protojson.Marshal(&gatewayv1.GenerateRequest{
		Provider: "fixture",
		Model:    "fixture/chat",
		Messages: []*gatewayv1.ModelMessage{textMessage("user", "say hello")},
	})
	req, _ := natskit.NewRequest("call-generate", "space-1", natskit.ActorRuntime, payload)
	operation, _ := natskit.ServiceOperation("gateway", "generate")
	resp, err := client.Call(context.Background(), operation, req)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	var generated gatewayv1.GenerateResponse
	if err := protojson.Unmarshal(resp.Payload, &generated); err != nil {
		t.Fatal(err)
	}
	if generated.GetText() == "" || generated.GetUsage().GetProvider() != "fixture" || resp.ReferenceID == "" {
		t.Fatalf("generate response = %+v envelope = %+v", &generated, resp)
	}
	if resp.Usage == nil || resp.Usage.Provider != "fixture" {
		t.Fatalf("gateway envelope usage = %+v", resp.Usage)
	}
}

func TestGatewayBindingStreamsSingleTerminalCompletion(t *testing.T) {
	broker := startGatewayNATS(t)
	server := gatewayServer(t)
	host, err := natskit.StartRPCService(context.Background(), natskit.Config{URL: broker.ClientURL(), Name: "gateway-stream-host"}, "q.gateway.test", gatewayBinding("svc.gateway.v1", nil, server))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(host.Close)
	client := gatewayClient(t, broker.ClientURL())
	payload, _ := protojson.Marshal(&gatewayv1.StreamGenerateRequest{
		Provider: "fixture",
		Model:    "fixture/chat",
		Messages: []*gatewayv1.ModelMessage{textMessage("user", "stream this")},
	})
	req, _ := natskit.NewRequest("call-stream", "space-1", natskit.ActorRuntime, payload)
	operation, _ := natskit.ServiceOperation("gateway", "stream_generate")
	stream, err := client.OpenServiceStream(context.Background(), operation, req)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var done int
	for {
		data, err := stream.Next(ctx)
		if err != nil {
			t.Fatalf("next stream event: %v", err)
		}
		envelope, err := natskit.DecodeServiceResponse(data)
		if err != nil {
			t.Fatal(err)
		}
		var event gatewayv1.StreamGenerateResponse
		if err := protojson.Unmarshal(envelope.Payload, &event); err != nil {
			t.Fatal(err)
		}
		if event.GetDone() {
			done++
			if event.GetUsage().GetProvider() != "fixture" {
				t.Fatalf("terminal usage = %+v", event.GetUsage())
			}
			if envelope.Usage == nil || envelope.Usage.Provider != "fixture" {
				t.Fatalf("terminal envelope usage = %+v", envelope.Usage)
			}
			break
		}
	}
	if done != 1 {
		t.Fatalf("terminal completions = %d", done)
	}
}

func TestGatewayUsageEnvelopeDoesNotCarryModelContent(t *testing.T) {
	usage := gatewayUsageFromResponse(&gatewayv1.GenerateResponse{
		Text: "private response content",
		Usage: &gatewayv1.ModelUsage{
			Provider:     "fixture",
			Model:        "fixture/chat",
			RequestId:    "provider-request-1",
			InputTokens:  3,
			OutputTokens: 4,
		},
	})
	if usage == nil {
		t.Fatal("usage envelope is nil")
	}
	if strings.Contains(string(usage.AdditionalJSON), "private response content") {
		t.Fatalf("usage envelope leaks model content: %s", usage.AdditionalJSON)
	}
}

func gatewayServer(t *testing.T) *gatewaysvc.Server {
	t.Helper()
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(endpoint.Close)
	server, err := gatewaysvc.NewServer(gatewaysvc.Config{Providers: []gatewaysvc.ProviderConfig{{
		ID: "fixture", Kind: "openai-compatible", APIKey: "test-key", BaseURL: endpoint.URL, Model: "fixture/chat", Enabled: true,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	return server
}

func textMessage(role, text string) *gatewayv1.ModelMessage {
	return &gatewayv1.ModelMessage{Role: role, Content: []*gatewayv1.ContentPart{{Kind: gatewayv1.ContentKind_CONTENT_KIND_TEXT, Text: text}}}
}

func startGatewayNATS(t *testing.T) *natsserver.Server {
	t.Helper()
	server, err := natsserver.NewServer(&natsserver.Options{Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true})
	if err != nil {
		t.Fatal(err)
	}
	go server.Start()
	if !server.ReadyForConnections(time.Second) {
		t.Fatal("nats broker not ready")
	}
	t.Cleanup(server.Shutdown)
	return server
}

func gatewayClient(t *testing.T, url string) *natskit.Client {
	t.Helper()
	client, err := natskit.Connect(context.Background(), natskit.Config{URL: url, Name: "gateway-client"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}
