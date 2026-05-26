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

	"github.com/quarkloop/pkg/plugin"
)

type serviceCoverageExpectation struct {
	Mode  string
	Tests []string
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
