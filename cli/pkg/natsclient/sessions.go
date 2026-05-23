package natsclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go"
	"github.com/quarkloop/pkg/serviceapi/clientcontract"
)

func (c *Client) CreateSession(ctx context.Context, req clientcontract.CreateSessionRequest) (clientcontract.SessionInfo, error) {
	return requestPayload[clientcontract.SessionInfo](ctx, c, clientcontract.SubjectSessionCreate, req.SpaceID, req)
}

func (c *Client) ListSessions(ctx context.Context, spaceID string) (clientcontract.ListSessionsResponse, error) {
	return requestPayload[clientcontract.ListSessionsResponse](ctx, c, clientcontract.SubjectSessionList, spaceID, clientcontract.ListSessionsRequest{SpaceID: spaceID})
}

func (c *Client) GetSession(ctx context.Context, spaceID, sessionID string) (clientcontract.SessionInfo, error) {
	return requestPayload[clientcontract.SessionInfo](ctx, c, clientcontract.SubjectSessionGet, spaceID, clientcontract.SessionRefRequest{
		SpaceID:   spaceID,
		SessionID: sessionID,
	})
}

func (c *Client) DeleteSession(ctx context.Context, spaceID, sessionID string) error {
	_, err := requestPayload[struct{}](ctx, c, clientcontract.SubjectSessionDelete, spaceID, clientcontract.SessionRefRequest{
		SpaceID:   spaceID,
		SessionID: sessionID,
	})
	return err
}

func (c *Client) IssueSessionCredential(ctx context.Context, spaceID, sessionID string) (clientcontract.NATSCredential, error) {
	resp, err := requestPayload[clientcontract.SessionCredentialResponse](ctx, c, clientcontract.SubjectSessionCredential, spaceID, clientcontract.SessionCredentialRequest{
		SpaceID:   spaceID,
		SessionID: sessionID,
	})
	if err != nil {
		return clientcontract.NATSCredential{}, err
	}
	return resp.Credential, nil
}

func (c *Client) SendSessionMessage(ctx context.Context, req clientcontract.SendMessageRequest) (clientcontract.SendMessageResponse, error) {
	subject, err := clientcontract.SessionInputSubject(req.SessionID)
	if err != nil {
		return clientcontract.SendMessageResponse{}, err
	}
	return requestPayload[clientcontract.SendMessageResponse](ctx, c, subject, req.SpaceID, req)
}

func (c *Client) SubscribeSessionEvents(ctx context.Context, sessionID string) (<-chan clientcontract.SessionEvent, <-chan error, func(), error) {
	if c == nil || c.conn == nil {
		return nil, nil, nil, errors.New("nats client is not connected")
	}
	subject, err := clientcontract.SessionEventsSubject(sessionID)
	if err != nil {
		return nil, nil, nil, err
	}
	events := make(chan clientcontract.SessionEvent, 64)
	errs := make(chan error, 8)
	sub, err := c.conn.Subscribe(subject, func(msg *nats.Msg) {
		var event clientcontract.SessionEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			notifySubscriptionError(errs, fmt.Errorf("decode session event: %w", err))
			return
		}
		event.Payload = append(json.RawMessage(nil), event.Payload...)
		select {
		case events <- event:
		case <-ctx.Done():
		}
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("subscribe %s: %w", subject, err)
	}
	stopOnce := make(chan struct{})
	stop := func() {
		select {
		case <-stopOnce:
			return
		default:
			close(stopOnce)
			_ = sub.Unsubscribe()
		}
	}
	go func() {
		select {
		case <-ctx.Done():
			stop()
		case <-stopOnce:
		}
	}()
	if err := c.flush(ctx); err != nil {
		stop()
		return nil, nil, nil, fmt.Errorf("flush session event subscription: %w", err)
	}
	return events, errs, stop, nil
}
