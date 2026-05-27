//go:build e2e

package e2e

import (
	"path/filepath"
	"testing"

	"github.com/quarkloop/e2e/utils"
)

func TestServicePluginInventoryHasE2ECoveragePlan(t *testing.T) {
	root := utils.QuarkRoot(t)
	manifests, err := filepath.Glob(filepath.Join(root, "plugins", "services", "*", "manifest.yaml"))
	if err != nil {
		t.Fatalf("glob service manifests: %v", err)
	}
	if len(manifests) == 0 {
		t.Fatal("no service plugin manifests found")
	}
	e2eTests := listE2ETests(t, root)

	expectations := map[string]serviceCoverageExpectation{
		"citation": {
			Mode:  "contract-only",
			Tests: []string{"TestAgentIndexesUploadedPDFDataset"},
		},
		"core": {
			Mode:  "supervisor-runtime-owned",
			Tests: []string{"TestAgentIndexesUploadedPDFDataset", "TestAgentRunArtifactsAreRedactedAndStructured"},
		},
		"devops": {
			Mode:  "runtime-backed",
			Tests: []string{"TestAgentUsesDevOpsReleaseServiceFunction"},
		},
		"document": {
			Mode:  "runtime-backed",
			Tests: []string{"TestAgentIndexesUploadedPDFDataset"},
		},
		"gateway": {
			Mode:  "runtime-backed",
			Tests: []string{"TestAgentIndexesUploadedPDFDataset"},
		},
		"indexer": {
			Mode:  "runtime-backed",
			Tests: []string{"TestAgentIndexesUploadedPDFDataset"},
		},
		"runstate": {
			Mode:  "runtime-backed",
			Tests: []string{"TestAgentIndexesUploadedPDFDataset"},
		},
		"io": {
			Mode:  "runtime-backed",
			Tests: []string{"TestAgentIndexesUploadedPDFDataset", "TestAgentUsesDevOpsReleaseServiceFunction"},
		},
		"harness": {
			Mode:  "contract-only",
			Tests: []string{"TestAgentIndexesUploadedPDFDataset"},
		},
		"secrets": {
			Mode:  "runtime-backed",
			Tests: []string{"TestSecretsServiceNATSContract"},
		},
		"space": {
			Mode:  "supervisor-runtime-owned",
			Tests: []string{"TestAgentIndexesUploadedPDFDataset"},
		},
		"system": {
			Mode:  "runtime-backed",
			Tests: []string{"TestAgentUsesSystemServiceForReadOnlyInspection"},
		},
		"workflow": {
			Mode:  "runtime-backed",
			Tests: []string{"TestWorkflowServiceNATSContract"},
		},
	}

	seen := make(map[string]bool, len(manifests))
	for _, manifestPath := range manifests {
		service := loadServiceManifest(t, manifestPath)
		expectation, ok := expectations[service.Name]
		if !ok {
			t.Fatalf("service %q has no E2E coverage expectation", service.Name)
		}
		if expectation.Mode == "" || len(expectation.Tests) == 0 {
			t.Fatalf("service %q has incomplete coverage expectation: %+v", service.Name, expectation)
		}
		for _, testName := range expectation.Tests {
			if !e2eTests[testName] {
				t.Fatalf("service %q expects missing E2E test %s", service.Name, testName)
			}
		}
		assertServiceDocumentationCoversFunctions(t, manifestPath, service)
		seen[service.Name] = true
	}

	for name := range expectations {
		if !seen[name] {
			t.Fatalf("coverage expectation %q has no service manifest", name)
		}
	}
}
