package ingestionsvc

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

type fileStore struct {
	mu   sync.Mutex
	path string
}

type persistedState struct {
	Runs []runRecord `json:"runs"`
}

func newFileStore(root string) (*fileStore, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create ingestion state root: %w", err)
	}
	store := &fileStore{path: filepath.Join(root, "ingestion-state.json")}
	if _, err := os.Stat(store.path); errors.Is(err, os.ErrNotExist) {
		if err := store.saveLocked(persistedState{}); err != nil {
			return nil, err
		}
	}
	return store, nil
}

func (s *fileStore) createRun(run runRecord) (runRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadLocked()
	if err != nil {
		return runRecord{}, err
	}
	state.Runs = append(state.Runs, cloneRun(run))
	if err := s.saveLocked(state); err != nil {
		return runRecord{}, err
	}
	return cloneRun(run), nil
}

func (s *fileStore) getRun(id string) (runRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadLocked()
	if err != nil {
		return runRecord{}, err
	}
	for _, run := range state.Runs {
		if run.ID == id {
			return cloneRun(run), nil
		}
	}
	return runRecord{}, errNotFound
}

func (s *fileStore) listRuns(space string, status sourceStatus, limit int) ([]runRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadLocked()
	if err != nil {
		return nil, err
	}
	runs := make([]runRecord, 0, len(state.Runs))
	for _, run := range state.Runs {
		if space != "" && run.Space != space {
			continue
		}
		if status != "" && run.Status != status {
			continue
		}
		runs = append(runs, cloneRun(run))
	}
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].CreatedAt > runs[j].CreatedAt })
	if limit > 0 && len(runs) > limit {
		runs = runs[:limit]
	}
	return runs, nil
}

func (s *fileStore) updateRun(id string, mutate func(*runRecord) error) (runRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, err := s.loadLocked()
	if err != nil {
		return runRecord{}, err
	}
	for i := range state.Runs {
		if state.Runs[i].ID != id {
			continue
		}
		run := cloneRun(state.Runs[i])
		if err := mutate(&run); err != nil {
			return runRecord{}, err
		}
		state.Runs[i] = cloneRun(run)
		if err := s.saveLocked(state); err != nil {
			return runRecord{}, err
		}
		return cloneRun(run), nil
	}
	return runRecord{}, errNotFound
}

func (s *fileStore) loadLocked() (persistedState, error) {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		state := persistedState{}
		if saveErr := s.saveLocked(state); saveErr != nil {
			return persistedState{}, saveErr
		}
		return state, nil
	}
	if err != nil {
		return persistedState{}, fmt.Errorf("read ingestion state: %w", err)
	}
	if len(data) == 0 {
		return persistedState{}, nil
	}
	var state persistedState
	if err := json.Unmarshal(data, &state); err != nil {
		return persistedState{}, fmt.Errorf("decode ingestion state: %w", err)
	}
	return state, nil
}

func (s *fileStore) saveLocked(state persistedState) error {
	tmp := s.path + ".tmp"
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode ingestion state: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create ingestion state root: %w", err)
	}
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write ingestion state: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("commit ingestion state: %w", err)
	}
	return nil
}
