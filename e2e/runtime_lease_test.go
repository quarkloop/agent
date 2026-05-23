//go:build e2e

package e2e

import "testing"

func TestRuntimeSpaceLeaseConflict(t *testing.T) {
	t.Skip("runtime lease conflict E2E is finalized with the NATS-native product flow gate")
}
