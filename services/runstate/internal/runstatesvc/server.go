package runstatesvc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	runstatev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/runstate/v1"
	"github.com/quarkloop/pkg/serviceapi/serviceerrors"
)

const defaultRetentionDays = 30

type Server struct {
	records *recordStore
	leases  LeaseStore
	now     func() time.Time
}

func New(root string, stores ...LeaseStore) (*Server, error) {
	records, err := newRecordStore(root)
	if err != nil {
		return nil, err
	}
	leases := LeaseStore(newMemoryLeaseStore())
	if len(stores) > 0 && stores[0] != nil {
		leases = stores[0]
	}
	return &Server{records: records, leases: leases, now: time.Now}, nil
}

func (s *Server) StartRun(_ context.Context, req *runstatev1.StartRunRequest) (*runstatev1.StartRunResponse, error) {
	run, err := s.newRun(req)
	if err != nil {
		return nil, serviceError(err)
	}
	stored, err := s.records.createRun(run)
	if err != nil {
		return nil, serviceError(err)
	}
	return &runstatev1.StartRunResponse{Run: runToProto(stored)}, nil
}

func (s *Server) GetRun(_ context.Context, req *runstatev1.GetRunRequest) (*runstatev1.GetRunResponse, error) {
	if strings.TrimSpace(req.GetRunId()) == "" {
		return nil, serviceError(fmt.Errorf("%w: run_id is required", errInvalidArgument))
	}
	run, err := s.records.getRun(req.GetRunId())
	if err != nil {
		return nil, serviceError(err)
	}
	return &runstatev1.GetRunResponse{Run: runToProto(run)}, nil
}

func (s *Server) ListRuns(_ context.Context, req *runstatev1.ListRunsRequest) (*runstatev1.ListRunsResponse, error) {
	filter, err := statusFromProto(req.GetStatus(), "")
	if err != nil {
		return nil, serviceError(err)
	}
	runs, err := s.records.listRuns(strings.TrimSpace(req.GetSpace()), filter, strings.TrimSpace(req.GetKind()), int(req.GetLimit()))
	if err != nil {
		return nil, serviceError(err)
	}
	out := make([]*runstatev1.Run, 0, len(runs))
	for _, run := range runs {
		out = append(out, runToProto(run))
	}
	return &runstatev1.ListRunsResponse{Runs: out}, nil
}

func (s *Server) ResumeRun(_ context.Context, req *runstatev1.ResumeRunRequest) (*runstatev1.ResumeRunResponse, error) {
	if strings.TrimSpace(req.GetRunId()) == "" {
		return nil, serviceError(fmt.Errorf("%w: run_id is required", errInvalidArgument))
	}
	now := s.timestamp()
	run, err := s.records.updateRun(req.GetRunId(), func(run *runRecord) error {
		if run.Status == statusSucceeded || run.Status == statusCancelled {
			return fmt.Errorf("%w: cannot resume %s run", errConflict, run.Status)
		}
		run.Status = statusRunning
		run.UpdatedAt = now
		run.Metadata = mergeMetadata(run.Metadata, map[string]string{"resume_requested_at": now})
		for i := range run.Items {
			if run.Items[i].Status != statusSucceeded && run.Items[i].Status != statusSkipped {
				run.Items[i].Status = statusPending
			}
		}
		return nil
	})
	if err != nil {
		return nil, serviceError(err)
	}
	return &runstatev1.ResumeRunResponse{Run: runToProto(run)}, nil
}

func (s *Server) UpdateItemState(_ context.Context, req *runstatev1.UpdateItemStateRequest) (*runstatev1.UpdateItemStateResponse, error) {
	phase, err := validatePhase(req.GetPhase())
	if err != nil {
		return nil, serviceError(err)
	}
	status, err := statusFromProto(req.GetStatus(), statusRunning)
	if err != nil {
		return nil, serviceError(err)
	}
	now := s.timestamp()
	var updated itemRecord
	_, err = s.records.updateRun(req.GetRunId(), func(run *runRecord) error {
		item, err := itemByID(run, req.GetItemId())
		if err != nil {
			return err
		}
		updateItemPhase(item, phase, status, req.GetArtifactRef(), req.GetError(), req.GetMetadata(), req.GetServiceCallRefs(), now)
		run.UpdatedAt = now
		run.Status = aggregateStatus(run.Items)
		updated = cloneItem(*item)
		return nil
	})
	if err != nil {
		return nil, serviceError(err)
	}
	return &runstatev1.UpdateItemStateResponse{Item: itemToProto(updated)}, nil
}

