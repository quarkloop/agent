//go:build e2e

package e2e

import (
	"strings"
	"testing"

	"github.com/quarkloop/e2e/utils"
)

func TestRuntimeSpaceLeaseConflict(t *testing.T) {
	env := utils.StartE2E(t, false, utils.StartOptions{DisableKnowledgeServices: true})
	output := env.Compose.RunServiceExpectFailure("runtime", map[string]string{
		"QUARK_RUNTIME_ID": "e2e-competing-runtime",
	})
	if !strings.Contains(output, "already leased") {
		t.Fatalf("second runtime did not expose lease conflict diagnostic:\n%s", output)
	}
}
