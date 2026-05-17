package rtclient

import (
	"context"
	"fmt"
	"net/url"
)

// SessionMessage is the runtime API representation of a message in a session.
type SessionMessage struct {
	ID        string `json:"id,omitempty"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp,omitempty"`
}

type postSessionMessageRequest struct {
	Content string `json:"content"`
}

// ListSessionMessages returns the persisted messages for a runtime session.
func (c *Client) ListSessionMessages(ctx context.Context, sessionID string) ([]SessionMessage, error) {
	var resp []SessionMessage
	if err := c.transport.Get(ctx, sessionMessagesPath(sessionID), &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// PostSessionMessage streams one user message through the runtime's primary
// session message endpoint. The caller owns output formatting.
func (c *Client) PostSessionMessage(ctx context.Context, sessionID, content string, fn func(SSEEvent) error) error {
	req := postSessionMessageRequest{Content: content}
	if err := c.transport.PostSSE(ctx, sessionMessagesPath(sessionID), req, fn); err != nil {
		return fmt.Errorf("post session message: %w", err)
	}
	return nil
}

func sessionMessagesPath(sessionID string) string {
	return "/v1/sessions/" + url.PathEscape(sessionID) + "/messages"
}
