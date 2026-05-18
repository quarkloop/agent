package modelservice

import "context"

type contextKey string

const (
	sessionIDKey contextKey = "modelservice.session_id"
	runIDKey     contextKey = "modelservice.run_id"
	spaceIDKey   contextKey = "modelservice.space_id"
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

// WithSpaceID returns a child context that binds the current Quark space to
// runtime-owned operations. Services that expose a top-level `space` field get
// this value from runtime context instead of asking the LLM to invent it.
func WithSpaceID(ctx context.Context, spaceID string) context.Context {
	if spaceID == "" {
		return ctx
	}
	return context.WithValue(ctx, spaceIDKey, spaceID)
}

// SpaceID returns the Quark space identifier bound to ctx.
func SpaceID(ctx context.Context) string {
	value, _ := ctx.Value(spaceIDKey).(string)
	return value
}
