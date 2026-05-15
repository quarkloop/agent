package modelservice

import "context"

type contextKey string

const (
	sessionIDKey contextKey = "modelservice.session_id"
	runIDKey     contextKey = "modelservice.run_id"
)

// WithSessionID returns a child context that binds model usage to a runtime
// session. The model service only records the identifier; it never stores
// prompt text or tool arguments.
func WithSessionID(ctx context.Context, sessionID string) context.Context {
	if sessionID == "" {
		return ctx
	}
	return context.WithValue(ctx, sessionIDKey, sessionID)
}

// SessionID returns the runtime session identifier bound to ctx.
func SessionID(ctx context.Context) string {
	value, _ := ctx.Value(sessionIDKey).(string)
	return value
}

// WithRunID returns a child context that binds model usage to a logical run.
func WithRunID(ctx context.Context, runID string) context.Context {
	if runID == "" {
		return ctx
	}
	return context.WithValue(ctx, runIDKey, runID)
}

// RunID returns the logical run identifier bound to ctx.
func RunID(ctx context.Context) string {
	value, _ := ctx.Value(runIDKey).(string)
	return value
}
