//go:build e2e

package e2e

import (
	"testing"
)

func TestSystemTraceOptionsAreCentralized(t *testing.T) {
	opts := systemServiceTraceOptions("system")
	if opts.OverallTimeout != systemServiceFlowTimeout {
		t.Fatalf("overall timeout = %s, want %s", opts.OverallTimeout, systemServiceFlowTimeout)
	}
	if opts.IdleTimeout <= 0 {
		t.Fatal("idle timeout must be configured")
	}
}
