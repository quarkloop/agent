package coreevents

import (
	"context"
	"sync"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/quarkloop/pkg/natskit"
	corev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/core/v1"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/runtime/pkg/activity"
	runtimeservices "github.com/quarkloop/runtime/pkg/services"
)

func TestRecorderPersistsActivityEventsAndAudits(t *testing.T) {
	ns := startCoreEventsNATS(t)
	coreServer := &captureCoreServer{}
	descriptor := coreServiceDescriptor()
	host, err := natskit.StartRPCService(context.Background(), natskit.Config{
		URL: ns.ClientURL(), Name: "core-events-test",
	}, "q.core.events.test", natskit.Binding{
		Descriptor: descriptor,
		Services: []natskit.RPCService{{
			Service:        "quark.core.v1.CoreService",
			Implementation: coreServer,
		}},
	})
	if err != nil {
		t.Fatalf("start core service host: %v", err)
	}
	t.Cleanup(host.Close)

	catalog := runtimeservices.NewCatalogWithCaller([]*servicev1.ServiceDescriptor{descriptor}, runtimeservices.NewNATSCaller(runtimeservices.NATSCallerConfig{
		URL:     ns.ClientURL(),
		SpaceID: "test-space",
		Name:    "core-events-runtime-test",
	}))
	recorder := New(catalog, nil)
	if recorder == nil {
		t.Fatal("expected recorder")
	}
	defer recorder.Close()

	recorder.Record(activity.Record{
		ID:        "activity-1",
		SessionID: "session-1",
		Type:      "tool_result",
		Timestamp: time.Now().UTC(),
		Data:      []byte(`{"name":"indexer_QueryContext"}`),
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

func startCoreEventsNATS(t *testing.T) *natsserver.Server {
	t.Helper()
	ns, err := natsserver.NewServer(&natsserver.Options{Host: "127.0.0.1", Port: -1, NoLog: true, NoSigs: true})
	if err != nil {
		t.Fatalf("new nats server: %v", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(time.Second) {
		ns.Shutdown()
		t.Fatal("nats server did not become ready")
	}
	t.Cleanup(ns.Shutdown)
	return ns
}

func coreServiceDescriptor() *servicev1.ServiceDescriptor {
	return &servicev1.ServiceDescriptor{
		Name:    "core",
		Type:    "core",
		Version: "1.0.0",
		Address: "svc.core.v1",
		Rpcs: []*servicev1.RpcDescriptor{
			{
				Service:       "quark.core.v1.CoreService",
				Method:        "PublishEvent",
				Request:       "quark.core.v1.PublishEventRequest",
				Response:      "quark.core.v1.PublishEventResponse",
				Owner:         "core",
				FunctionName:  "core_PublishEvent",
				Subject:       "svc.core.v1.publish_event",
				TimeoutMillis: 2000,
			},
			{
				Service:       "quark.core.v1.CoreService",
				Method:        "RecordAuditEvent",
				Request:       "quark.core.v1.RecordAuditEventRequest",
				Response:      "quark.core.v1.RecordAuditEventResponse",
				Owner:         "core",
				FunctionName:  "core_RecordAuditEvent",
				Subject:       "svc.core.v1.record_audit_event",
				TimeoutMillis: 2000,
			},
		},
	}
}

type captureCoreServer struct {
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