func (s *Server) AppendArtifact(_ context.Context, req *runstatev1.AppendArtifactRequest) (*runstatev1.AppendArtifactResponse, error) {
	now := s.timestamp()
	artifact, err := artifactFromProto(req.GetArtifact(), req.GetItemId(), now)
	if err != nil {
		return nil, serviceError(err)
	}
	_, err = s.records.updateRun(req.GetRunId(), func(run *runRecord) error {
		if artifact.ItemID == "" {
			run.Metadata = mergeMetadata(run.Metadata, map[string]string{"last_artifact_ref": artifact.Ref})
			run.UpdatedAt = now
			return nil
		}
		item, err := itemByID(run, artifact.ItemID)
		if err != nil {
			return err
		}
		item.Artifacts = append(item.Artifacts, cloneArtifact(artifact))
		run.UpdatedAt = now
		return nil
	})
	if err != nil {
		return nil, serviceError(err)
	}
	return &runstatev1.AppendArtifactResponse{Artifact: artifactToProto(artifact)}, nil
}

func (s *Server) AppendReference(_ context.Context, req *runstatev1.AppendReferenceRequest) (*runstatev1.AppendReferenceResponse, error) {
	now := s.timestamp()
	ref, err := referenceFromProto(req.GetReference(), req.GetItemId(), now)
	if err != nil {
		return nil, serviceError(err)
	}
	_, err = s.records.updateRun(req.GetRunId(), func(run *runRecord) error {
		if ref.ItemID != "" {
			if _, err := itemByID(run, ref.ItemID); err != nil {
				return err
			}
		}
		run.References = append(run.References, cloneReference(ref))
		if ref.Kind == "service_call" {
			run.ServiceCallRefs = appendUnique(run.ServiceCallRefs, ref.Ref)
		}
		run.UpdatedAt = now
		return nil
	})
	if err != nil {
		return nil, serviceError(err)
	}
	return &runstatev1.AppendReferenceResponse{Reference: referenceToProto(ref)}, nil
}

func (s *Server) MarkFailed(_ context.Context, req *runstatev1.MarkFailedRequest) (*runstatev1.MarkFailedResponse, error) {
	now := s.timestamp()
	phase, err := validateOptionalPhase(req.GetPhase())
	if err != nil {
		return nil, serviceError(err)
	}
	run, err := s.records.updateRun(req.GetRunId(), func(run *runRecord) error {
		if strings.TrimSpace(req.GetItemId()) == "" {
			run.Status = statusFailed
			run.Metadata = mergeMetadata(run.Metadata, req.GetMetadata())
			run.Metadata["last_error"] = req.GetError()
			run.ServiceCallRefs = appendUnique(run.ServiceCallRefs, req.GetServiceCallRefs()...)
			run.UpdatedAt = now
			return nil
		}
		item, err := itemByID(run, req.GetItemId())
		if err != nil {
			return err
		}
		updateItemPhase(item, phase, statusFailed, "", req.GetError(), req.GetMetadata(), req.GetServiceCallRefs(), now)
		run.Status = aggregateStatus(run.Items)
		run.UpdatedAt = now
		return nil
	})
	if err != nil {
		return nil, serviceError(err)
	}
	return &runstatev1.MarkFailedResponse{Run: runToProto(run)}, nil
}

