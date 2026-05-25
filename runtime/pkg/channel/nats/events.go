package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/quarkloop/pkg/natskit"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	"github.com/quarkloop/runtime/pkg/activity"
	"github.com/quarkloop/runtime/pkg/message"
)

func (c *Channel) postAndStream(ctx context.Context, req clientcontract.RequestEnvelope, payload clientcontract.SendMessageRequest) {
	resp := make(chan message.StreamMessage, 64)
	c.poster.Post(ctx, message.PostRequest{
		SpaceID:   payload.SpaceID,
		SessionID: payload.SessionID,
		Content:   payload.Content,
	}, resp)
	for stream := range resp {
		if err := c.publishStreamEvent(req, payload.SessionID, stream); err != nil {
			slog.Error("publish nats session event", "session_id", payload.SessionID, "type", stream.Type, "error", err)
			return
		}
	}
	if err := c.publishEvent(req, clientcontract.SessionEvent{Type: "done", SessionID: payload.SessionID}); err != nil {
		slog.Error("publish nats session done event", "session_id", payload.SessionID, "error", err)
	}
}

func (c *Channel) publishStreamEvent(req clientcontract.RequestEnvelope, sessionID string, stream message.StreamMessage) error {
	payload, err := json.Marshal(stream.Data)
	if err != nil {
		return fmt.Errorf("marshal stream payload: %w", err)
	}
	return c.publishEvent(req, clientcontract.SessionEvent{
		Type:      stream.Type,
		SessionID: sessionID,
		Payload:   append(json.RawMessage(nil), payload...),
	})
}

func (c *Channel) publishEvent(req clientcontract.RequestEnvelope, event clientcontract.SessionEvent) error {
	c.mu.Lock()
	host := c.host
	c.mu.Unlock()
	if host == nil {
		return errors.New("nats runtime channel is not connected")
	}
	subject, err := clientcontract.SessionEventsSubject(event.SessionID)
	if err != nil {
		return err
	}
	event.RequestID = req.RequestID
	event.SpaceID = req.SpaceID
	event.TraceParent = req.TraceParent
	event.TraceState = req.TraceState
	event.Payload = append(json.RawMessage(nil), event.Payload...)
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal session event: %w", err)
	}
	route, err := natskit.NewDurableApplicationEvent("session.events", subject)
	if err != nil {
		return err
	}
	if err := host.Publish(c.requestContext(), route, data, req.CorrelationHeaders()); err != nil {
		return fmt.Errorf("publish %s: %w", subject, err)
	}
	return nil
}

func (c *Channel) forwardActivity(ctx context.Context) {
	if c.activity == nil {
		return
	}
	records := c.activity.Subscribe()
	go func() {
		defer c.activity.Unsubscribe(records)
		for {
			select {
			case record, ok := <-records:
				if !ok {
					return
				}
				if err := c.publishActivity(record); err != nil {
					slog.Error("publish runtime activity event", "id", record.ID, "error", err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

func (c *Channel) publishActivity(record activity.Record) error {
	c.mu.Lock()
	host := c.host
	c.mu.Unlock()
	if host == nil {
		return errors.New("nats runtime channel is not connected")
	}
	payload := mapActivityRecord(record)
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal activity record: %w", err)
	}
	event, err := natskit.NewDurableApplicationEvent("runtime.activity.events", clientcontract.SubjectRuntimeActivityFeed)
	if err != nil {
		return err
	}
	if err := host.Publish(c.requestContext(), event, data, map[string]string{natskit.HeaderSessionID: record.SessionID}); err != nil {
		return fmt.Errorf("publish %s: %w", clientcontract.SubjectRuntimeActivityFeed, err)
	}
	return nil
}
