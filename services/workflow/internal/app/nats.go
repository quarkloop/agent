package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/quarkloop/pkg/natskit"
	workflowv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/workflow/v1"
	"github.com/quarkloop/services/workflow/internal/workflowsvc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type workflowUnary func(context.Context, proto.Message) (proto.Message, error)

func startWorkflowHost(ctx context.Context, cfg natskit.Config, queue string, server *workflowsvc.Server) (*natskit.Host, error) {
	if queue == "" {
		queue = "q.workflow.v1"
	}
	host, err := natskit.NewHost(ctx, cfg, queue)
	if err != nil {
		return nil, err
	}
	operations := []struct {
		function string
		request  func() proto.Message
		invoke   workflowUnary
	}{
		{"start", func() proto.Message { return &workflowv1.StartWorkflowRequest{} }, func(ctx context.Context, msg proto.Message) (proto.Message, error) {
			return server.Start(ctx, msg.(*workflowv1.StartWorkflowRequest))
		}},
		{"signal", func() proto.Message { return &workflowv1.SignalWorkflowRequest{} }, func(ctx context.Context, msg proto.Message) (proto.Message, error) {
			return server.Signal(ctx, msg.(*workflowv1.SignalWorkflowRequest))
		}},
		{"query", func() proto.Message { return &workflowv1.QueryWorkflowRequest{} }, func(ctx context.Context, msg proto.Message) (proto.Message, error) {
			return server.Query(ctx, msg.(*workflowv1.QueryWorkflowRequest))
		}},
		{"cancel", func() proto.Message { return &workflowv1.CancelWorkflowRequest{} }, func(ctx context.Context, msg proto.Message) (proto.Message, error) {
			return server.Cancel(ctx, msg.(*workflowv1.CancelWorkflowRequest))
		}},
		{"describe", func() proto.Message { return &workflowv1.DescribeWorkflowRequest{} }, func(ctx context.Context, msg proto.Message) (proto.Message, error) {
			return server.Describe(ctx, msg.(*workflowv1.DescribeWorkflowRequest))
		}},
		{"list", func() proto.Message { return &workflowv1.ListWorkflowsRequest{} }, func(ctx context.Context, msg proto.Message) (proto.Message, error) {
			return server.List(ctx, msg.(*workflowv1.ListWorkflowsRequest))
		}},
	}
	for _, item := range operations {
		operation, err := natskit.ServiceOperation("workflow", item.function)
		if err != nil {
			host.Close()
			return nil, err
		}
		item := item
		if err := host.RegisterUnary(operation, transportTimeout(cfg), func(ctx context.Context, req natskit.RequestEnvelope) (natskit.ResponseEnvelope, error) {
			input := item.request()
			if err := protojson.Unmarshal(req.Payload, input); err != nil {
				return natskit.ResponseEnvelope{}, err
			}
			output, err := item.invoke(ctx, input)
			if err != nil {
				return natskit.ResponseEnvelope{}, err
			}
			payload, err := protojson.MarshalOptions{UseProtoNames: false}.Marshal(output)
			if err != nil {
				return natskit.ResponseEnvelope{}, err
			}
			return natskit.OKResponse(req.ServiceCallID, payload), nil
		}); err != nil {
			host.Close()
			return nil, err
		}
	}
	streamOperation, _ := natskit.ServiceOperation("workflow", "stream_events")
	if err := host.RegisterStream(streamOperation, transportTimeout(cfg), func(ctx context.Context, req natskit.RequestEnvelope, publish func(natskit.ResponseEnvelope) error) (natskit.ResponseEnvelope, error) {
		var input workflowv1.StreamWorkflowEventsRequest
		if err := protojson.Unmarshal(req.Payload, &input); err != nil {
			return natskit.ResponseEnvelope{}, err
		}
		events, err := server.EngineEvents(ctx, &input)
		if err != nil {
			return natskit.ResponseEnvelope{}, err
		}
		for event := range events {
			payload, err := protojson.MarshalOptions{UseProtoNames: false}.Marshal(event)
			if err != nil {
				return natskit.ResponseEnvelope{}, err
			}
			if err := publish(natskit.OKResponse(req.ServiceCallID, payload)); err != nil {
				return natskit.ResponseEnvelope{}, err
			}
		}
		return natskit.OKResponse(req.ServiceCallID, json.RawMessage(`{}`)), nil
	}); err != nil {
		host.Close()
		return nil, err
	}
	if err := host.Ready(ctx); err != nil {
		host.Close()
		return nil, fmt.Errorf("ready workflow nats operations: %w", err)
	}
	return host, nil
}

func transportTimeout(cfg natskit.Config) time.Duration {
	if cfg.Timeout > 0 {
		return cfg.Timeout
	}
	return natskit.DefaultTimeout
}
