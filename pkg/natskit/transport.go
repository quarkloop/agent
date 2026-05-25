package natskit

import (
	"context"
	"fmt"
	"strings"
	"time"

	natsgo "github.com/nats-io/nats.go"
)

type Message struct {
	Subject string
	Reply   string
	Headers map[string]string
	Data    []byte
}

type Handler func(context.Context, Message) ([]byte, error)
type StreamHandler func(context.Context, Message, *Publisher) error

type Subscription struct {
	sub *natsgo.Subscription
}

func (s *Subscription) Unsubscribe() error {
	if s == nil || s.sub == nil {
		return nil
	}
	return s.sub.Unsubscribe()
}

func (c *Client) Request(ctx context.Context, subject string, data []byte, headers map[string]string) ([]byte, error) {
	if c == nil || c.conn == nil {
		return nil, fmt.Errorf("nats client is not connected")
	}
	subject, err := Subject(subject)
	if err != nil {
		return nil, err
	}
	msg := natsgo.NewMsg(subject)
	msg.Data = append([]byte(nil), data...)
	setHeaders(msg, headers)
	reply, err := c.conn.RequestMsgWithContext(ctx, msg)
	if err != nil {
		return nil, fmt.Errorf("request %s: %w", subject, err)
	}
	return append([]byte(nil), reply.Data...), nil
}

func (c *Client) Publish(ctx context.Context, subject string, data []byte, headers map[string]string) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("nats client is not connected")
	}
	subject, err := Subject(subject)
	if err != nil {
		return err
	}
	msg := natsgo.NewMsg(subject)
	msg.Data = append([]byte(nil), data...)
	setHeaders(msg, headers)
	if err := c.conn.PublishMsg(msg); err != nil {
		return fmt.Errorf("publish %s: %w", subject, err)
	}
	return c.Flush(ctx)
}

// PublishStored publishes an application event through JetStream and waits
// for its storage acknowledgement. Durable resources must already have been
// provisioned by their storage owner.
func (c *Client) PublishStored(ctx context.Context, subject string, data []byte, headers map[string]string) error {
	if c == nil || c.conn == nil {
		return fmt.Errorf("nats client is not connected")
	}
	subject, err := Subject(subject)
	if err != nil {
		return err
	}
	js, err := c.jetStream()
	if err != nil {
		return err
	}
	msg := natsgo.NewMsg(subject)
	msg.Data = append([]byte(nil), data...)
	setHeaders(msg, headers)
	if _, err := js.PublishMsg(msg, natsgo.Context(ctx)); err != nil {
		return fmt.Errorf("persist %s: %w", subject, err)
	}
	return nil
}

func (c *Client) Subscribe(subject string, handler func(Message)) (*Subscription, error) {
	if c == nil || c.conn == nil {
		return nil, fmt.Errorf("nats client is not connected")
	}
	subject, err := SubscriptionSubject(subject)
	if err != nil {
		return nil, err
	}
	sub, err := c.conn.Subscribe(subject, func(msg *natsgo.Msg) {
		handler(messageFromNATS(msg))
	})
	if err != nil {
		return nil, fmt.Errorf("subscribe %s: %w", subject, err)
	}
	return &Subscription{sub: sub}, nil
}

// Respond registers a transient request/reply operation. The queue group is
// deliberately present only on this responder-side API.
func (c *Client) Respond(subject, queue string, timeout time.Duration, handler Handler) (*Subscription, error) {
	if c == nil || c.conn == nil {
		return nil, fmt.Errorf("nats client is not connected")
	}
	subject, err := SubscriptionSubject(subject)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(queue) == "" {
		return nil, fmt.Errorf("responder queue is required for %s", subject)
	}
	if timeout <= 0 {
		timeout = c.cfg.Timeout
	}
	sub, err := c.conn.QueueSubscribe(subject, queue, func(raw *natsgo.Msg) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		response, handlerErr := handler(ctx, messageFromNATS(raw))
		if handlerErr != nil {
			c.cfg.Logger.Error("nats responder handler failed", "subject", subject, "error", handlerErr)
			return
		}
		if raw.Reply != "" {
			if err := raw.Respond(append([]byte(nil), response...)); err != nil {
				c.cfg.Logger.Error("nats responder reply failed", "subject", subject, "error", err)
			}
		}
	})
	if err != nil {
		return nil, fmt.Errorf("register responder %s: %w", subject, err)
	}
	return &Subscription{sub: sub}, nil
}