func (s *Server) MarkComplete(_ context.Context, req *runstatev1.MarkCompleteRequest) (*runstatev1.MarkCompleteResponse, error) {
	now := s.timestamp()
	phase, err := validateOptionalPhase(req.GetPhase())
	if err != nil {
		return nil, serviceError(err)
	}
	run, err := s.records.updateRun(req.GetRunId(), func(run *runRecord) error {
		if strings.TrimSpace(req.GetItemId()) == "" {
			for i := range run.Items {
				if run.Items[i].Status == statusSucceeded || run.Items[i].Status == statusSkipped {
					continue
				}
				updateItemPhase(&run.Items[i], phase, statusSucceeded, req.GetArtifactRef(), "", req.GetMetadata(), req.GetServiceCallRefs(), now)
			}
			run.Status = statusSucceeded
			run.ServiceCallRefs = appendUnique(run.ServiceCallRefs, req.GetServiceCallRefs()...)
			run.Metadata = mergeMetadata(run.Metadata, req.GetMetadata())
			run.UpdatedAt = now
			return nil
		}
		item, err := itemByID(run, req.GetItemId())
		if err != nil {
			return err
		}
		updateItemPhase(item, phase, statusSucceeded, req.GetArtifactRef(), "", req.GetMetadata(), req.GetServiceCallRefs(), now)
		run.Status = aggregateStatus(run.Items)
		run.UpdatedAt = now
		return nil
	})
	if err != nil {
		return nil, serviceError(err)
	}
	return &runstatev1.MarkCompleteResponse{Run: runToProto(run)}, nil
}

func (s *Server) CancelRun(_ context.Context, req *runstatev1.CancelRunRequest) (*runstatev1.CancelRunResponse, error) {
	if strings.TrimSpace(req.GetRunId()) == "" {
		return nil, serviceError(fmt.Errorf("%w: run_id is required", errInvalidArgument))
	}
	now := s.timestamp()
	run, err := s.records.updateRun(req.GetRunId(), func(run *runRecord) error {
		run.Status = statusCancelled
		run.Metadata = mergeMetadata(run.Metadata, map[string]string{"cancelled_at": now})
		if req.GetReason() != "" {
			run.Metadata["cancel_reason"] = req.GetReason()
		}
		for i := range run.Items {
			if run.Items[i].Status != statusSucceeded && run.Items[i].Status != statusSkipped {
				run.Items[i].Status = statusCancelled
			}
		}
		run.UpdatedAt = now
		return nil
	})
	if err != nil {
		return nil, serviceError(err)
	}
	return &runstatev1.CancelRunResponse{Run: runToProto(run)}, nil
}

func (s *Server) ListIncompleteItems(_ context.Context, req *runstatev1.ListIncompleteItemsRequest) (*runstatev1.ListIncompleteItemsResponse, error) {
	run, err := s.records.getRun(req.GetRunId())
	if err != nil {
		return nil, serviceError(err)
	}
	includePending, includeFailed := req.GetIncludePending(), req.GetIncludeFailed()
	if !includePending && !includeFailed {
		includePending, includeFailed = true, true
	}
	var items []*runstatev1.ItemState
	for _, item := range run.Items {
		if (includePending && (item.Status == statusPending || item.Status == statusRunning)) ||
			(includeFailed && item.Status == statusFailed) {
			items = append(items, itemToProto(item))
		}
	}
	return &runstatev1.ListIncompleteItemsResponse{Items: items}, nil
}

func (s *Server) ListArtifacts(_ context.Context, req *runstatev1.ListArtifactsRequest) (*runstatev1.ListArtifactsResponse, error) {
	run, err := s.records.getRun(req.GetRunId())
	if err != nil {
		return nil, serviceError(err)
	}
	var records []artifactRecord
	if req.GetItemId() == "" {
		for _, item := range run.Items {
			records = append(records, item.Artifacts...)
		}
	} else {
		item, err := itemByID(&run, req.GetItemId())
		if err != nil {
			return nil, serviceError(err)
		}
		records = append(records, item.Artifacts...)
	}
	out := make([]*runstatev1.Artifact, 0, len(records))
	for _, artifact := range records {
		out = append(out, artifactToProto(artifact))
	}
	return &runstatev1.ListArtifactsResponse{Artifacts: out}, nil
}

