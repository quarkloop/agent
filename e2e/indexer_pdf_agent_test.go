//go:build e2e

package e2e

import (
	"os"
	"strings"
	"testing"

	"github.com/quarkloop/e2e/utils"
)

func TestAgentIndexesUploadedPDFDataset(t *testing.T) {
	model := strings.TrimSpace(os.Getenv("OPENROUTER_E2E_EMBEDDING_MODEL"))
	if model == "" {
		t.Fatal("OPENROUTER_E2E_EMBEDDING_MODEL is required for real Gateway embedding E2E execution")
	}
	runAgentIndexesUploadedPDFDataset(t, utils.GatewayEmbeddingOptions{
		Provider: "openrouter",
		Model:    model,
	})
}
