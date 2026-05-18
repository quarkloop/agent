//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/quarkloop/e2e/utils"
)

const devOpsServiceFlowTimeout = 5 * time.Minute

func devOpsServiceTraceOptions(label string) utils.MessageTraceOptions {
	return utils.MessageTraceOptions{
		Label:          label,
		OverallTimeout: devOpsServiceFlowTimeout,
		IdleTimeout:    90 * time.Second,
	}
}

func TestDevOpsTraceOptionsAreCentralized(t *testing.T) {
	opts := devOpsServiceTraceOptions("devops flow")
	if opts.OverallTimeout != devOpsServiceFlowTimeout {
		t.Fatalf("overall timeout = %s, want %s", opts.OverallTimeout, devOpsServiceFlowTimeout)
	}
	if opts.IdleTimeout <= 0 {
		t.Fatal("idle timeout must be explicit")
	}
}
