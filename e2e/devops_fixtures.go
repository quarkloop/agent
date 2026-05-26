//go:build e2e

package e2e

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func writeDevOpsReleaseFixture(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/quarkdevopsfixture\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := map[string]any{
		"package_name": "quark-devops-fixture",
		"binary_name":  "quark-devops-fixture",
		"release_dir":  "dist",
		"builds": []map[string]any{{
			"name":        "quark-devops-fixture",
			"source_path": ".",
			"binary_name": "quark-devops-fixture",
			"source_dir":  ".",
		}},
		"targets": []map[string]any{{
			"os":   "linux",
			"arch": "amd64",
		}},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "build_release.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeFailingGoTestFixture(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/quarkdevopsfailure\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc Stable() bool { return false }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main_test.go"), []byte(`package main

import "testing"

func TestBroken(t *testing.T) {
	if !Stable() {
		t.Fatalf("expected stable behavior")
	}
}
`), 0o644); err != nil {
		t.Fatal(err)
	}
}

func initGitRepository(t *testing.T, dir string) {
	t.Helper()
	runGitFixtureCommand(t, dir, "init")
	runGitFixtureCommand(t, dir, "config", "user.email", "e2e@example.invalid")
	runGitFixtureCommand(t, dir, "config", "user.name", "Quark E2E")
	runGitFixtureCommand(t, dir, "add", ".")
	runGitFixtureCommand(t, dir, "commit", "-m", "initial fixture")
}

func runGitFixtureCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
}
