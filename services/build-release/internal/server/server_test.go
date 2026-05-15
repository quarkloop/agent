package server

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	buildreleasev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/buildrelease/v1"
	"github.com/quarkloop/services/build-release/pkg/buildrelease"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestBuildReleaseServiceDryRunAndInit(t *testing.T) {
	t.Parallel()

	listener, stop := startTestServer(t)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial service: %v", err)
	}
	defer conn.Close()
	client := buildreleasev1.NewBuildReleaseServiceClient(conn)

	wd := t.TempDir()
	initResp, err := client.Init(ctx, &buildreleasev1.InitRequest{WorkingDir: wd})
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
	dryResp, err := client.DryRun(ctx, &buildreleasev1.DryRunRequest{WorkingDir: wd, Version: "v9.9.9"})
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
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("init error = %v, code %s", err, status.Code(err))
	}
	_, err = server.DryRun(context.Background(), &buildreleasev1.DryRunRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("dry run error = %v, code %s", err, status.Code(err))
	}
	_, err = server.Release(context.Background(), &buildreleasev1.ReleaseRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("release error = %v, code %s", err, status.Code(err))
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
	if status.Code(err) != codes.Canceled {
		t.Fatalf("cancel error = %v, code %s", err, status.Code(err))
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

func startTestServer(t *testing.T) (*bufconn.Listener, func()) {
	t.Helper()
	rpcServer, err := New(buildrelease.NewRunner())
	if err != nil {
		t.Fatal(err)
	}
	grpcServer := grpc.NewServer()
	buildreleasev1.RegisterBuildReleaseServiceServer(grpcServer, rpcServer)
	ln := bufconn.Listen(1024 * 1024)
	go func() { _ = grpcServer.Serve(ln) }()
	return ln, grpcServer.Stop
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