func (s *Server) AcquireLease(_ context.Context, req *runstatev1.AcquireLeaseRequest) (*runstatev1.AcquireLeaseResponse, error) {
	if strings.TrimSpace(req.GetRunId()) == "" || strings.TrimSpace(req.GetOwnerId()) == "" {
		return nil, serviceError(fmt.Errorf("%w: run_id and owner_id are required", errInvalidArgument))
	}
	if _, err := s.records.getRun(req.GetRunId()); err != nil {
		return nil, serviceError(err)
	}
	ttl, err := validateTTL(req.GetTtlSeconds())
	if err != nil {
		return nil, serviceError(err)
	}
	lease, err := s.leases.Acquire(leaseKey(req.GetRunId(), req.GetItemId()), req.GetRunId(), req.GetItemId(), req.GetOwnerId(), s.now().Add(ttl), s.now())
	if err != nil {
		return nil, serviceError(err)
	}
	return &runstatev1.AcquireLeaseResponse{Lease: leaseToProto(lease)}, nil
}

func (s *Server) RenewLease(_ context.Context, req *runstatev1.RenewLeaseRequest) (*runstatev1.RenewLeaseResponse, error) {
	ttl, err := validateTTL(req.GetTtlSeconds())
	if err != nil {
		return nil, serviceError(err)
	}
	lease, err := s.leases.Renew(req.GetKey(), req.GetOwnerId(), req.GetRevision(), s.now().Add(ttl), s.now())
	if err != nil {
		return nil, serviceError(err)
	}
	return &runstatev1.RenewLeaseResponse{Lease: leaseToProto(lease)}, nil
}

func (s *Server) ReleaseLease(_ context.Context, req *runstatev1.ReleaseLeaseRequest) (*runstatev1.ReleaseLeaseResponse, error) {
	if strings.TrimSpace(req.GetKey()) == "" || strings.TrimSpace(req.GetOwnerId()) == "" {
		return nil, serviceError(fmt.Errorf("%w: key and owner_id are required", errInvalidArgument))
	}
	if err := s.leases.Release(req.GetKey(), req.GetOwnerId(), req.GetRevision()); err != nil {
		return nil, serviceError(err)
	}
	return &runstatev1.ReleaseLeaseResponse{Key: req.GetKey()}, nil
}

func (s *Server) newRun(req *runstatev1.StartRunRequest) (runRecord, error) {
	if strings.TrimSpace(req.GetSpace()) == "" || len(req.GetItems()) == 0 {
		return runRecord{}, fmt.Errorf("%w: space and at least one item are required", errInvalidArgument)
	}
	nowTime := s.now()
	now := nowTime.UTC().Format(time.RFC3339Nano)
	title := strings.TrimSpace(req.GetTitle())
	if title == "" {
		title = "Execution run"
	}
	kind := strings.TrimSpace(req.GetKind())
	if kind == "" {
		kind = "general"
	}
	retention := req.GetRetentionDays()
	if retention <= 0 {
		retention = defaultRetentionDays
	}
	seen := make(map[string]int, len(req.GetItems()))
	items := make([]itemRecord, 0, len(req.GetItems()))
	for i, input := range req.GetItems() {
		item := itemFromInput(input, i)
		item.ID = uniqueID(item.ID, seen)
		items = append(items, item)
	}
	return runRecord{
		ID: newRunID(nowTime), Space: strings.TrimSpace(req.GetSpace()), Title: title, Kind: kind,
		ActorRef: strings.TrimSpace(req.GetActorRef()), Status: statusRunning, Items: items,
		CreatedAt: now, UpdatedAt: now, Metadata: cloneMap(req.GetMetadata()),
		RetentionExpiresAt: nowTime.AddDate(0, 0, int(retention)).UTC().Format(time.RFC3339Nano),
	}, nil
}

func itemFromInput(input *runstatev1.ItemInput, index int) itemRecord {
	seed := fmt.Sprintf("%s|%s|%s", input.GetContentHash(), input.GetResourceUri(), input.GetName())
	kind := strings.TrimSpace(input.GetKind())
	if kind == "" {
		kind = "resource"
	}
	return itemRecord{
		ID: itemID(seed, index), Kind: kind, ResourceURI: strings.TrimSpace(input.GetResourceUri()),
		Name: strings.TrimSpace(input.GetName()), ContentHash: strings.TrimSpace(input.GetContentHash()),
		Phase: "registered", Status: statusPending, Metadata: cloneMap(input.GetMetadata()),
	}
}

