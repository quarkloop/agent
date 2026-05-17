// Package coreevents persists runtime activity into the Core service when the
// supervisor resolved a Core service function catalog for this runtime.
package coreevents

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	corev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/core/v1"
	servicev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/service/v1"
	"github.com/quarkloop/pkg/serviceapi/servicekit"
	"github.com/quarkloop/runtime/pkg/activity"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	coreServiceName = "core"
	runtimeActor    = "runtime"
)

type Recorder struct {
	address string
	logger  *slog.Logger

	ch     chan activity.Record
	done   chan struct{}
	closed sync.Once
	wg     sync.WaitGroup
}

func New(descriptors []*servicev1.ServiceDescriptor, logger *slog.Logger) *Recorder {
	address := coreAddress(descriptors)
	if address == "" {
		return nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	recorder := &Recorder{
		address: address,
		logger:  logger,
		ch:      make(chan activity.Record, 256),
		done:    make(chan struct{}),
	}
	recorder.wg.Add(1)
	go recorder.run()
	return recorder
}

func (r *Recorder) Record(record activity.Record) {
	if r == nil || record.Type == "" {
		return
	}
	select {
	case r.ch <- cloneRecord(record):
	default:
		r.logger.Warn("core activity recorder queue full", "activity_id", record.ID, "type", record.Type)
	}
}

func (r *Recorder) Close() {
	if r == nil {
		return
	}
	r.closed.Do(func() {
		close(r.done)
		r.wg.Wait()
	})
}

func (r *Recorder) run() {
	defer r.wg.Done()
	for {
		select {
		case <-r.done:
			return
		case record := <-r.ch:
			r.persist(record)
		}
	}
}

func (r *Recorder) persist(record activity.Record) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := servicekit.Dial(ctx, r.address)
	if err != nil {
		r.logger.Warn("dial core service for activity persistence failed", "error", err)
		return
	}
	defer conn.Close()

	client := corev1.NewCoreServiceClient(conn)
	payload := string(record.Data)
	stream := "runtime"
	runID := "runtime"
	if record.SessionID != "" {
		stream = "session/" + record.SessionID
		runID = record.SessionID
	}
	createdAt := timestamppb.New(record.Timestamp)
	if record.Timestamp.IsZero() {
		createdAt = timestamppb.Now()
	}
	if _, err := client.PublishEvent(ctx, &corev1.PublishEventRequest{Event: &corev1.Event{
		Id:          record.ID,
		Stream:      stream,
		Kind:        record.Type,
		PayloadJson: payload,
		CreatedAt:   createdAt,
	}}); err != nil {
		r.logger.Warn("publish core activity event failed", "error", err, "activity_id", record.ID)
		return
	}
	if _, err := client.RecordAuditEvent(ctx, &corev1.RecordAuditEventRequest{Event: &corev1.AuditEvent{
		Id:        record.ID + "-audit",
		RunId:     runID,
		Actor:     runtimeActor,
		Action:    record.Type,
		Target:    record.ID,
		CreatedAt: createdAt,
	}}); err != nil {
		r.logger.Warn("record core activity audit failed", "error", err, "activity_id", record.ID)
	}
}

func coreAddress(descriptors []*servicev1.ServiceDescriptor) string {
	for _, desc := range descriptors {
		if desc == nil || strings.TrimSpace(desc.GetAddress()) == "" {
			continue
		}
		if desc.GetName() == coreServiceName || desc.GetType() == coreServiceName {
			return desc.GetAddress()
		}
		for _, rpc := range desc.GetRpcs() {
			if rpc.GetService() == corev1.CoreService_ServiceDesc.ServiceName {
				return desc.GetAddress()
			}
		}
	}
	return ""
}

func cloneRecord(record activity.Record) activity.Record {
	record.Data = append(record.Data[:0:0], record.Data...)
	return record
}
