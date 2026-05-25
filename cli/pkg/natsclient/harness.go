package natsclient

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/quarkloop/pkg/boundary"
	"github.com/quarkloop/pkg/natskit"
	harnessv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/harness/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func (c *Client) GetContextReport(ctx context.Context, spaceID, reportID string) (*harnessv1.ContextReport, error) {
	var report harnessv1.ContextReport
	err := callHarness(ctx, c, spaceID, "get_context_report", &harnessv1.GetContextReportRequest{
		Space: spaceID, Id: reportID,
	}, &report)
	if err != nil {
		return nil, err
	}
	return &report, nil
}

func (c *Client) ContextReports(ctx context.Context, spaceID, sessionID string, limit int32) ([]*harnessv1.ContextReport, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("nats client is not connected")
	}
	payload, err := protojson.Marshal(&harnessv1.StreamContextReportsRequest{Space: spaceID, SessionId: sessionID, Limit: limit})
	if err != nil {
		return nil, fmt.Errorf("marshal Harness stream request: %w", err)
	}
	request, err := natskit.NewRequest(natskit.NewServiceCallID(), spaceID, natskit.ActorUser, json.RawMessage(payload))
	if err != nil {
		return nil, err
	}
	operation, err := natskit.ServiceOperation("harness", "stream_context_reports")
	if err != nil {
		return nil, err
	}
	stream, err := c.client.OpenServiceStream(ctx, operation, request)
	if err != nil {
		return nil, fmt.Errorf("open Harness context report stream: %w", err)
	}
	defer stream.Close()
	var reports []*harnessv1.ContextReport
	for {
		data, err := stream.Next(ctx)
		if err != nil {
			return nil, fmt.Errorf("read Harness context report stream: %w", err)
		}
		envelope, err := natskit.DecodeServiceResponse(data)
		if err != nil {
			return nil, err
		}
		if envelope.Status == natskit.StatusError {
			return nil, harnessResponseError(envelope)
		}
		if envelope.Final {
			return reports, nil
		}
		var report harnessv1.ContextReport
		if err := protojson.Unmarshal(envelope.Payload, &report); err != nil {
			return nil, fmt.Errorf("decode Harness context report: %w", err)
		}
		reports = append(reports, &report)
	}
}

func callHarness(ctx context.Context, c *Client, spaceID, function string, input, output proto.Message) error {
	if c == nil || c.client == nil {
		return fmt.Errorf("nats client is not connected")
	}
	payload, err := protojson.Marshal(input)
	if err != nil {
		return fmt.Errorf("marshal Harness request: %w", err)
	}
	request, err := natskit.NewRequest(natskit.NewServiceCallID(), spaceID, natskit.ActorUser, json.RawMessage(payload))
	if err != nil {
		return err
	}
	operation, err := natskit.ServiceOperation("harness", function)
	if err != nil {
		return err
	}
	envelope, err := c.client.Call(ctx, operation, request)
	if err != nil {
		return err
	}
	if envelope.Status == natskit.StatusError {
		return harnessResponseError(envelope)
	}
	if err := protojson.Unmarshal(envelope.Payload, output); err != nil {
		return fmt.Errorf("decode Harness response: %w", err)
	}
	return nil
}

func harnessResponseError(envelope natskit.ResponseEnvelope) error {
	if envelope.Error == nil {
		return &ResponseError{Category: boundary.Internal, Message: "missing Harness response error"}
	}
	return &ResponseError{Category: envelope.Error.Category, Message: envelope.Error.Message}
}
