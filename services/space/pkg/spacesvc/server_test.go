package spacesvc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	spacev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/space/v1"
	spacemodel "github.com/quarkloop/pkg/space"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestStoreRequiresInjectedStorageRoot(t *testing.T) {
	t.Parallel()

	if _, err := NewStore(""); err == nil || !strings.Contains(err.Error(), "root is required") {
		t.Fatalf("NewStore empty root error = %v", err)
	}
}

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

	configResp, err := server.GetConfig(ctx, &spacev1.GetConfigRequest{Name: "svc-space"})
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if _, err := spacemodel.ParseAndValidateConfig(configResp.GetConfig(), "svc-space"); err != nil {
		t.Fatalf("stored config: %v", err)
	}
}

func TestOpaqueRecordsAreStoredAndScopedByNamespace(t *testing.T) {
	t.Parallel()

	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(store)
	if err != nil {
		t.Fatal(err)
	}
	config := spacemodel.NewConfig("record-space", t.TempDir())
	config = config.WithPluginSelection(spacemodel.PluginRef{Ref: "quark/service-io"}, nil)
	data, err := spacemodel.MarshalConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.CreateSpace(context.Background(), &spacev1.CreateSpaceRequest{
		Config: data,
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	put, err := server.PutRecord(context.Background(), &spacev1.PutRecordRequest{
		Name: "record-space", Namespace: "sessions", Key: "session-1", Data: []byte(`{"state":"active"}`),
	})
	if err != nil {
		t.Fatalf("put record: %v", err)
	}
	if put.GetUpdatedAt() == nil {
		t.Fatal("record update timestamp missing")
	}
	got, err := server.GetRecord(context.Background(), &spacev1.GetRecordRequest{Name: "record-space", Namespace: "sessions", Key: "session-1"})
	if err != nil || string(got.GetData()) != `{"state":"active"}` {
		t.Fatalf("get record = %q, err=%v", got.GetData(), err)
	}
	listed, err := server.ListRecords(context.Background(), &spacev1.ListRecordsRequest{Name: "record-space", Namespace: "sessions"})
	if err != nil || len(listed.GetRecords()) != 1 {
		t.Fatalf("list records = %+v, err=%v", listed, err)
	}
	if _, err := server.DeleteRecord(context.Background(), &spacev1.DeleteRecordRequest{Name: "record-space", Namespace: "sessions", Key: "session-1"}); err != nil {
		t.Fatalf("delete record: %v", err)
	}
	if _, err := server.GetRecord(context.Background(), &spacev1.GetRecordRequest{Name: "record-space", Namespace: "sessions", Key: "session-1"}); err == nil {
		t.Fatal("deleted record remained readable")
	}
}
