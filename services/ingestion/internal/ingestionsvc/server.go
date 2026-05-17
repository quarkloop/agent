package ingestionsvc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	ingestionv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/ingestion/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	ingestionv1.UnimplementedIngestionServiceServer

	store *fileStore
	now   func() time.Time
}

func New(root string) (*Server, error) {
	store, err := newFileStore(root)
	if err != nil {
		return nil, err
	}
	return &Server{store: store, now: time.Now}, nil
}

func (s *Server) StartRun(_ context.Context, req *ingestionv1.StartRunRequest) (*ingestionv1.StartRunResponse, error) {
	run, err := s.newRun(req)
	if err != nil {
		return nil, grpcError(err)
	}
	stored, err := s.store.createRun(run)
	if err != nil {
		return nil, grpcError(err)
	}
	return &ingestionv1.StartRunResponse{Run: runToProto(stored)}, nil
}

func (s *Server) GetRun(_ context.Context, req *ingestionv1.GetRunRequest) (*ingestionv1.GetRunResponse, error) {
	if strings.TrimSpace(req.GetRunId()) == "" {
		return nil, grpcError(fmt.Errorf("%w: run_id is required", errInvalidArgument))
	}
	run, err := s.store.getRun(req.GetRunId())
	if err != nil {
		return nil, grpcError(err)
	}
	return &ingestionv1.GetRunResponse{Run: runToProto(run)}, nil
}

func (s *Server) ListRuns(_ context.Context, req *ingestionv1.ListRunsRequest) (*ingestionv1.ListRunsResponse, error) {
	filter, err := statusFromProto(req.GetStatus(), "")
	if err != nil {
		return nil, grpcError(err)
	}
	runs, err := s.store.listRuns(strings.TrimSpace(req.GetSpace()), filter, int(req.GetLimit()))
	if err != nil {
		return nil, grpcError(err)
	}
	out := make([]*ingestionv1.IngestionRun, 0, len(runs))
	for _, run := range runs {
		out = append(out, runToProto(run))
	}
	return &ingestionv1.ListRunsResponse{Runs: out}, nil
}

