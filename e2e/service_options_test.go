//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestKnowledgeAgentServicePermissionsMatchStandardE2EStack(t *testing.T) {
	permissions := knowledgeAgentServicePermissions()["quark-main"]
	delegatePermissions := knowledgeAgentServicePermissions()["quark-knowledge"]
	if len(delegatePermissions) != len(permissions) {
		t.Fatalf("knowledge specialist service permissions = %d, main coordinator = %d", len(delegatePermissions), len(permissions))
	}
	required := []string{
		"io_Read", "document_ExtractText", "runstate_StartRun", "gateway_Embed",
		"indexer_UpsertChunk", "citation_VerifyGrounding", "runstate_MarkComplete",
	}
	seen := make(map[string]struct{}, len(permissions))
	for _, function := range permissions {
		if strings.HasPrefix(function, "workflow_") {
			t.Fatalf("standard knowledge e2e stack must not require workflow deployment: %s", function)
		}
		for _, forbidden := range []string{"io_Write", "io_Remove", "indexer_Delete", "core_", "harness_"} {
			if strings.HasPrefix(function, forbidden) {
				t.Fatalf("knowledge E2E exposed out-of-scope service function: %s", function)
			}
		}
		seen[function] = struct{}{}
	}
	for _, function := range required {
		if _, ok := seen[function]; !ok {
			t.Fatalf("standard knowledge e2e stack missing required service function %s", function)
		}
	}
}
