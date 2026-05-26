package runcontext

import (
	"context"
	"testing"
)

func TestCorrelationIdentifiersRoundTrip(t *testing.T) {
	ctx := WithRunID(WithSessionID(WithSpaceID(context.Background(), "space-1"), "session-1"), "run-1")

	if SpaceID(ctx) != "space-1" || SessionID(ctx) != "session-1" || RunID(ctx) != "run-1" {
		t.Fatalf("runtime context values = space:%q session:%q run:%q", SpaceID(ctx), SessionID(ctx), RunID(ctx))
	}
}