func itemByID(run *runRecord, id string) (*itemRecord, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("%w: item_id is required", errInvalidArgument)
	}
	for i := range run.Items {
		if run.Items[i].ID == id {
			return &run.Items[i], nil
		}
	}
	return nil, errNotFound
}

func updateItemPhase(item *itemRecord, phase string, status runStatus, artifactRef, lastErr string, metadata map[string]string, refs []string, now string) {
	item.Phase, item.Status, item.LastError = phase, status, lastErr
	item.Metadata = mergeMetadata(item.Metadata, metadata)
	item.ServiceCallRefs = appendUnique(item.ServiceCallRefs, refs...)
	if status == statusFailed {
		item.RetryCount++
	}
	found := false
	for i := range item.Phases {
		if item.Phases[i].Phase == phase {
			item.Phases[i] = phaseRecord{Phase: phase, Status: status, ArtifactRef: artifactRef, Error: lastErr, UpdatedAt: now, Metadata: cloneMap(metadata), ServiceCallRefs: cloneStrings(refs)}
			found = true
			break
		}
	}
	if !found {
		item.Phases = append(item.Phases, phaseRecord{Phase: phase, Status: status, ArtifactRef: artifactRef, Error: lastErr, UpdatedAt: now, Metadata: cloneMap(metadata), ServiceCallRefs: cloneStrings(refs)})
	}
	if artifactRef != "" {
		item.Artifacts = append(item.Artifacts, artifactRecord{Ref: artifactRef, Kind: phase, ItemID: item.ID, CreatedAt: now, Metadata: map[string]string{"recorded_by": "runstate"}})
	}
}

func validatePhase(phase string) (string, error) {
	phase = strings.TrimSpace(phase)
	if phase == "" {
		return "", fmt.Errorf("%w: phase is required", errInvalidArgument)
	}
	return phase, nil
}

func validateOptionalPhase(phase string) (string, error) {
	if strings.TrimSpace(phase) == "" {
		return "complete", nil
	}
	return validatePhase(phase)
}

func validateTTL(seconds int32) (time.Duration, error) {
	if seconds <= 0 {
		seconds = 300
	}
	if seconds > 600 {
		return 0, fmt.Errorf("%w: lease ttl_seconds must not exceed 600", errInvalidArgument)
	}
	return time.Duration(seconds) * time.Second, nil
}

func aggregateStatus(items []itemRecord) runStatus {
	if len(items) == 0 {
		return statusSucceeded
	}
	allSucceeded, failed, running := true, false, false
	for _, item := range items {
		switch item.Status {
		case statusFailed:
			failed, allSucceeded = true, false
		case statusPending, statusRunning:
			running, allSucceeded = true, false
		case statusCancelled:
			allSucceeded = false
		}
	}
	if running {
		return statusRunning
	}
	if failed {
		return statusFailed
	}
	if allSucceeded {
		return statusSucceeded
	}
	return statusRunning
}

func appendUnique(existing []string, values ...string) []string {
	seen := make(map[string]struct{}, len(existing)+len(values))
	out := make([]string, 0, len(existing)+len(values))
	for _, value := range append(existing, values...) {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func mergeMetadata(base, extra map[string]string) map[string]string {
	out := cloneMap(base)
	if len(extra) == 0 {
		return out
	}
	if out == nil {
		out = make(map[string]string, len(extra))
	}
	for key, value := range extra {
		out[key] = value
	}
	return out
}

func leaseToProto(lease leaseRecord) *runstatev1.Lease {
	return &runstatev1.Lease{Key: lease.Key, OwnerId: lease.OwnerID, RunId: lease.RunID, ItemId: lease.ItemID, ExpiresAt: lease.ExpiresAt, Revision: lease.Revision}
}

func (s *Server) timestamp() string { return s.now().UTC().Format(time.RFC3339Nano) }

func serviceError(err error) error {
	switch {
	case errors.Is(err, errInvalidArgument):
		return serviceerrors.InvalidArgument(err.Error())
	case errors.Is(err, errNotFound):
		return serviceerrors.NotFound(err.Error())
	case errors.Is(err, errConflict):
		return serviceerrors.FailedPrecondition(err.Error())
	default:
		return serviceerrors.Internal(err.Error())
	}
}