func (c *Client) RespondStream(subject, queue string, timeout time.Duration, handler StreamHandler) (*Subscription, error) {
	if c == nil || c.conn == nil {
		return nil, fmt.Errorf("nats client is not connected")
	}
	subject, err := SubscriptionSubject(subject)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(queue) == "" {
		return nil, fmt.Errorf("responder queue is required for %s", subject)
	}
	if timeout <= 0 {
		timeout = c.cfg.Timeout
	}
	sub, err := c.conn.QueueSubscribe(subject, queue, func(raw *natsgo.Msg) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := handler(ctx, messageFromNATS(raw), &Publisher{client: c, subject: raw.Reply}); err != nil {
			c.cfg.Logger.Error("nats stream responder failed", "subject", subject, "error", err)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("register streaming responder %s: %w", subject, err)
	}
	return &Subscription{sub: sub}, nil
}

type Publisher struct {
	client  *Client
	subject string
}

func (p *Publisher) Publish(data []byte, headers map[string]string) error {
	if p == nil || p.client == nil || strings.TrimSpace(p.subject) == "" {
		return fmt.Errorf("reply publisher is not available")
	}
	msg := natsgo.NewMsg(p.subject)
	msg.Data = append([]byte(nil), data...)
	setHeaders(msg, headers)
	return p.client.conn.PublishMsg(msg)
}

type ReplyStream struct {
	client *Client
	sub    *natsgo.Subscription
}

func (c *Client) OpenStream(ctx context.Context, subject string, data []byte, headers map[string]string) (*ReplyStream, error) {
	subject, err := Subject(subject)
	if err != nil {
		return nil, err
	}
	inbox := natsgo.NewInbox()
	sub, err := c.conn.SubscribeSync(inbox)
	if err != nil {
		return nil, fmt.Errorf("subscribe stream inbox: %w", err)
	}
	msg := natsgo.NewMsg(subject)
	msg.Reply = inbox
	msg.Data = append([]byte(nil), data...)
	setHeaders(msg, headers)
	if err := c.conn.PublishMsg(msg); err != nil {
		_ = sub.Unsubscribe()
		return nil, fmt.Errorf("publish stream request %s: %w", subject, err)
	}
	if err := c.Flush(ctx); err != nil {
		_ = sub.Unsubscribe()
		return nil, err
	}
	return &ReplyStream{client: c, sub: sub}, nil
}

func (s *ReplyStream) Next(ctx context.Context) ([]byte, error) {
	if s == nil || s.sub == nil {
		return nil, fmt.Errorf("reply stream is closed")
	}
	msg, err := s.sub.NextMsgWithContext(ctx)
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), msg.Data...), nil
}

func (s *ReplyStream) Close() error {
	if s == nil || s.sub == nil {
		return nil
	}
	return s.sub.Unsubscribe()
}

func setHeaders(msg *natsgo.Msg, headers map[string]string) {
	for name, value := range headers {
		if strings.TrimSpace(value) != "" {
			msg.Header.Set(name, value)
		}
	}
}

func messageFromNATS(msg *natsgo.Msg) Message {
	headers := make(map[string]string)
	for name, values := range msg.Header {
		if len(values) > 0 {
			headers[name] = values[0]
		}
	}
	return Message{
		Subject: msg.Subject,
		Reply:   msg.Reply,
		Headers: headers,
		Data:    append([]byte(nil), msg.Data...),
	}
}