func (s *Server) ResumeRun(_ context.Context, req *ingestionv1.ResumeRunRequest) (*ingestionv1.ResumeRunResponse, error) {
	if strings.TrimSpace(req.GetRunId()) == "" {
		return nil, grpcError(fmt.Errorf("%w: run_id is required", errInvalidArgument))
	}
	now := s.timestamp()
	run, err := s.store.updateRun(req.GetRunId(), func(run *runRecord) error {
		if run.Status == statusSucceeded || run.Status == statusCancelled {
			return fmt.Errorf("%w: cannot resume %s run", errConflict, run.Status)
		}
		run.Status = statusRunning
		run.UpdatedAt = now
		if run.Metadata == nil {
			run.Metadata = make(map[string]string)
		}
		run.Metadata["resume_requested_at"] = now
		for i := range run.Sources {
			if run.Sources[i].Status == statusSucceeded || run.Sources[i].Status == statusSkipped {
				continue
			}
			run.Sources[i].Status = statusPending
			run.Sources[i].UpdatedAt(now)
		}
		return nil
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &ingestionv1.ResumeRunResponse{Run: runToProto(run)}, nil
}

func (s *Server) UpdateSourceState(_ context.Context, req *ingestionv1.UpdateSourceStateRequest) (*ingestionv1.UpdateSourceStateResponse, error) {
	ph, err := phaseFromProto(req.GetPhase())
	if err != nil {
		return nil, grpcError(err)
	}
	st, err := statusFromProto(req.GetStatus(), statusRunning)
	if err != nil {
		return nil, grpcError(err)
	}
	now := s.timestamp()
	var updated sourceRecord
	_, err = s.store.updateRun(req.GetRunId(), func(run *runRecord) error {
		source, err := sourceByID(run, req.GetSourceId())
		if err != nil {
			return err
		}
		updateSourceStep(source, ph, st, req.GetArtifactRef(), req.GetError(), req.GetMetadata(), now)
		run.UpdatedAt = now
		run.Status = aggregateRunStatus(run.Sources)
		updated = cloneSource(*source)
		return nil
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &ingestionv1.UpdateSourceStateResponse{Source: sourceToProto(updated)}, nil
}

func (s *Server) AppendArtifact(_ context.Context, req *ingestionv1.AppendArtifactRequest) (*ingestionv1.AppendArtifactResponse, error) {
	now := s.timestamp()
	artifact, err := artifactFromProto(req.GetArtifact(), req.GetSourceId(), now)
	if err != nil {
		return nil, grpcError(err)
	}
	_, err = s.store.updateRun(req.GetRunId(), func(run *runRecord) error {
		if req.GetSourceId() == "" {
			run.Metadata = mergeMetadata(run.Metadata, map[string]string{"last_artifact_ref": artifact.Ref})
			run.UpdatedAt = now
			return nil
		}
		source, err := sourceByID(run, req.GetSourceId())
		if err != nil {
			return err
		}
		source.Artifacts = append(source.Artifacts, cloneArtifact(artifact))
		source.UpdatedAt(now)
		run.UpdatedAt = now
		return nil
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &ingestionv1.AppendArtifactResponse{Artifact: artifactToProto(artifact)}, nil
}

func (s *Server) MarkFailed(_ context.Context, req *ingestionv1.MarkFailedRequest) (*ingestionv1.MarkFailedResponse, error) {
	ph, err := phaseFromProto(req.GetPhase())
	if err != nil {
		return nil, grpcError(err)
	}
	now := s.timestamp()
	run, err := s.store.updateRun(req.GetRunId(), func(run *runRecord) error {
		if req.GetSourceId() == "" {
			run.Status = statusFailed
			run.Metadata = mergeMetadata(run.Metadata, req.GetMetadata())
			run.Metadata["last_error"] = req.GetError()
			run.UpdatedAt = now
			return nil
		}
		source, err := sourceByID(run, req.GetSourceId())
		if err != nil {
			return err
		}
		updateSourceStep(source, ph, statusFailed, "", req.GetError(), req.GetMetadata(), now)
		run.UpdatedAt = now
		run.Status = aggregateRunStatus(run.Sources)
		return nil
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &ingestionv1.MarkFailedResponse{Run: runToProto(run)}, nil
}

func (s *Server) MarkComplete(_ context.Context, req *ingestionv1.MarkCompleteRequest) (*ingestionv1.MarkCompleteResponse, error) {
	ph, err := phaseFromProto(req.GetPhase())
	if err != nil {
		return nil, grpcError(err)
	}
	now := s.timestamp()
	run, err := s.store.updateRun(req.GetRunId(), func(run *runRecord) error {
		if req.GetSourceId() == "" {
			for i := range run.Sources {
				updateSourceStep(&run.Sources[i], phaseCited, statusSucceeded, "", "", nil, now)
			}
			run.Status = statusSucceeded
			run.UpdatedAt = now
			return nil
		}
		source, err := sourceByID(run, req.GetSourceId())
		if err != nil {
			return err
		}
		updateSourceStep(source, ph, statusSucceeded, req.GetArtifactRef(), "", req.GetMetadata(), now)
		run.UpdatedAt = now
		run.Status = aggregateRunStatus(run.Sources)
		return nil
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &ingestionv1.MarkCompleteResponse{Run: runToProto(run)}, nil
}

func (s *Server) CancelRun(_ context.Context, req *ingestionv1.CancelRunRequest) (*ingestionv1.CancelRunResponse, error) {
	if strings.TrimSpace(req.GetRunId()) == "" {
		return nil, grpcError(fmt.Errorf("%w: run_id is required", errInvalidArgument))
	}
	now := s.timestamp()
	run, err := s.store.updateRun(req.GetRunId(), func(run *runRecord) error {
		run.Status = statusCancelled
		if run.Metadata == nil {
			run.Metadata = make(map[string]string)
		}
		run.Metadata["cancelled_at"] = now
		if req.GetReason() != "" {
			run.Metadata["cancel_reason"] = req.GetReason()
		}
		for i := range run.Sources {
			if run.Sources[i].Status == statusSucceeded || run.Sources[i].Status == statusSkipped {
				continue
			}
			run.Sources[i].Status = statusCancelled
			run.Sources[i].UpdatedAt(now)
		}
		run.UpdatedAt = now
		return nil
	})
	if err != nil {
		return nil, grpcError(err)
	}
	return &ingestionv1.CancelRunResponse{Run: runToProto(run)}, nil
}

func (s *Server) ListIncompleteSources(_ context.Context, req *ingestionv1.ListIncompleteSourcesRequest) (*ingestionv1.ListIncompleteSourcesResponse, error) {
	run, err := s.store.getRun(req.GetRunId())
	if err != nil {
		return nil, grpcError(err)
	}
	includePending := req.GetIncludePending()
	includeFailed := req.GetIncludeFailed()
	if !includePending && !includeFailed {
		includePending = true
		includeFailed = true
	}
	var out []*ingestionv1.SourceState
	for _, source := range run.Sources {
		if source.Status == statusPending || source.Status == statusRunning {
			if includePending {
				out = append(out, sourceToProto(source))
			}
			continue
		}
		if source.Status == statusFailed && includeFailed {
			out = append(out, sourceToProto(source))
		}
	}
	return &ingestionv1.ListIncompleteSourcesResponse{Sources: out}, nil
}

func (s *Server) ListArtifacts(_ context.Context, req *ingestionv1.ListArtifactsRequest) (*ingestionv1.ListArtifactsResponse, error) {
	run, err := s.store.getRun(req.GetRunId())
	if err != nil {
		return nil, grpcError(err)
	}
	var artifacts []artifactRecord
	if req.GetSourceId() == "" {
		for _, source := range run.Sources {
			artifacts = append(artifacts, source.Artifacts...)
		}
	} else {
		source, err := sourceByID(&run, req.GetSourceId())
		if err != nil {
			return nil, grpcError(err)
		}
		artifacts = append(artifacts, source.Artifacts...)
	}
	out := make([]*ingestionv1.IngestionArtifact, 0, len(artifacts))
	for _, artifact := range artifacts {
		out = append(out, artifactToProto(artifact))
	}
	return &ingestionv1.ListArtifactsResponse{Artifacts: out}, nil
}

func (s *Server) newRun(req *ingestionv1.StartRunRequest) (runRecord, error) {
	if strings.TrimSpace(req.GetSpace()) == "" {
		return runRecord{}, fmt.Errorf("%w: space is required", errInvalidArgument)
	}
	if len(req.GetSources()) == 0 {
		return runRecord{}, fmt.Errorf("%w: at least one source is required", errInvalidArgument)
	}
	nowTime := s.now()
	now := nowTime.UTC().Format(time.RFC3339Nano)
	title := strings.TrimSpace(req.GetTitle())
	if title == "" {
		title = "Ingestion run"
	}
	seen := make(map[string]int, len(req.GetSources()))
	sources := make([]sourceRecord, 0, len(req.GetSources()))
	for i, input := range req.GetSources() {
		source := sourceFromInput(input, i, now)
		source.ID = uniqueSourceID(source.ID, seen)
		sources = append(sources, source)
	}
	return runRecord{
		ID:        newRunID(nowTime),
		Space:     strings.TrimSpace(req.GetSpace()),
		Title:     title,
		Status:    statusRunning,
		Sources:   sources,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  cloneMap(req.GetMetadata()),
	}, nil
}

func sourceFromInput(input *ingestionv1.SourceInput, index int, now string) sourceRecord {
	seed := fmt.Sprintf("%s|%s|%s|%s", input.GetSourceHash(), input.GetSourceUri(), input.GetFilePath(), input.GetFilename())
	source := sourceRecord{
		ID:          sourceID(seed, index),
		SourceURI:   strings.TrimSpace(input.GetSourceUri()),
		Filename:    strings.TrimSpace(input.GetFilename()),
		SourceHash:  strings.TrimSpace(input.GetSourceHash()),
		Phase:       phaseRegistered,
		Status:      statusPending,
		Metadata:    cloneMap(input.GetMetadata()),
		FilePath:    strings.TrimSpace(input.GetFilePath()),
		Extraction:  newStep(phaseParsed, now),
		Structuring: newStep(phaseStructured, now),
		Embedding:   newStep(phaseEmbedded, now),
		Indexing:    newStep(phaseIndexed, now),
		Citation:    newStep(phaseCited, now),
	}
	if source.Metadata == nil {
		source.Metadata = make(map[string]string)
	}
	source.Metadata["retry_count"] = "0"
	return source
}

func newStep(ph phase, now string) stepRecord {
	return stepRecord{Phase: ph, Status: statusPending, UpdatedAt: now}
}

func sourceByID(run *runRecord, sourceID string) (*sourceRecord, error) {
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" {
		return nil, fmt.Errorf("%w: source_id is required", errInvalidArgument)
	}
	for i := range run.Sources {
		if run.Sources[i].ID == sourceID {
			return &run.Sources[i], nil
		}
	}
	return nil, errNotFound
}

func updateSourceStep(source *sourceRecord, ph phase, st sourceStatus, artifactRef, lastErr string, metadata map[string]string, now string) {
	source.Phase = ph
	source.Status = st
	source.LastError = lastErr
	source.Metadata = mergeMetadata(source.Metadata, metadata)
	if st == statusFailed {
		source.RetryCount++
		if source.Metadata == nil {
			source.Metadata = make(map[string]string)
		}
		source.Metadata["retry_count"] = fmt.Sprint(source.RetryCount)
	}
	step := stepForPhase(source, ph)
	if step != nil {
		step.Status = st
		step.ArtifactRef = artifactRef
		step.Error = lastErr
		step.Metadata = mergeMetadata(step.Metadata, metadata)
		step.UpdatedAt = now
	}
	if artifactRef != "" {
		source.Artifacts = append(source.Artifacts, artifactRecord{
			Ref:       artifactRef,
			Kind:      lowerPhase(ph),
			SourceID:  source.ID,
			CreatedAt: now,
			Metadata:  map[string]string{"generated_by": "ingestion_state_update"},
		})
	}
	source.UpdatedAt(now)
}

func stepForPhase(source *sourceRecord, ph phase) *stepRecord {
	switch ph {
	case phaseParsed:
		return &source.Extraction
	case phaseStructured:
		return &source.Structuring
	case phaseEmbedded:
		return &source.Embedding
	case phaseIndexed:
		return &source.Indexing
	case phaseCited:
		return &source.Citation
	default:
		return nil
	}
}

func (s *sourceRecord) UpdatedAt(now string) {
	if s.Metadata == nil {
		s.Metadata = make(map[string]string)
	}
	s.Metadata["updated_at"] = now
}

func mergeMetadata(base map[string]string, extra map[string]string) map[string]string {
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

func aggregateRunStatus(sources []sourceRecord) sourceStatus {
	if len(sources) == 0 {
		return statusSucceeded
	}
	allTerminalSuccess := true
	anyFailed := false
	anyRunning := false
	for _, source := range sources {
		switch source.Status {
		case statusFailed:
			anyFailed = true
			allTerminalSuccess = false
		case statusPending, statusRunning:
			anyRunning = true
			allTerminalSuccess = false
		case statusCancelled:
			allTerminalSuccess = false
		}
	}
	if anyRunning {
		return statusRunning
	}
	if anyFailed {
		return statusFailed
	}
	if allTerminalSuccess {
		return statusSucceeded
	}
	return statusRunning
}

func (s *Server) timestamp() string {
	return s.now().UTC().Format(time.RFC3339Nano)
}

func grpcError(err error) error {
	switch {
	case errors.Is(err, errInvalidArgument):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, errNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, errConflict):
		return status.Error(codes.FailedPrecondition, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
