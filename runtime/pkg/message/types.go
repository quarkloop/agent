package message

import "context"

// Poster posts messages to the agent inbox.
type Poster interface {
	Post(ctx context.Context, request PostRequest, resp chan StreamMessage)
}

// PostRequest is the runtime-owned request to post a user message into a
// session. Transport DTOs must be mapped to this type at channel/API edges.
type PostRequest struct {
	SessionID string
	Content   string
}

// SessionAccess provides session state for message handlers.
type SessionAccess interface {
	Has(id string) bool
	GetMessages(id string) []Message
	Subscribe(id string) chan Message
	Unsubscribe(id string, ch chan Message)
}
