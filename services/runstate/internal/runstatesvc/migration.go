package runstatesvc

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

type legacyState struct {
	Runs []legacyRun `json:"runs"`
}

type legacyRun struct {
	ID        string            `json:"id"`
	Space     string            `json:"space"`
	Title     string            `json:"title"`
	Status    runStatus         `json:"status"`
	Sources   []legacySource    `json:"sources"`
	CreatedAt string            `json:"created_at"`
	UpdatedAt string            `json:"updated_at"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type legacySource struct {
	ID          string            `json:"id"`
	SourceURI   string            `json:"source_uri,omitempty"`
	Filename    string            `json:"filename,omitempty"`
	SourceHash  string            `json:"source_hash,omitempty"`
	Phase       string            `json:"phase"`
	Status      runStatus         `json:"status"`
	LastError   string            `json:"last_error,omitempty"`
	Artifacts   []legacyArtifact  `json:"artifacts,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	FilePath    string            `json:"file_path,omitempty"`
	Extraction  legacyStep        `json:"extraction"`
	Structuring legacyStep        `json:"structuring"`
	Embedding   legacyStep        `json:"embedding"`
	Indexing    legacyStep        `json:"indexing"`
	Citation    legacyStep        `json:"citation"`
	RetryCount  int32             `json:"retry_count,omitempty"`
}

type legacyArtifact struct {
	Ref       string            `json:"ref"`
	Kind      string            `json:"kind"`
	SourceID  string            `json:"source_id,omitempty"`
	CreatedAt string            `json:"created_at"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type legacyStep struct {
	Phase       string            `json:"phase"`
	Status      runStatus         `json:"status"`
	ArtifactRef string            `json:"artifact_ref,omitempty"`
	Error       string            `json:"error,omitempty"`
	UpdatedAt   string            `json:"updated_at"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

func loadLegacyState(path string) (persistedState, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return persistedState{}, nil
	}
	if err != nil {
		return persistedState{}, fmt.Errorf("read legacy ingestion state: %w", err)
	}
	var old legacyState
	if err := json.Unmarshal(data, &old); err != nil {
		return persistedState{}, fmt.Errorf("decode legacy ingestion state: %w", err)
	}
	state := persistedState{Runs: make([]runRecord, 0, len(old.Runs))}
	for _, legacy := range old.Runs {
		run := runRecord{
			ID: legacy.ID, Space: legacy.Space, Title: legacy.Title,
			Kind: "knowledge.index", Status: legacy.Status,
			CreatedAt: legacy.CreatedAt, UpdatedAt: legacy.UpdatedAt,
			Metadata: cloneMap(legacy.Metadata),
		}
		run.Metadata = mergeMetadata(run.Metadata, map[string]string{"migrated_from": "ingestion-state.json"})
		for _, source := range legacy.Sources {
			item := itemRecord{
				ID: source.ID, Kind: "document", ResourceURI: source.SourceURI,
				Name: source.Filename, ContentHash: source.SourceHash, Phase: source.Phase,
				Status: source.Status, LastError: source.LastError,
				Metadata: cloneMap(source.Metadata), RetryCount: source.RetryCount,
			}
			if source.FilePath != "" {
				item.Metadata = mergeMetadata(item.Metadata, map[string]string{"file_path": source.FilePath})
			}
			for _, artifact := range source.Artifacts {
				item.Artifacts = append(item.Artifacts, artifactRecord{
					Ref: artifact.Ref, Kind: artifact.Kind, ItemID: source.ID,
					CreatedAt: artifact.CreatedAt, Metadata: cloneMap(artifact.Metadata),
				})
			}
			for _, step := range []legacyStep{source.Extraction, source.Structuring, source.Embedding, source.Indexing, source.Citation} {
				if step.Phase == "" {
					continue
				}
				item.Phases = append(item.Phases, phaseRecord{
					Phase: step.Phase, Status: step.Status, ArtifactRef: step.ArtifactRef,
					Error: step.Error, UpdatedAt: step.UpdatedAt, Metadata: cloneMap(step.Metadata),
				})
			}
			run.Items = append(run.Items, item)
		}
		state.Runs = append(state.Runs, run)
	}
	return state, nil
}
