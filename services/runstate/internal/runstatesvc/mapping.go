package runstatesvc

import (
	"fmt"
	"strings"

	runstatev1 "github.com/quarkloop/pkg/serviceapi/gen/quark/runstate/v1"
)

func statusFromProto(in runstatev1.RunStatus, fallback runStatus) (runStatus, error) {
	switch in {
	case runstatev1.RunStatus_RUN_STATUS_UNSPECIFIED:
		return fallback, nil
	case runstatev1.RunStatus_RUN_STATUS_PENDING:
		return statusPending, nil
	case runstatev1.RunStatus_RUN_STATUS_RUNNING:
		return statusRunning, nil
	case runstatev1.RunStatus_RUN_STATUS_SUCCEEDED:
		return statusSucceeded, nil
	case runstatev1.RunStatus_RUN_STATUS_FAILED:
		return statusFailed, nil
	case runstatev1.RunStatus_RUN_STATUS_SKIPPED:
		return statusSkipped, nil
	case runstatev1.RunStatus_RUN_STATUS_CANCELLED:
		return statusCancelled, nil
	default:
		return "", fmt.Errorf("%w: unknown run status %d", errInvalidArgument, in)
	}
}

func statusToProto(in runStatus) runstatev1.RunStatus {
	switch in {
	case statusPending:
		return runstatev1.RunStatus_RUN_STATUS_PENDING
	case statusRunning:
		return runstatev1.RunStatus_RUN_STATUS_RUNNING
	case statusSucceeded:
		return runstatev1.RunStatus_RUN_STATUS_SUCCEEDED
	case statusFailed:
		return runstatev1.RunStatus_RUN_STATUS_FAILED
	case statusSkipped:
		return runstatev1.RunStatus_RUN_STATUS_SKIPPED
	case statusCancelled:
		return runstatev1.RunStatus_RUN_STATUS_CANCELLED
	default:
		return runstatev1.RunStatus_RUN_STATUS_UNSPECIFIED
	}
}

func runToProto(run runRecord) *runstatev1.Run {
	items := make([]*runstatev1.ItemState, 0, len(run.Items))
	for _, item := range run.Items {
		items = append(items, itemToProto(item))
	}
	references := make([]*runstatev1.Reference, 0, len(run.References))
	for _, ref := range run.References {
		references = append(references, referenceToProto(ref))
	}
	return &runstatev1.Run{
		Id: run.ID, Space: run.Space, Title: run.Title, Kind: run.Kind,
		ActorRef: run.ActorRef, Status: statusToProto(run.Status), Items: items,
		References: references, ServiceCallRefs: cloneStrings(run.ServiceCallRefs),
		CreatedAt: run.CreatedAt, UpdatedAt: run.UpdatedAt,
		RetentionExpiresAt: run.RetentionExpiresAt, Metadata: cloneMap(run.Metadata),
	}
}

func itemToProto(item itemRecord) *runstatev1.ItemState {
	artifacts := make([]*runstatev1.Artifact, 0, len(item.Artifacts))
	for _, artifact := range item.Artifacts {
		artifacts = append(artifacts, artifactToProto(artifact))
	}
	phases := make([]*runstatev1.PhaseState, 0, len(item.Phases))
	for _, phase := range item.Phases {
		phases = append(phases, phaseToProto(phase))
	}
	return &runstatev1.ItemState{
		Id: item.ID, Kind: item.Kind, ResourceUri: item.ResourceURI, Name: item.Name,
		ContentHash: item.ContentHash, Phase: item.Phase, Status: statusToProto(item.Status),
		LastError: item.LastError, Artifacts: artifacts, Metadata: cloneMap(item.Metadata),
		Phases: phases, RetryCount: item.RetryCount,
		ServiceCallRefs: cloneStrings(item.ServiceCallRefs),
	}
}

func artifactToProto(artifact artifactRecord) *runstatev1.Artifact {
	return &runstatev1.Artifact{
		Ref: artifact.Ref, Kind: artifact.Kind, ItemId: artifact.ItemID,
		CreatedAt: artifact.CreatedAt, Metadata: cloneMap(artifact.Metadata),
	}
}

func referenceToProto(ref referenceRecord) *runstatev1.Reference {
	return &runstatev1.Reference{
		Ref: ref.Ref, Kind: ref.Kind, ItemId: ref.ItemID,
		CreatedAt: ref.CreatedAt, Metadata: cloneMap(ref.Metadata),
	}
}

func phaseToProto(phase phaseRecord) *runstatev1.PhaseState {
	return &runstatev1.PhaseState{
		Phase: phase.Phase, Status: statusToProto(phase.Status),
		ArtifactRef: phase.ArtifactRef, Error: phase.Error, UpdatedAt: phase.UpdatedAt,
		Metadata: cloneMap(phase.Metadata), ServiceCallRefs: cloneStrings(phase.ServiceCallRefs),
	}
}

func artifactFromProto(in *runstatev1.Artifact, itemID, now string) (artifactRecord, error) {
	if in == nil || strings.TrimSpace(in.GetRef()) == "" {
		return artifactRecord{}, fmt.Errorf("%w: artifact ref is required", errInvalidArgument)
	}
	kind := strings.TrimSpace(in.GetKind())
	if kind == "" {
		kind = "artifact"
	}
	createdAt := in.GetCreatedAt()
	if createdAt == "" {
		createdAt = now
	}
	if itemID == "" {
		itemID = strings.TrimSpace(in.GetItemId())
	}
	return artifactRecord{Ref: in.GetRef(), Kind: kind, ItemID: itemID, CreatedAt: createdAt, Metadata: cloneMap(in.GetMetadata())}, nil
}

func referenceFromProto(in *runstatev1.Reference, itemID, now string) (referenceRecord, error) {
	if in == nil || strings.TrimSpace(in.GetRef()) == "" {
		return referenceRecord{}, fmt.Errorf("%w: reference ref is required", errInvalidArgument)
	}
	kind := strings.TrimSpace(in.GetKind())
	if kind == "" {
		kind = "service_call"
	}
	createdAt := in.GetCreatedAt()
	if createdAt == "" {
		createdAt = now
	}
	if itemID == "" {
		itemID = strings.TrimSpace(in.GetItemId())
	}
	return referenceRecord{Ref: in.GetRef(), Kind: kind, ItemID: itemID, CreatedAt: createdAt, Metadata: cloneMap(in.GetMetadata())}, nil
}
