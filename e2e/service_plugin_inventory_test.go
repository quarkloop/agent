//go:build e2e

package e2e

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/quarkloop/e2e/utils"
	"github.com/quarkloop/pkg/plugin"
)

type serviceCoverageExpectation struct {
	Mode  string
	Tests []string
}

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
			Tests: []string{"TestAgentIndexesUploadedPDFDataset", "TestAgentIndexesITCompanyMarkdownDocuments"},
		},
		"core": {
			Mode:  "supervisor-runtime-owned",
			Tests: []string{"TestSupervisorSessionEventReachesAgent", "TestAgentRunArtifactsAreRedactedAndStructured"},
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
			Tests: []string{"TestAskMode", "TestAgentIndexesUploadedPDFDataset", "TestAgentIndexesITCompanyMarkdownDocuments"},
		},
		"indexer": {
			Mode:  "runtime-backed",
			Tests: []string{"TestIndexerServiceWithRealDgraph", "TestAgentIndexesUploadedPDFDataset", "TestAgentIndexesITCompanyMarkdownDocuments"},
		},
		"runstate": {
			Mode:  "runtime-backed",
			Tests: []string{"TestAgentIndexesUploadedPDFDataset", "TestAgentIndexesITCompanyMarkdownDocuments"},
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
			Tests: []string{"TestSupervisorSessionEventReachesAgent"},
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

func loadServiceManifest(t *testing.T, manifestPath string) *plugin.Manifest {
	t.Helper()
	manifest, err := plugin.ParseManifest(manifestPath)
	if err != nil {
		t.Fatalf("parse service manifest %s: %v", manifestPath, err)
	}
	if manifest.Type != plugin.TypeService {
		t.Fatalf("manifest %s type = %s, want service", manifestPath, manifest.Type)
	}
	if manifest.Service == nil {
		t.Fatalf("manifest %s has nil service config", manifestPath)
	}
	if len(manifest.Service.ProtoServices) == 0 {
		t.Fatalf("service %s has no proto_services", manifest.Name)
	}
	return manifest
}

func assertServiceDocumentationCoversFunctions(t *testing.T, manifestPath string, manifest *plugin.Manifest) {
	t.Helper()
	serviceDir := filepath.Dir(manifestPath)
	readme := readCoverageFile(t, filepath.Join(serviceDir, manifest.Service.Readme))
	skill := readCoverageFile(t, filepath.Join(serviceDir, manifest.Service.Skill))
	for _, function := range manifest.Service.Functions {
		if !strings.Contains(readme, function.Name) {
			t.Fatalf("%s README does not document service function %s", manifest.Name, function.Name)
		}
		if !strings.Contains(skill, function.Name) {
			t.Fatalf("%s SKILL does not document service function %s", manifest.Name, function.Name)
		}
	}
}

func readCoverageFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func listE2ETests(t *testing.T, root string) map[string]bool {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, "e2e", "*_test.go"))
	if err != nil {
		t.Fatalf("glob e2e tests: %v", err)
	}
	fset := token.NewFileSet()
	names := make(map[string]bool)
	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !strings.HasPrefix(fn.Name.Name, "Test") {
				continue
			}
			names[fn.Name.Name] = true
		}
	}
	return names
}
