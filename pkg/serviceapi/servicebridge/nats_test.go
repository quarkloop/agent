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
			Service:      embeddingv1.EmbeddingService_ServiceDesc.ServiceName,
			Method:       "Embed",
			Request:      "quark.embedding.v1.EmbedRequest",
			Response:     "quark.embedding.v1.EmbedResponse",
			FunctionName: "embedding_Embed",
		}},
	}
	if err := server.Start(context.Background(), Binding{
		Descriptor: desc,
		Services: []GRPCService{{
			Desc:           &embeddingv1.EmbeddingService_ServiceDesc,
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

type embeddingBridgeServer struct {
	embeddingv1.UnimplementedEmbeddingServiceServer
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
