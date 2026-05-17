package modelclient

import (
	"context"
	"net"
	"testing"

	"github.com/quarkloop/pkg/plugin"
	modelv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/model/v1"
	"google.golang.org/grpc"
)

func TestProviderStreamsThroughModelService(t *testing.T) {
	fake := &fakeModelService{}
	server := grpc.NewServer()
	modelv1.RegisterModelServiceServer(server, fake)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve(ln) }()
	defer server.Stop()

	provider := New(ln.Addr().String(), "openrouter")
	stream, err := provider.ChatCompletionStream(context.Background(), &plugin.ChatRequest{
		Model: "model-a",
		Messages: []plugin.Message{{
			Role:    "user",
			Content: "hello",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got string
	for event := range stream {
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		got += event.Delta
		if event.Done {
			break
		}
	}
	if got != "hello from model service" {
		t.Fatalf("stream text = %q", got)
	}
	if fake.provider != "openrouter" || fake.model != "model-a" {
		t.Fatalf("request provider/model = %q/%q", fake.provider, fake.model)
	}
}

type fakeModelService struct {
	modelv1.UnimplementedModelServiceServer
	provider string
	model    string
}

func (s *fakeModelService) StreamGenerate(req *modelv1.StreamGenerateRequest, stream modelv1.ModelService_StreamGenerateServer) error {
	s.provider = req.GetProvider()
	s.model = req.GetModel()
	if err := stream.Send(&modelv1.StreamGenerateResponse{Delta: "hello "}); err != nil {
		return err
	}
	if err := stream.Send(&modelv1.StreamGenerateResponse{Delta: "from model service"}); err != nil {
		return err
	}
	return stream.Send(&modelv1.StreamGenerateResponse{Done: true})
}
