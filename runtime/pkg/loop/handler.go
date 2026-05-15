package loop

import "context"

// HandlerFunc is the signature for message handlers.
type HandlerFunc func(ctx context.Context, msg Message) error

// Handler is the interface for stateful message handlers.
// Use HandlerFunc for simple function handlers.
type Handler interface {
	// Type returns the message type this handler processes.
	Type() string

	// Handle processes a message.
	Handle(ctx context.Context, msg Message) error
}

// TypedHandler creates a Handler from a type string and HandlerFunc.
type TypedHandler struct {
	MsgType  string
	HandleFn HandlerFunc
}

// Type returns the message type.
func (h TypedHandler) Type() string { return h.MsgType }

// Handle processes the message.
func (h TypedHandler) Handle(ctx context.Context, msg Message) error {
	return h.HandleFn(ctx, msg)
}
