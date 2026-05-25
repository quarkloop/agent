package natskit

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ApplicationOperation identifies a non-service request/reply protocol
// operation, such as a supervisor or runtime client contract endpoint.
// Protocol packages own the subject values; natskit owns their transport.
type ApplicationOperation struct {
	Name    string
	Subject string
}

// ApplicationEvent identifies a live Core NATS event destination. Durable
// replay remains a JetStream storage-owner concern and is not implied here.
type ApplicationEvent struct {
	Name    string
	Subject string
	Durable bool
}

func NewApplicationOperation(name, subject string) (ApplicationOperation, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ApplicationOperation{}, fmt.Errorf("application operation name is required")
	}
	subject, err := SubscriptionSubject(subject)
	if err != nil {
		return ApplicationOperation{}, err
	}
	return ApplicationOperation{Name: name, Subject: subject}, nil
}

func NewApplicationEvent(name, subject string) (ApplicationEvent, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ApplicationEvent{}, fmt.Errorf("application event name is required")
	}
	subject, err := Subject(subject)
	if err != nil {
		return ApplicationEvent{}, err
	}
	return ApplicationEvent{Name: name, Subject: subject}, nil
}

// NewDurableApplicationEvent constructs an event that must receive a
// JetStream storage acknowledgement before publication is reported complete.
func NewDurableApplicationEvent(name, subject string) (ApplicationEvent, error) {
	event, err := NewApplicationEvent(name, subject)
	if err != nil {
		return ApplicationEvent{}, err
	}
	event.Durable = true
	return event, nil
}

// ApplicationHost owns NATS connection, responder registrations, event
// publication, readiness, and draining for a non-service application process.
type ApplicationHost struct {
	client  *Client
	queue   string
	timeout time.Duration
	subs    []*Subscription
}

func NewApplicationHost(ctx context.Context, cfg Config, queue string) (*ApplicationHost, error) {
	if strings.TrimSpace(queue) == "" {
		return nil, fmt.Errorf("application responder queue is required")
	}
	client, err := Connect(ctx, cfg)
	if err != nil {
		return nil, err
	}
	cfg = normalizeConfig(cfg)
	return &ApplicationHost{client: client, queue: strings.TrimSpace(queue), timeout: cfg.Timeout}, nil
}

func (h *ApplicationHost) Register(operation ApplicationOperation, handler Handler) error {
	if h == nil || h.client == nil {
		return fmt.Errorf("nats application host is not configured")
	}
	if strings.TrimSpace(operation.Name) == "" {
		return fmt.Errorf("application operation name is required")
	}
	if _, err := SubscriptionSubject(operation.Subject); err != nil {
		return err
	}
	sub, err := h.client.Respond(operation.Subject, h.queue, h.timeout, handler)
	if err != nil {
		return fmt.Errorf("register application operation %s: %w", operation.Name, err)
	}
	h.subs = append(h.subs, sub)
	return nil
}

func (h *ApplicationHost) Publish(ctx context.Context, event ApplicationEvent, data []byte, headers map[string]string) error {
	if h == nil || h.client == nil {
		return fmt.Errorf("nats application host is not configured")
	}
	if strings.TrimSpace(event.Name) == "" {
		return fmt.Errorf("application event name is required")
	}
	if _, err := Subject(event.Subject); err != nil {
		return err
	}
	publish := h.client.Publish
	if event.Durable {
		publish = h.client.PublishStored
	}
	if err := publish(ctx, event.Subject, data, headers); err != nil {
		return fmt.Errorf("publish application event %s: %w", event.Name, err)
	}
	return nil
}

func (h *ApplicationHost) Ready(ctx context.Context) error {
	if h == nil || h.client == nil {
		return fmt.Errorf("nats application host is not configured")
	}
	return h.client.Flush(ctx)
}

func (h *ApplicationHost) Close() {
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
