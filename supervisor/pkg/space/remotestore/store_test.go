package remotestore

import (
	"bytes"
	"context"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/quarkloop/pkg/natskit"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	spacev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/space/v1"
	spacemodel "github.com/quarkloop/pkg/space"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestStoreDelegatesConfigOperationsThroughCanonicalNATSSubjects(t *testing.T) {
	broker := startTestNATS(t)
	responder := &recordingSpaceResponder{}
	host, err := natskit.StartRPCService(context.Background(), natskit.Config{
		URL: broker.ClientURL(), Name: "space-persistence-responder",
	}, natskit.Binding{
		Descriptor: configDescriptor(),
		Services: []natskit.RPCService{{
			Service:        "quark.space.v1.SpaceService",
			Implementation: responder,
		}},
	})
	if err != nil {
		t.Fatalf("start responder: %v", err)
	}
	t.Cleanup(host.Close)

	store, err := New(context.Background(), natskit.Config{
		URL: broker.ClientURL(), Name: "space-persistence-caller",
	})
	if err != nil {
		t.Fatalf("new remote store: %v", err)
	}
	t.Cleanup(store.Close)

	cfg := spacemodel.NewConfig("research", "/work/research")
	createdData, err := spacemodel.MarshalConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Create(createdData)
	if err != nil {
		t.Fatalf("create through service: %v", err)
	}
	if created.Name != cfg.Name || !bytes.Equal(responder.created, createdData) {
		t.Fatalf("create result/payload = %+v / %q", created, responder.created)
	}

	cfg.Description = "updated"
	updatedData, err := spacemodel.MarshalConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpdateConfig(updatedData); err != nil {
		t.Fatalf("update through service: %v", err)
	}
	got, err := store.Config(cfg.Name)
	if err != nil {
		t.Fatalf("get config through service: %v", err)
	}
	if !bytes.Equal(responder.updated, updatedData) || !bytes.Equal(got, updatedData) {
		t.Fatalf("updated/config payloads = %q / %q", responder.updated, got)
	}
	if err := store.PutRecord(cfg.Name, "sessions", "session-1", []byte("opaque")); err != nil {
		t.Fatalf("put record through service: %v", err)
	}
	record, err := store.GetRecord(cfg.Name, "sessions", "session-1")
	if err != nil || string(record) != "opaque" {
		t.Fatalf("get record = %q, err = %v", record, err)
	}
	records, err := store.ListRecords(cfg.Name, "sessions")
	if err != nil || len(records) != 1 || string(records[0]) != "opaque" {
		t.Fatalf("list records = %q, err = %v", records, err)
	}
	if err := store.DeleteRecord(cfg.Name, "sessions", "session-1"); err != nil {
		t.Fatalf("delete record through service: %v", err)
	}
}

type recordingSpaceResponder struct {
	created []byte
	updated []byte
	record  []byte
}

func (s *recordingSpaceResponder) CreateSpace(_ context.Context, req *spacev1.CreateSpaceRequest) (*spacev1.Space, error) {
	s.created = append([]byte(nil), req.GetConfig()...)
	return responseSpace(req.GetConfig()), nil
}

func (s *recordingSpaceResponder) UpdateConfig(_ context.Context, req *spacev1.UpdateConfigRequest) (*spacev1.Space, error) {
	s.updated = append([]byte(nil), req.GetConfig()...)
	return responseSpace(req.GetConfig()), nil
}

func (s *recordingSpaceResponder) GetConfig(_ context.Context, req *spacev1.GetConfigRequest) (*spacev1.ConfigResponse, error) {
	return &spacev1.ConfigResponse{Name: req.GetName(), Version: "0.1.0", Config: append([]byte(nil), s.updated...), UpdatedAt: timestamppb.Now()}, nil
}

func (s *recordingSpaceResponder) PutRecord(_ context.Context, req *spacev1.PutRecordRequest) (*spacev1.Record, error) {
	s.record = append([]byte(nil), req.GetData()...)
	return &spacev1.Record{Namespace: req.GetNamespace(), Key: req.GetKey(), Data: append([]byte(nil), s.record...), UpdatedAt: timestamppb.Now()}, nil
}

func (s *recordingSpaceResponder) GetRecord(_ context.Context, req *spacev1.GetRecordRequest) (*spacev1.Record, error) {
	return &spacev1.Record{Namespace: req.GetNamespace(), Key: req.GetKey(), Data: append([]byte(nil), s.record...), UpdatedAt: timestamppb.Now()}, nil
}

func (s *recordingSpaceResponder) ListRecords(_ context.Context, req *spacev1.ListRecordsRequest) (*spacev1.ListRecordsResponse, error) {
	return &spacev1.ListRecordsResponse{Records: []*spacev1.Record{{
		Namespace: req.GetNamespace(), Key: "session-1", Data: append([]byte(nil), s.record...), UpdatedAt: timestamppb.Now(),
	}}}, nil
}

func (s *recordingSpaceResponder) DeleteRecord(_ context.Context, _ *spacev1.DeleteRecordRequest) (*emptypb.Empty, error) {
	s.record = nil
	return &emptypb.Empty{}, nil
}

func responseSpace(data []byte) *spacev1.Space {
	cfg, _ := spacemodel.ParseConfig(data)
	return &spacev1.Space{
		Name:       cfg.Name,
		Version:    cfg.Version,
		WorkingDir: cfg.WorkingDir,
		CreatedAt:  timestamppb.New(cfg.CreatedAt),
		UpdatedAt:  timestamppb.New(cfg.UpdatedAt),
	}
}

func configDescriptor() *servicev1.ServiceDescriptor {
	return &servicev1.ServiceDescriptor{
		Name: "space",
		Rpcs: []*servicev1.RpcDescriptor{
			natskit.MustServiceRPC("space", "space_CreateSpace", "quark.space.v1.SpaceService", "CreateSpace", "quark.space.v1.CreateSpaceRequest", "quark.space.v1.Space", "Create space."),
			natskit.MustServiceRPC("space", "space_UpdateConfig", "quark.space.v1.SpaceService", "UpdateConfig", "quark.space.v1.UpdateConfigRequest", "quark.space.v1.Space", "Update config."),
			natskit.MustServiceRPC("space", "space_GetConfig", "quark.space.v1.SpaceService", "GetConfig", "quark.space.v1.GetConfigRequest", "quark.space.v1.ConfigResponse", "Get config."),
			natskit.MustServiceRPC("space", "space_PutRecord", "quark.space.v1.SpaceService", "PutRecord", "quark.space.v1.PutRecordRequest", "quark.space.v1.Record", "Put record."),
			natskit.MustServiceRPC("space", "space_GetRecord", "quark.space.v1.SpaceService", "GetRecord", "quark.space.v1.GetRecordRequest", "quark.space.v1.Record", "Get record."),
			natskit.MustServiceRPC("space", "space_ListRecords", "quark.space.v1.SpaceService", "ListRecords", "quark.space.v1.ListRecordsRequest", "quark.space.v1.ListRecordsResponse", "List records."),
			natskit.MustServiceRPC("space", "space_DeleteRecord", "quark.space.v1.SpaceService", "DeleteRecord", "quark.space.v1.DeleteRecordRequest", "google.protobuf.Empty", "Delete record."),
		},
	}
}

func startTestNATS(t *testing.T) *natsserver.Server {
	t.Helper()
	server, err := natsserver.NewServer(&natsserver.Options{
		Host:   "127.0.0.1",
		Port:   -1,
		NoLog:  true,
		NoSigs: true,
	})
	if err != nil {
		t.Fatalf("new nats server: %v", err)
	}
	go server.Start()
	if !server.ReadyForConnections(5 * time.Second) {
		t.Fatal("nats server did not become ready")
	}
	t.Cleanup(server.Shutdown)
	return server
}
