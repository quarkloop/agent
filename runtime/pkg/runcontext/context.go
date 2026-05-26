// Package runcontext carries runtime correlation identifiers across agent,
// Gateway, Harness, and service-function calls.
package runcontext

import "context"

type key string

const (
	sessionIDKey key = "runtime.session_id"
	runIDKey     key = "runtime.run_id"
	spaceIDKey   key = "runtime.space_id"
)

func WithSessionID(ctx context.Context, sessionID string) context.Context {
	if sessionID == "" {
		return ctx
	}
	return context.WithValue(ctx, sessionIDKey, sessionID)
}

func SessionID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(sessionIDKey).(string)
	return value
}

func WithRunID(ctx context.Context, runID string) context.Context {
	if runID == "" {
		return ctx
	}
	return context.WithValue(ctx, runIDKey, runID)
}

func RunID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(runIDKey).(string)
	return value
}

func WithSpaceID(ctx context.Context, spaceID string) context.Context {
	if spaceID == "" {
		return ctx
	}
	return context.WithValue(ctx, spaceIDKey, spaceID)
}

func SpaceID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(spaceIDKey).(string)
	return value
}
