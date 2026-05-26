//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestKnowledgeAgentServicePermissionsMatchStandardE2EStack(t *testing.T) {
	permissions := knowledgeAgentServicePermissions()["quark-main"]
	required := []string{
		"io_Read", "document_ExtractText", "runstate_StartRun", "gateway_Embed",
		"indexer_UpsertChunk", "citation_VerifyGrounding", "core_RecordAuditEvent",
		"harness_GetContextReport",
	}
	seen := make(map[string]struct{}, len(permissions))
	for _, function := range permissions {
		if strings.HasPrefix(function, "workflow_") {
			t.Fatalf("standard knowledge e2e stack must not require workflow deployment: %s", function)
		}
		seen[function] = struct{}{}
	}
	for _, function := range required {
		if _, ok := seen[function]; !ok {
			t.Fatalf("standard knowledge e2e stack missing required service function %s", function)
		}
	}
}
