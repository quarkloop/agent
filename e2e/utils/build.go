//go:build e2e

package utils

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// BuiltBinaries collects the paths of subprocesses the suite compiles once
// per `go test` invocation.
type BuiltBinaries struct {
	Supervisor string
	Agent      string
	IO         string
	Indexer    string
	Citation   string
	Core       string
	Gateway    string
	DevOps     string
	Document   string
	RunState   string
	System     string
	Workflow   string
	Secrets    string
}

var (
	buildOnce sync.Once
	buildRes  BuiltBinaries
	buildErr  error
)

// QuarkRoot returns the absolute path to the quark/ directory (the parent of
// e2e/).
func QuarkRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve caller path")
	}
	// utils/build.go → e2e/ → quark/
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// BuildAllOnce builds every subprocess this suite needs. The result is
// cached across tests within the same `go test` invocation so each run
// compiles once.
func BuildAllOnce(t *testing.T) BuiltBinaries {
	t.Helper()
	buildOnce.Do(func() {
		root := QuarkRoot(t)
		binDir := filepath.Join(os.TempDir(), fmt.Sprintf("quark-e2e-bin-%d", os.Getpid()))
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			buildErr = fmt.Errorf("create bin dir: %w", err)
			return
		}
		Logf(t, "building binaries into %s", binDir)

		build := func(pkg, name string) string {
			if buildErr != nil {
				return ""
			}
			out := filepath.Join(binDir, name)
			cmd := exec.Command("go", "build", "-o", out, pkg)
			cmd.Dir = root
			if output, err := cmd.CombinedOutput(); err != nil {
				buildErr = fmt.Errorf("build %s: %w\n%s", pkg, err, string(output))
				return ""
			}
			return out
		}

		buildRes.Supervisor = build("./supervisor/cmd/supervisor", "supervisor")
		buildRes.Agent = build("./runtime/cmd/runtime", "runtime")
		buildRes.IO = build("./services/io/cmd/io", "io")
		buildRes.Indexer = build("./services/indexer/cmd/indexer", "indexer")
		buildRes.Citation = build("./services/citation/cmd/citation", "citation")
		buildRes.Core = build("./services/core/cmd/core", "core")
		buildRes.Gateway = build("./services/gateway/cmd/gateway", "gateway")
		buildRes.DevOps = build("./services/devops/cmd/devops", "devops")
		buildRes.Document = build("./services/document/cmd/document", "document")
		buildRes.RunState = build("./services/runstate/cmd/runstate", "runstate")
		buildRes.System = build("./services/system/cmd/system", "system")
		buildRes.Workflow = build("./services/workflow/cmd/workflow", "workflow")
		buildRes.Secrets = build("./services/secrets/cmd/secrets", "secrets")
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return buildRes
}
