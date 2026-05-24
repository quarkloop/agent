package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/quarkloop/pkg/boundary"
	buildreleasev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/buildrelease/v1"
	"github.com/quarkloop/services/build-release/pkg/buildrelease"
)

func TestBuildReleaseServiceDryRunAndInit(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	server, err := New(buildrelease.NewRunner())
	if err != nil {
		t.Fatal(err)
	}

	wd := t.TempDir()
	initResp, err := server.Init(ctx, &buildreleasev1.InitRequest{WorkingDir: wd})
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if !initResp.GetCreated() {
		t.Fatal("expected init to create config")
	}
	if _, err := os.Stat(initResp.GetConfigPath()); err != nil {
		t.Fatalf("config missing: %v", err)
	}

	writeConfig(t, wd)
	dryResp, err := server.DryRun(ctx, &buildreleasev1.DryRunRequest{WorkingDir: wd, Version: "v9.9.9"})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if dryResp.GetVersion() != "v9.9.9" {
		t.Fatalf("version = %q", dryResp.GetVersion())
	}
	if got := len(dryResp.GetPlanned()); got != 1 {
		t.Fatalf("planned = %d, want 1", got)
	}
}

func TestBuildReleaseServiceRequiresWorkingDir(t *testing.T) {
	t.Parallel()

	server, err := New(buildrelease.NewRunner())
	if err != nil {
		t.Fatal(err)
	}

	_, err = server.Init(context.Background(), &buildreleasev1.InitRequest{})
	if !boundary.IsCategory(err, boundary.InvalidArgument) {
		t.Fatalf("init error = %v, want invalid argument", err)
	}
	_, err = server.DryRun(context.Background(), &buildreleasev1.DryRunRequest{})
	if !boundary.IsCategory(err, boundary.InvalidArgument) {
		t.Fatalf("dry run error = %v, want invalid argument", err)
	}
	_, err = server.Release(context.Background(), &buildreleasev1.ReleaseRequest{})
	if !boundary.IsCategory(err, boundary.InvalidArgument) {
		t.Fatalf("release error = %v, want invalid argument", err)
	}
}

func TestBuildReleaseServiceMapsCancellation(t *testing.T) {
	t.Parallel()

	server, err := New(buildrelease.NewRunner())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = server.Init(ctx, &buildreleasev1.InitRequest{WorkingDir: t.TempDir()})
	if !boundary.IsCategory(err, boundary.Canceled) {
		t.Fatalf("cancel error = %v, want canceled", err)
	}
}

func TestArtifactsToProtoMapsReleaseArtifacts(t *testing.T) {
	t.Parallel()

	got := artifactsToProto([]buildrelease.Artifact{{
		BuildName:   "quark",
		Target:      buildrelease.Target{OS: "linux", Arch: "amd64"},
		Filename:    "quark",
		ArchiveName: "quark_linux_amd64.tar.gz",
		Checksum:    "abc123",
		Size:        12,
		Duration:    1500 * time.Millisecond,
		Error:       "compile failed",
	}})

	if len(got) != 1 {
		t.Fatalf("artifacts = %d, want 1", len(got))
	}
	artifact := got[0]
	if artifact.GetBuildName() != "quark" || artifact.GetOs() != "linux" || artifact.GetArch() != "amd64" || artifact.GetDurationMillis() != 1500 || artifact.GetError() != "compile failed" {
		t.Fatalf("artifact = %+v", artifact)
	}
}

func writeConfig(t *testing.T, wd string) {
	t.Helper()
	cfg := buildrelease.ReleaseConfig{
		PackageName: "svc-test",
		ReleaseDir:  "dist",
		Builds: []buildrelease.BuildTarget{{
			Name:       "svc-test",
			SourcePath: ".",
			BinaryName: "svc-test",
			SourceDir:  ".",
		}},
		Targets: []buildrelease.Target{{OS: "linux", Arch: "amd64"}},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wd, "build_release.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
