//go:build e2e

package e2e

import (
	"testing"
	"time"

	"github.com/quarkloop/e2e/utils"
)

const systemServiceFlowTimeout = 3 * time.Minute

func systemServiceTraceOptions(label string) utils.MessageTraceOptions {
	return utils.MessageTraceOptions{
		Label:          label,
		OverallTimeout: systemServiceFlowTimeout,
		IdleTimeout:    90 * time.Second,
	}
}

func TestSystemTraceOptionsAreCentralized(t *testing.T) {
	opts := systemServiceTraceOptions("system")
	if opts.OverallTimeout != systemServiceFlowTimeout {
		t.Fatalf("overall timeout = %s, want %s", opts.OverallTimeout, systemServiceFlowTimeout)
	}
	if opts.IdleTimeout <= 0 {
		t.Fatal("idle timeout must be configured")
	}
}
