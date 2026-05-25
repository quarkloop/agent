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
	config := spacemodel.NewConfig("svc-space", workDir)
	data, err := spacemodel.MarshalConfig(config)
	if err != nil {
		t.Fatal(err)
	}

	created, err := server.CreateSpace(ctx, &spacev1.CreateSpaceRequest{
		Config: data,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.GetName() != "svc-space" {
		t.Fatalf("created name = %q", created.GetName())
	}
	if _, err := os.Stat(filepath.Join(workDir, spacemodel.ConfigFile)); !os.IsNotExist(err) {
		t.Fatalf("working directory received hidden space config, stat error = %v", err)
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

	configResp, err := server.GetConfig(ctx, &spacev1.GetConfigRequest{Name: "svc-space"})
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if _, err := spacemodel.ParseAndValidateConfig(configResp.GetConfig(), "svc-space"); err != nil {
		t.Fatalf("stored config: %v", err)
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
	config := spacemodel.NewConfig("env-space", t.TempDir())
	config.Plugins = []spacemodel.PluginRef{{Ref: "quark/service-io"}}
	config.Model = spacemodel.Model{Provider: "openrouter", Name: "openai/gpt-5-mini", Env: []string{"OPENROUTER_API_KEY"}}
	data, err := spacemodel.MarshalConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.CreateSpace(context.Background(), &spacev1.CreateSpaceRequest{
		Config: data,
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
	config := spacemodel.NewConfig("missing-env-space", t.TempDir())
	config.Plugins = []spacemodel.PluginRef{{Ref: "quark/service-io"}}
	config.Model = spacemodel.Model{Provider: "openrouter", Name: "openai/gpt-5-mini", Env: []string{"OPENROUTER_API_KEY"}}
	data, err := spacemodel.MarshalConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.CreateSpace(context.Background(), &spacev1.CreateSpaceRequest{
		Config: data,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := server.GetAgentEnvironment(context.Background(), &spacev1.GetAgentEnvironmentRequest{Name: "missing-env-space"}); err == nil {
		t.Fatal("expected missing injected environment error")
	}
}
