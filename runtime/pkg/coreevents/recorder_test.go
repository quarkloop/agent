package coreevents

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	corev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/core/v1"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/runtime/pkg/activity"
	"google.golang.org/grpc"
)

func TestRecorderPersistsActivityEventsAndAudits(t *testing.T) {
	coreServer := &captureCoreServer{}
	grpcServer := grpc.NewServer()
	corev1.RegisterCoreServiceServer(grpcServer, coreServer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = grpcServer.Serve(ln) }()
	defer grpcServer.Stop()

	recorder := New([]*servicev1.ServiceDescriptor{{
		Name:    "core",
		Type:    "core",
		Address: ln.Addr().String(),
		Rpcs: []*servicev1.RpcDescriptor{{
			Service: corev1.CoreService_ServiceDesc.ServiceName,
			Method:  "PublishEvent",
		}},
	}}, nil)
	if recorder == nil {
		t.Fatal("expected recorder")
	}
	defer recorder.Close()

	recorder.Record(activity.Record{
		ID:        "activity-1",
		SessionID: "session-1",
		Type:      "tool_result",
		Timestamp: time.Now().UTC(),
		Data:      []byte(`{"name":"indexer_GetContext"}`),
	})

	deadline := time.After(2 * time.Second)
	for {
		if coreServer.counts() == (coreCounts{events: 1, audits: 1}) {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("core records = %+v", coreServer.counts())
		case <-time.After(10 * time.Millisecond):
		}
	}
	event, audit := coreServer.first()
	if event.GetStream() != "session/session-1" || event.GetKind() != "tool_result" {
		t.Fatalf("unexpected core event: %#v", event)
	}
	if audit.GetRunId() != "session-1" || audit.GetAction() != "tool_result" {
		t.Fatalf("unexpected core audit: %#v", audit)
	}
}

type captureCoreServer struct {
	corev1.UnimplementedCoreServiceServer
	mu     sync.Mutex
	events []*corev1.Event
	audits []*corev1.AuditEvent
}

type coreCounts struct {
	events int
	audits int
}

func (s *captureCoreServer) PublishEvent(_ context.Context, req *corev1.PublishEventRequest) (*corev1.PublishEventResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, req.GetEvent())
	return &corev1.PublishEventResponse{Event: req.GetEvent()}, nil
}

func (s *captureCoreServer) RecordAuditEvent(_ context.Context, req *corev1.RecordAuditEventRequest) (*corev1.RecordAuditEventResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.audits = append(s.audits, req.GetEvent())
	return &corev1.RecordAuditEventResponse{Event: req.GetEvent()}, nil
}

func (s *captureCoreServer) counts() coreCounts {
	s.mu.Lock()
	defer s.mu.Unlock()
	return coreCounts{events: len(s.events), audits: len(s.audits)}
}

func (s *captureCoreServer) first() (*corev1.Event, *corev1.AuditEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.events[0], s.audits[0]
}
