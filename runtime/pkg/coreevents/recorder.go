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
	"github.com/quarkloop/runtime/pkg/activity"
	runtimeservices "github.com/quarkloop/runtime/pkg/services"
	"google.golang.org/protobuf/encoding/protojson"
)

const runtimeActor = "runtime"

type Recorder struct {
	catalog *runtimeservices.Catalog
	logger  *slog.Logger

	ch     chan activity.Record
	done   chan struct{}
	closed sync.Once
	wg     sync.WaitGroup
}

func New(catalog *runtimeservices.Catalog, logger *slog.Logger) *Recorder {
	if catalog == nil || catalog.Empty() || !hasCoreFunctions(catalog.Descriptors()) {
		return nil
	}
	if logger == nil {
		logger = slog.Default()
	}
	recorder := &Recorder{
		catalog: catalog,
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
	payload := string(record.Data)
	stream := "runtime"
	runID := "runtime"
	if record.SessionID != "" {
		stream = "session/" + record.SessionID
		runID = record.SessionID
	}
	eventArgs, err := protojson.MarshalOptions{UseProtoNames: false}.Marshal(&corev1.PublishEventRequest{Event: &corev1.Event{
		Id:          record.ID,
		Stream:      stream,
		Kind:        record.Type,
		PayloadJson: payload,
	}})
	if err != nil {
		r.logger.Warn("encode core activity event failed", "error", err, "activity_id", record.ID)
		return
	}
	if _, err := r.catalog.Execute(ctx, "core_PublishEvent", string(eventArgs)); err != nil {
		r.logger.Warn("publish core activity event failed", "error", err, "activity_id", record.ID)
		return
	}
	auditArgs, err := protojson.MarshalOptions{UseProtoNames: false}.Marshal(&corev1.RecordAuditEventRequest{Event: &corev1.AuditEvent{
		Id:     record.ID + "-audit",
		RunId:  runID,
		Actor:  runtimeActor,
		Action: record.Type,
		Target: record.ID,
	}})
	if err != nil {
		r.logger.Warn("encode core activity audit failed", "error", err, "activity_id", record.ID)
		return
	}
	if _, err := r.catalog.Execute(ctx, "core_RecordAuditEvent", string(auditArgs)); err != nil {
		r.logger.Warn("record core activity audit failed", "error", err, "activity_id", record.ID)
	}
}

func hasCoreFunctions(descriptors []*servicev1.ServiceDescriptor) bool {
	foundEvent := false
	foundAudit := false
	for _, desc := range descriptors {
		if desc == nil {
			continue
		}
		for _, rpc := range desc.GetRpcs() {
			if rpc.GetService() != corev1.CoreService_ServiceDesc.ServiceName {
				continue
			}
			switch strings.TrimSpace(rpc.GetFunctionName()) {
			case "core_PublishEvent":
				foundEvent = true
			case "core_RecordAuditEvent":
				foundAudit = true
			default:
				switch strings.TrimSpace(rpc.GetMethod()) {
				case "PublishEvent":
					foundEvent = true
				case "RecordAuditEvent":
					foundAudit = true
				}
			}
		}
	}
	return foundEvent && foundAudit
}

func cloneRecord(record activity.Record) activity.Record {
	record.Data = append(record.Data[:0:0], record.Data...)
	return record
}
