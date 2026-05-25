package gatewayclient

import (
	"context"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/pkg/plugin"
	gatewayv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/gateway/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestProviderStreamsGatewayResponsesAndUsage(t *testing.T) {
	ns := startProviderTestNATS(t)
	host, err := natskit.NewHost(context.Background(), natskit.Config{
		URL: ns.ClientURL(), Username: "quark-control", Password: "secret",
		Name: "gateway-provider-test",
	}, "q.test.gateway")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(host.Close)
	operation, _ := natskit.ServiceOperation("gateway", "stream_generate")
	err = host.RegisterStream(operation, time.Second, func(_ context.Context, req natskit.RequestEnvelope, publish func(natskit.ResponseEnvelope) error) (natskit.ResponseEnvelope, error) {
		var payload gatewayv1.StreamGenerateRequest
		if err := protojson.Unmarshal(req.Payload, &payload); err != nil {
			return natskit.ResponseEnvelope{}, err
		}
		if payload.GetProvider() != "openrouter" || payload.GetModel() != "test/model" {
			t.Errorf("request provider/model = %q/%q", payload.GetProvider(), payload.GetModel())
		}
		if payload.GetOptions()["max_output_tokens"] != "512" {
			t.Errorf("max_output_tokens option = %q", payload.GetOptions()["max_output_tokens"])
		}
		first := providerChunk(t, req.ServiceCallID, &gatewayv1.StreamGenerateResponse{Delta: "hello"})
		if err := publish(first); err != nil {
			return natskit.ResponseEnvelope{}, err
		}
		return providerChunk(t, req.ServiceCallID, &gatewayv1.StreamGenerateResponse{
			Done: true,
			Usage: &gatewayv1.ModelUsage{
				Provider:      "openrouter",
				Model:         "test/model",
				InputTokens:   7,
				OutputTokens:  3,
				RequestId:     "provider-request-1",
				FinishReason:  "stop",
				FallbackChain: []string{"openrouter"},
			},
		}), nil
	})
	if err != nil {
		t.Fatalf("subscribe gateway subject: %v", err)
	}
	if err := host.Ready(context.Background()); err != nil {
		t.Fatalf("flush gateway subscription: %v", err)
	}

	provider := New(Config{URL: ns.ClientURL(), Username: "quark-control", Password: "secret", Timeout: time.Second, MaxOutputTokens: 512}, "openrouter")
	stream, err := provider.ChatCompletionStream(context.Background(), &plugin.ChatRequest{
		Model:    "test/model",
		Messages: []plugin.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}

	var text string
	var usage *plugin.StreamUsage
	for event := range stream {
		if event.Err != nil {
			t.Fatalf("stream event error: %v", event.Err)
		}
		text += event.Delta
		if event.Usage != nil {
			usage = event.Usage
		}
		if event.Done {
			break
		}
	}
	if text != "hello" {
		t.Fatalf("streamed text = %q", text)
	}
	if usage == nil || usage.Provider != "openrouter" || usage.Model != "test/model" || usage.InputTokens != 7 || usage.OutputTokens != 3 {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestProviderBoundsGatewayStreamWait(t *testing.T) {
	ns := startProviderTestNATS(t)

	provider := New(Config{URL: ns.ClientURL(), Username: "quark-control", Password: "secret", Timeout: 20 * time.Millisecond}, "openrouter")
	stream, err := provider.ChatCompletionStream(context.Background(), &plugin.ChatRequest{
		Model:    "test/model",
		Messages: []plugin.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	select {
	case event := <-stream:
		if event.Err == nil {
			t.Fatalf("expected gateway wait error, got %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("gateway stream wait did not terminate")
	}
}

func TestConfigFromEnvReadsGatewayTimeout(t *testing.T) {
	t.Setenv(EnvGatewayTimeout, "2m")

	cfg := ConfigFromEnv()
	if cfg.Timeout != 2*time.Minute {
		t.Fatalf("timeout = %s", cfg.Timeout)
	}
}

func startProviderTestNATS(t *testing.T) *natsserver.Server {
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

func providerChunk(t *testing.T, callID string, chunk *gatewayv1.StreamGenerateResponse) natskit.ResponseEnvelope {
	t.Helper()
	payload, err := protojson.Marshal(chunk)
	if err != nil {
		t.Fatalf("marshal chunk: %v", err)
	}
	envelope := natskit.OKResponse(callID, payload)
	envelope.Usage = usageEnvelope(chunk.GetUsage())
	return envelope
}

func usageEnvelope(usage *gatewayv1.ModelUsage) *natskit.Usage {
	if usage == nil {
		return nil
	}
	return &natskit.Usage{
		Provider:     usage.GetProvider(),
		Model:        usage.GetModel(),
		RequestID:    usage.GetRequestId(),
		InputTokens:  usage.GetInputTokens(),
		OutputTokens: usage.GetOutputTokens(),
	}
}
