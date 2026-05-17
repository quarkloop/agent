package ingestionsvc

import (
	"fmt"

	ingestionv1 "github.com/quarkloop/pkg/serviceapi/gen/quark/ingestion/v1"
)

func phaseFromProto(in ingestionv1.SourcePhase) (phase, error) {
	switch in {
	case ingestionv1.SourcePhase_SOURCE_PHASE_REGISTERED, ingestionv1.SourcePhase_SOURCE_PHASE_UNSPECIFIED:
		return phaseRegistered, nil
	case ingestionv1.SourcePhase_SOURCE_PHASE_PARSED:
		return phaseParsed, nil
	case ingestionv1.SourcePhase_SOURCE_PHASE_STRUCTURED:
		return phaseStructured, nil
	case ingestionv1.SourcePhase_SOURCE_PHASE_EMBEDDED:
		return phaseEmbedded, nil
	case ingestionv1.SourcePhase_SOURCE_PHASE_INDEXED:
		return phaseIndexed, nil
	case ingestionv1.SourcePhase_SOURCE_PHASE_CITED:
		return phaseCited, nil
	default:
		return "", fmt.Errorf("%w: unknown source phase %d", errInvalidArgument, in)
	}
}

func phaseToProto(in phase) ingestionv1.SourcePhase {
	switch in {
	case phaseRegistered:
		return ingestionv1.SourcePhase_SOURCE_PHASE_REGISTERED
	case phaseParsed:
		return ingestionv1.SourcePhase_SOURCE_PHASE_PARSED
	case phaseStructured:
		return ingestionv1.SourcePhase_SOURCE_PHASE_STRUCTURED
	case phaseEmbedded:
		return ingestionv1.SourcePhase_SOURCE_PHASE_EMBEDDED
	case phaseIndexed:
		return ingestionv1.SourcePhase_SOURCE_PHASE_INDEXED
	case phaseCited:
		return ingestionv1.SourcePhase_SOURCE_PHASE_CITED
	default:
		return ingestionv1.SourcePhase_SOURCE_PHASE_UNSPECIFIED
	}
}

func statusFromProto(in ingestionv1.SourceStatus, fallback sourceStatus) (sourceStatus, error) {
	switch in {
	case ingestionv1.SourceStatus_SOURCE_STATUS_UNSPECIFIED:
		return fallback, nil
	case ingestionv1.SourceStatus_SOURCE_STATUS_PENDING:
		return statusPending, nil
	case ingestionv1.SourceStatus_SOURCE_STATUS_RUNNING:
		return statusRunning, nil
	case ingestionv1.SourceStatus_SOURCE_STATUS_SUCCEEDED:
		return statusSucceeded, nil
	case ingestionv1.SourceStatus_SOURCE_STATUS_FAILED:
		return statusFailed, nil
	case ingestionv1.SourceStatus_SOURCE_STATUS_SKIPPED:
		return statusSkipped, nil
	case ingestionv1.SourceStatus_SOURCE_STATUS_CANCELLED:
		return statusCancelled, nil
	default:
		return "", fmt.Errorf("%w: unknown source status %d", errInvalidArgument, in)
	}
}

func statusToProto(in sourceStatus) ingestionv1.SourceStatus {
	switch in {
	case statusPending:
		return ingestionv1.SourceStatus_SOURCE_STATUS_PENDING
	case statusRunning:
		return ingestionv1.SourceStatus_SOURCE_STATUS_RUNNING
	case statusSucceeded:
		return ingestionv1.SourceStatus_SOURCE_STATUS_SUCCEEDED
	case statusFailed:
		return ingestionv1.SourceStatus_SOURCE_STATUS_FAILED
	case statusSkipped:
		return ingestionv1.SourceStatus_SOURCE_STATUS_SKIPPED
	case statusCancelled:
		return ingestionv1.SourceStatus_SOURCE_STATUS_CANCELLED
	default:
		return ingestionv1.SourceStatus_SOURCE_STATUS_UNSPECIFIED
	}
}

func runToProto(run runRecord) *ingestionv1.IngestionRun {
	sources := make([]*ingestionv1.SourceState, 0, len(run.Sources))
	for _, source := range run.Sources {
		sources = append(sources, sourceToProto(source))
	}
	return &ingestionv1.IngestionRun{
		Id:        run.ID,
		Space:     run.Space,
		Title:     run.Title,
		Status:    statusToProto(run.Status),
		Sources:   sources,
		CreatedAt: run.CreatedAt,
		UpdatedAt: run.UpdatedAt,
		Metadata:  cloneMap(run.Metadata),
	}
}

func sourceToProto(source sourceRecord) *ingestionv1.SourceState {
	artifacts := make([]*ingestionv1.IngestionArtifact, 0, len(source.Artifacts))
	for _, artifact := range source.Artifacts {
		artifacts = append(artifacts, artifactToProto(artifact))
	}
	return &ingestionv1.SourceState{
		Id:          source.ID,
		SourceUri:   source.SourceURI,
		Filename:    source.Filename,
		SourceHash:  source.SourceHash,
		Phase:       phaseToProto(source.Phase),
		Status:      statusToProto(source.Status),
		LastError:   source.LastError,
		Artifacts:   artifacts,
		Metadata:    cloneMap(source.Metadata),
		FilePath:    source.FilePath,
		Extraction:  stepToProto(source.Extraction),
		Structuring: stepToProto(source.Structuring),
		Embedding:   stepToProto(source.Embedding),
		Indexing:    stepToProto(source.Indexing),
		Citation:    stepToProto(source.Citation),
		RetryCount:  source.RetryCount,
	}
}

func artifactToProto(artifact artifactRecord) *ingestionv1.IngestionArtifact {
	return &ingestionv1.IngestionArtifact{
		Ref:       artifact.Ref,
		Kind:      artifact.Kind,
		SourceId:  artifact.SourceID,
		CreatedAt: artifact.CreatedAt,
		Metadata:  cloneMap(artifact.Metadata),
	}
}

func stepToProto(step stepRecord) *ingestionv1.SourceStepState {
	return &ingestionv1.SourceStepState{
		Phase:       phaseToProto(step.Phase),
		Status:      statusToProto(step.Status),
		ArtifactRef: step.ArtifactRef,
		Error:       step.Error,
		UpdatedAt:   step.UpdatedAt,
		Metadata:    cloneMap(step.Metadata),
	}
}

func artifactFromProto(in *ingestionv1.IngestionArtifact, sourceID, now string) (artifactRecord, error) {
	if in == nil {
		return artifactRecord{}, fmt.Errorf("%w: artifact is required", errInvalidArgument)
	}
	if in.GetRef() == "" {
		return artifactRecord{}, fmt.Errorf("%w: artifact ref is required", errInvalidArgument)
	}
	kind := in.GetKind()
	if kind == "" {
		kind = "artifact"
	}
	createdAt := in.GetCreatedAt()
	if createdAt == "" {
		createdAt = now
	}
	if sourceID == "" {
		sourceID = in.GetSourceId()
	}
	return artifactRecord{
		Ref:       in.GetRef(),
		Kind:      kind,
		SourceID:  sourceID,
		CreatedAt: createdAt,
		Metadata:  cloneMap(in.GetMetadata()),
	}, nil
}
