package natskit

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/quarkloop/pkg/boundary"
)

type Host struct {
	client   *Client
	recorder *Recorder
	queue    string
	subs     []*Subscription
}

type UnaryServiceHandler func(context.Context, RequestEnvelope) (ResponseEnvelope, error)
type StreamServiceHandler func(context.Context, RequestEnvelope, func(ResponseEnvelope) error) (ResponseEnvelope, error)

func NewHost(ctx context.Context, cfg Config, queue string) (*Host, error) {
	if strings.TrimSpace(queue) == "" {
		return nil, fmt.Errorf("nats responder queue is required")
	}
	client, err := Connect(ctx, cfg)
	if err != nil {
		return nil, err
	}
	recorder, err := newRecorder(client)
	if err != nil {
		client.Close()
		return nil, err
	}
	return &Host{client: client, recorder: recorder, queue: strings.TrimSpace(queue)}, nil
}

func (h *Host) RegisterUnary(operation Operation, timeout time.Duration, handler UnaryServiceHandler) error {
	if err := validateOperation(operation); err != nil {
		return err
	}
	sub, err := h.client.Respond(operation.Subject, h.queue, timeout, func(ctx context.Context, msg Message) ([]byte, error) {
		started := time.Now()
		req, err := decodeRequest(msg.Data)
		if err != nil {
			resp := ErrorResponse("", err, boundary.Service, operation.Subject)
			return encodeResponse(resp)
		}
		var resp ResponseEnvelope
		if err := req.Validate(); err != nil {
			resp = ErrorResponse(req.ServiceCallID, err, boundary.Service, operation.Subject)
		} else {
			resp, err = handler(ctx, req.Clone())
			if err != nil {
				resp = ErrorResponse(req.ServiceCallID, err, boundary.Service, operation.Subject)
			}
		}
		resp = resp.WithTraceParent(req.TraceParent)
		if err := h.recorder.Record(req, operation, resp, time.Since(started)); err != nil {
			h.client.cfg.Logger.Error("record service call", "subject", operation.Subject, "reference_id", resp.ReferenceID, "error", err)
		}
		return encodeResponse(resp)
	})
	if err != nil {
		return err
	}
	h.subs = append(h.subs, sub)
	return nil
}

func (h *Host) RegisterStream(operation Operation, timeout time.Duration, handler StreamServiceHandler) error {
	if err := validateOperation(operation); err != nil {
		return err
	}
	sub, err := h.client.RespondStream(operation.Subject, h.queue, timeout, func(ctx context.Context, msg Message, publisher *Publisher) error {
		started := time.Now()
		req, err := decodeRequest(msg.Data)
		if err != nil {
			resp := ErrorResponse("", err, boundary.Service, operation.Subject)
			resp.Final = true
			return publishEnvelope(publisher, resp)
		}
		if err := req.Validate(); err != nil {
			resp := ErrorResponse(req.ServiceCallID, err, boundary.Service, operation.Subject).WithTraceParent(req.TraceParent)
			resp.Final = true
			_ = h.recorder.Record(req, operation, resp, time.Since(started))
			return publishEnvelope(publisher, resp)
		}
		publish := func(resp ResponseEnvelope) error {
			return publishEnvelope(publisher, resp.WithTraceParent(req.TraceParent))
		}
		terminal, err := handler(ctx, req.Clone(), publish)
		if err != nil {
			terminal = ErrorResponse(req.ServiceCallID, err, boundary.Service, operation.Subject)
		}
		terminal = terminal.WithTraceParent(req.TraceParent)
		terminal.Final = true
		if err := publishEnvelope(publisher, terminal); err != nil {
			return err
		}
		if err := h.recorder.Record(req, operation, terminal, time.Since(started)); err != nil {
			h.client.cfg.Logger.Error("record streaming service call", "subject", operation.Subject, "reference_id", terminal.ReferenceID, "error", err)
		}
		return nil
	})
	if err != nil {
		return err
	}
	h.subs = append(h.subs, sub)
	return nil
}

func (h *Host) Ready(ctx context.Context) error {
	if h == nil || h.client == nil {
		return fmt.Errorf("nats host is not configured")
	}
	return h.client.Flush(ctx)
}

func (h *Host) Close() {
	if h == nil {
		return
	}
	for _, sub := range h.subs {
		_ = sub.Unsubscribe()
	}
	h.subs = nil
	if h.client != nil {
		h.client.Close()
		h.client = nil
	}
}

func (c *Client) Call(ctx context.Context, operation Operation, req RequestEnvelope) (ResponseEnvelope, error) {
	if err := validateOperation(operation); err != nil {
		return ResponseEnvelope{}, err
	}
	if err := req.Validate(); err != nil {
		return ResponseEnvelope{}, err
	}
	data, err := json.Marshal(req)
	if err != nil {
		return ResponseEnvelope{}, fmt.Errorf("marshal service request: %w", err)
	}
	reply, err := c.Request(ctx, operation.Subject, data, req.CorrelationHeaders())
	if err != nil {
		return ResponseEnvelope{}, err
	}
	var resp ResponseEnvelope
	if err := json.Unmarshal(reply, &resp); err != nil {
		return ResponseEnvelope{}, fmt.Errorf("decode service response: %w", err)
	}
	if err := resp.Validate(); err != nil {
		return ResponseEnvelope{}, err
	}
	return resp, nil
}

func (c *Client) OpenServiceStream(ctx context.Context, operation Operation, req RequestEnvelope) (*ReplyStream, error) {
	if err := validateOperation(operation); err != nil {
		return nil, err
	}
	if err := req.Validate(); err != nil {
		return nil, err
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal streaming service request: %w", err)
	}
	return c.OpenStream(ctx, operation.Subject, data, req.CorrelationHeaders())
}

func DecodeServiceResponse(data []byte) (ResponseEnvelope, error) {
	var resp ResponseEnvelope
	if err := json.Unmarshal(data, &resp); err != nil {
		return ResponseEnvelope{}, fmt.Errorf("decode service response: %w", err)
	}
	if err := resp.Validate(); err != nil {
		return ResponseEnvelope{}, err
	}
	return resp, nil
}

func decodeRequest(data []byte) (RequestEnvelope, error) {
	var req RequestEnvelope
	if err := json.Unmarshal(data, &req); err != nil {
		return RequestEnvelope{}, err
	}
	return req, nil
}

func encodeResponse(resp ResponseEnvelope) ([]byte, error) {
	if err := resp.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(resp)
}

func publishEnvelope(publisher *Publisher, resp ResponseEnvelope) error {
	data, err := encodeResponse(resp)
	if err != nil {
		return err
	}
	return publisher.Publish(data, nil)
}

func validateOperation(operation Operation) error {
	parsed, err := ParseServiceOperation(operation.Subject)
	if err != nil {
		return err
	}
	if operation.Owner != parsed.Owner || operation.Function != parsed.Function {
		return fmt.Errorf("operation metadata does not match concrete subject %q", operation.Subject)
	}
	return nil
}
