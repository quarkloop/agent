package rtclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

type Client struct {
	transport *Transport
	basePath  string
}

type ClientOption func(*Client)

func New(baseURL string, opts ...ClientOption) *Client {
	c := &Client{
		transport: NewTransport(baseURL),
		basePath:  defaultRuntimeBasePath,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func WithTransport(transport *Transport) ClientOption {
	return func(c *Client) {
		if transport != nil {
			c.transport = transport
		}
	}
}

func WithBasePath(basePath string) ClientOption {
	return func(c *Client) {
		if basePath != "" {
			c.basePath = basePath
		}
	}
}

func (c *Client) Transport() *Transport {
	return c.transport
}

func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	var resp HealthResponse
	if err := c.transport.Get(ctx, c.path(pathHealth), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Info(ctx context.Context) (*InfoResponse, error) {
	var resp InfoResponse
	if err := c.transport.Get(ctx, c.path(pathInfo), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Mode(ctx context.Context) (*ModeResponse, error) {
	var resp ModeResponse
	if err := c.transport.Get(ctx, c.path(pathMode), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) SetMode(ctx context.Context, mode string) (*ModeResponse, error) {
	var resp ModeResponse
	if err := c.transport.Post(ctx, c.path(pathMode), setModeRequest{Mode: mode}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Stats(ctx context.Context) (*StatsResponse, error) {
	var resp StatsResponse
	if err := c.transport.Get(ctx, c.path(pathStats), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	var resp ChatResponse
	if err := c.transport.Post(ctx, c.path(pathChat), req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Stop(ctx context.Context) error {
	return c.transport.Post(ctx, c.path(pathStop), nil, nil)
}

func (c *Client) Plan(ctx context.Context) (*PlanResponse, error) {
	var resp PlanResponse
	if err := c.transport.Get(ctx, "/v1/plan", &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) Activity(ctx context.Context, limit int) ([]ActivityRecord, error) {
	path := "/v1/activity"
	if limit > 0 {
		path = fmt.Sprintf("%s?limit=%d", path, limit)
	}
	var resp []ActivityRecord
	if err := c.transport.Get(ctx, path, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) ApprovePlan(ctx context.Context, planID string) (*PlanResponse, error) {
	var resp PlanResponse
	if err := c.transport.Post(ctx, "/v1/plan/approve", planActionRequest{PlanID: planID}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *Client) RejectPlan(ctx context.Context, planID string) error {
	return c.transport.Post(ctx, "/v1/plan/reject", planActionRequest{PlanID: planID}, nil)
}

func (c *Client) StreamActivity(ctx context.Context, fn func(ActivityRecord)) error {
	return c.transport.StreamSSEEvents(ctx, "/v1/activity/stream", func(event SSEEvent) error {
		var record ActivityRecord

		if err := json.Unmarshal(event.Data, &record); err != nil {
			fn(ActivityRecord{
				Type: "stream.decode_error",
				Data: mustRawJSON(map[string]string{"error": err.Error(), "chunk": string(event.Data)}),
			})
			return nil
		}

		fn(record)
		return nil
	})
}

func (c *Client) path(suffix string) string {
	return joinPath(c.basePath, suffix)
}

func mustRawJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{"error":"marshal failure"}`)
	}
	return data
}

func IsCanceled(err error) bool {
	return errors.Is(err, context.Canceled)
}
