//go:build e2e

package e2e

import (
	"testing"
)

func TestDevOpsTraceOptionsAreCentralized(t *testing.T) {
	opts := devOpsServiceTraceOptions("devops flow")
	if opts.OverallTimeout != devOpsServiceFlowTimeout {
		t.Fatalf("overall timeout = %s, want %s", opts.OverallTimeout, devOpsServiceFlowTimeout)
	}
	if opts.IdleTimeout <= 0 {
		t.Fatal("idle timeout must be explicit")
	}
}
