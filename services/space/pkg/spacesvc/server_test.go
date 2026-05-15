package spacesvc

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	spacev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/space/v1"
	spacemodel "github.com/quarkloop/pkg/space"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestSpaceServiceLifecycle(t *testing.T) {
	t.Parallel()

	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(store)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	workDir := t.TempDir()
	qf := spacemodel.DefaultQuarkfile("svc-space")

	created, err := server.CreateSpace(ctx, &spacev1.CreateSpaceRequest{
		Name:       "svc-space",
		Quarkfile:  qf,
		WorkingDir: workDir,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.GetName() != "svc-space" {
		t.Fatalf("created name = %q", created.GetName())
	}
	if _, err := os.Stat(filepath.Join(workDir, spacemodel.QuarkfileName)); err != nil {
		t.Fatalf("working Quarkfile missing: %v", err)
	}

	listed, err := server.ListSpaces(ctx, &emptypb.Empty{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if got := len(listed.GetSpaces()); got != 1 {
		t.Fatalf("spaces = %d, want 1", got)
	}

	paths, err := server.GetSpacePaths(ctx, &spacev1.GetSpacePathsRequest{Name: "svc-space"})
	if err != nil {
		t.Fatalf("paths: %v", err)
	}
	if paths.GetPluginsDir() == "" || paths.GetKbDir() == "" || paths.GetSessionsDir() == "" {
		t.Fatalf("incomplete paths: %+v", paths)
	}

	qfResp, err := server.GetQuarkfile(ctx, &spacev1.GetQuarkfileRequest{Name: "svc-space"})
	if err != nil {
		t.Fatalf("quarkfile: %v", err)
	}
	if string(qfResp.GetQuarkfile()) != string(qf) {
		t.Fatal("stored Quarkfile mismatch")
	}
}

func TestAgentEnvironmentUsesInjectedEnvironmentSnapshot(t *testing.T) {
	t.Parallel()

	store, err := NewStoreWithEnvironment(t.TempDir(), []string{"OPENROUTER_API_KEY=from-snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(store)
	if err != nil {
		t.Fatal(err)
	}
	quarkfile := []byte(`quark: "1.0"
meta:
  name: env-space
  version: "0.1.0"
plugins:
  - ref: quark/tool-bash
model:
  provider: openrouter
  name: openai/gpt-5-mini
  env:
    - OPENROUTER_API_KEY
`)
	if _, err := server.CreateSpace(context.Background(), &spacev1.CreateSpaceRequest{
		Name:       "env-space",
		Quarkfile:  quarkfile,
		WorkingDir: t.TempDir(),
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	env, err := server.GetAgentEnvironment(context.Background(), &spacev1.GetAgentEnvironmentRequest{Name: "env-space"})
	if err != nil {
		t.Fatalf("agent environment: %v", err)
	}
	got := env.GetEntries()
	want := []string{
		"QUARK_MODEL_PROVIDER=openrouter",
		"QUARK_MODEL_NAME=openai/gpt-5-mini",
		"OPENROUTER_API_KEY=from-snapshot",
	}
	if len(got) != len(want) {
		t.Fatalf("env entries = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("env[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestAgentEnvironmentReportsMissingInjectedVariable(t *testing.T) {
	t.Parallel()

	store, err := NewStoreWithEnvironment(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(store)
	if err != nil {
		t.Fatal(err)
	}
	quarkfile := []byte(`quark: "1.0"
meta:
  name: missing-env-space
  version: "0.1.0"
plugins:
  - ref: quark/tool-bash
model:
  provider: openrouter
  name: openai/gpt-5-mini
  env:
    - OPENROUTER_API_KEY
`)
	if _, err := server.CreateSpace(context.Background(), &spacev1.CreateSpaceRequest{
		Name:       "missing-env-space",
		Quarkfile:  quarkfile,
		WorkingDir: t.TempDir(),
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := server.GetAgentEnvironment(context.Background(), &spacev1.GetAgentEnvironmentRequest{Name: "missing-env-space"}); err == nil {
		t.Fatal("expected missing injected environment error")
	}
}
