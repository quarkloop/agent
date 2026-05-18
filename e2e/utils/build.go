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
	Supervisor   string
	Agent        string
	Bash         string
	FS           string
	Indexer      string
	Embedding    string
	Citation     string
	DevOps       string
	Document     string
	Ingestion    string
	System       string
	BuildRelease string

	// Lib-mode tool .so paths. Empty if the build failed (e.g. no CGO);
	// callers should fall back to api-mode installation.
	BashLib string
	FSLib   string

	OpenRouterLib string
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

		// buildLib builds a tool as a Go plugin .so. Failures are tolerated
		// and reported as empty paths; the caller will install the tool in
		// api mode instead.
		buildLib := func(pkg, name string) string {
			out := filepath.Join(binDir, name+".so")
			cmd := exec.Command("go", "build", "-buildmode=plugin", "-tags", "plugin", "-o", out, pkg)
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "CGO_ENABLED=1")
			if output, err := cmd.CombinedOutput(); err != nil {
				Logf(t, "build %s lib mode failed (will fall back to binary): %v\n%s", pkg, err, string(output))
				return ""
			}
			return out
		}

		buildRes.Supervisor = build("./supervisor/cmd/supervisor", "supervisor")
		buildRes.Agent = build("./runtime/cmd/runtime", "runtime")
		buildRes.Bash = build("./plugins/tools/bash/cmd/bash", "bash")
		buildRes.FS = build("./plugins/tools/fs/cmd/fs", "fs")
		buildRes.Indexer = build("./services/indexer/cmd/indexer", "indexer")
		buildRes.Embedding = build("./services/embedding/cmd/embedding", "embedding")
		buildRes.Citation = build("./services/citation/cmd/citation", "citation")
		buildRes.DevOps = build("./services/devops/cmd/devops", "devops")
		buildRes.Document = build("./services/document/cmd/document", "document")
		buildRes.Ingestion = build("./services/ingestion/cmd/ingestion", "ingestion")
		buildRes.System = build("./services/system/cmd/system", "system")
		buildRes.BuildRelease = build("./services/build-release/cmd/build-release", "build-release")

		buildRes.BashLib = buildLib("./plugins/tools/bash", "bash")
		buildRes.FSLib = buildLib("./plugins/tools/fs", "fs")
		buildRes.OpenRouterLib = buildLib("./plugins/providers/openrouter", "openrouter")
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return buildRes
}
