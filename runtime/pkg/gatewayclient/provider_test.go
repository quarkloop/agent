package gatewayclient

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsgo "github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/plugin"
	modelv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/model/v1"
	"github.com/quarkloop/pkg/serviceapi/servicefunction"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestProviderStreamsGatewayResponsesAndUsage(t *testing.T) {
	ns := startProviderTestNATS(t)
	conn := connectProviderTestNATS(t, ns.ClientURL())
	defer conn.Close()

	subject, err := servicefunction.Subject("gateway", servicefunction.DefaultVersion, "stream_generate")
	if err != nil {
		t.Fatal(err)
	}
	sub, err := conn.QueueSubscribe(subject, "q.test.gateway", func(msg *natsgo.Msg) {
		var req servicefunction.RequestEnvelope
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		var payload modelv1.StreamGenerateRequest
		if err := protojson.Unmarshal(req.Payload, &payload); err != nil {
			t.Errorf("decode payload: %v", err)
			return
		}
		if payload.GetProvider() != "openrouter" || payload.GetModel() != "test/model" {
			t.Errorf("request provider/model = %q/%q", payload.GetProvider(), payload.GetModel())
			return
		}
		if payload.GetOptions()["max_output_tokens"] != "512" {
			t.Errorf("max_output_tokens option = %q", payload.GetOptions()["max_output_tokens"])
			return
		}
		publishProviderChunk(t, conn, msg.Reply, req.CallID, &modelv1.StreamGenerateResponse{Delta: "hello"})
		publishProviderChunk(t, conn, msg.Reply, req.CallID, &modelv1.StreamGenerateResponse{
			Done: true,
			Usage: &modelv1.ModelUsage{
				Provider:      "openrouter",
				Model:         "test/model",
				InputTokens:   7,
				OutputTokens:  3,
				RequestId:     "provider-request-1",
				FinishReason:  "stop",
				FallbackChain: []string{"openrouter"},
			},
		})
	})
	if err != nil {
		t.Fatalf("subscribe gateway subject: %v", err)
	}
	defer sub.Unsubscribe()
	if err := conn.FlushTimeout(time.Second); err != nil {
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

func connectProviderTestNATS(t *testing.T, url string) *natsgo.Conn {
	t.Helper()
	conn, err := natsgo.Connect(url, natsgo.UserInfo("quark-control", "secret"), natsgo.Timeout(time.Second))
	if err != nil {
		t.Fatalf("connect nats: %v", err)
	}
	return conn
}

func publishProviderChunk(t *testing.T, conn *natsgo.Conn, reply, callID string, chunk *modelv1.StreamGenerateResponse) {
	t.Helper()
	payload, err := protojson.Marshal(chunk)
	if err != nil {
		t.Fatalf("marshal chunk: %v", err)
	}
	envelope := servicefunction.OKResponse(callID, payload)
	envelope.Usage = usageEnvelope(chunk.GetUsage())
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	if err := conn.Publish(reply, data); err != nil {
		t.Fatalf("publish chunk: %v", err)
	}
}

func usageEnvelope(usage *modelv1.ModelUsage) *servicefunction.Usage {
	if usage == nil {
		return nil
	}
	return &servicefunction.Usage{
		Provider:     usage.GetProvider(),
		Model:        usage.GetModel(),
		RequestID:    usage.GetRequestId(),
		InputTokens:  usage.GetInputTokens(),
		OutputTokens: usage.GetOutputTokens(),
	}
}
