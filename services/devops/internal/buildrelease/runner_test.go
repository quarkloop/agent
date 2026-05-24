package buildrelease

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunnerInitCreatesDefaultConfig(t *testing.T) {
	t.Parallel()

	wd := t.TempDir()
	runner := NewRunner()
	result, err := runner.Init(context.Background(), InitRequest{WorkingDir: wd})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created {
		t.Fatal("expected config to be created")
	}
	if result.ConfigPath != filepath.Join(wd, defaultConfigPath) {
		t.Fatalf("config path = %q", result.ConfigPath)
	}
	if _, err := os.Stat(result.ConfigPath); err != nil {
		t.Fatalf("config missing: %v", err)
	}

	result, err = runner.Init(context.Background(), InitRequest{WorkingDir: wd})
	if err != nil {
		t.Fatal(err)
	}
	if result.Created {
		t.Fatal("second init should report existing config")
	}
}

func TestRunnerDryRunPlansArtifactMatrix(t *testing.T) {
	t.Parallel()

	wd := t.TempDir()
	writeConfig(t, wd, ReleaseConfig{
		PackageName: "quark-test",
		ReleaseDir:  "dist",
		Builds: []BuildTarget{
			{Name: "quark-test", SourcePath: ".", BinaryName: "quark-test", SourceDir: "."},
		},
		Targets: []Target{
			{OS: "linux", Arch: "amd64"},
			{OS: "darwin", Arch: "arm64"},
		},
	})

	runner := NewRunner()
	runner.Clock = func() time.Time { return time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC) }
	result, err := runner.DryRun(context.Background(), DryRunRequest{
		WorkingDir: wd,
		Version:    "v1.2.3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Version != "v1.2.3" {
		t.Fatalf("version = %q", result.Version)
	}
	if got := len(result.Planned); got != 2 {
		t.Fatalf("planned = %d, want 2", got)
	}
	if result.Planned[0].ArchiveName == "" || !strings.HasSuffix(result.Planned[0].ArchiveName, ".tar.gz") {
		t.Fatalf("unexpected artifact: %+v", result.Planned[0])
	}
}

func TestRunnerReleaseBuildsArchiveAndChecksums(t *testing.T) {
	t.Parallel()

	wd := t.TempDir()
	if err := os.WriteFile(filepath.Join(wd, "go.mod"), []byte("module example.com/quarkbuild\n\ngo 1.26\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wd, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, wd, ReleaseConfig{
		PackageName: "quarkbuild",
		BinaryName:  "quarkbuild",
		ReleaseDir:  "dist",
		Homepage:    "https://github.com/quarkloop/quarkbuild",
		License:     "Apache-2.0",
		Checksums:   true,
		Builds: []BuildTarget{
			{Name: "quarkbuild", SourcePath: ".", BinaryName: "quarkbuild", SourceDir: "."},
		},
		Targets: []Target{
			{OS: runtime.GOOS, Arch: runtime.GOARCH},
		},
	})

	runner := NewRunner()
	result, err := runner.Release(context.Background(), ReleaseRequest{
		WorkingDir: wd,
		Version:    "v0.0.1",
		SkipTests:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success {
		t.Fatal("release did not report success")
	}
	if got := len(result.Artifacts); got != 1 {
		t.Fatalf("artifacts = %d, want 1", got)
	}
	if result.Artifacts[0].Checksum == "" {
		t.Fatal("checksum was not populated")
	}
	for _, name := range []string{"checksums.txt", "README.md", "release.json", "install.sh"} {
		if _, err := os.Stat(filepath.Join(result.ReleaseDir, name)); err != nil {
			t.Fatalf("%s missing: %v", name, err)
		}
	}
}

func writeConfig(t *testing.T, wd string, cfg ReleaseConfig) {
	t.Helper()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wd, defaultConfigPath), append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
