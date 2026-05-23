//go:build e2e

package e2e

import "testing"

func TestSecretsServiceNATSContract(t *testing.T) {
	t.Skip("secrets service deployment E2E is finalized with the NATS-native product flow gate")
}
