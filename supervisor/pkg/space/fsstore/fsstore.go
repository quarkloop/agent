package fsstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	spacemodel "github.com/quarkloop/pkg/space"
	"github.com/quarkloop/supervisor/pkg/api"
	"github.com/quarkloop/supervisor/pkg/kb"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
	"github.com/quarkloop/supervisor/pkg/sessions"
	"github.com/quarkloop/supervisor/pkg/space"
	"github.com/quarkloop/supervisor/pkg/space/store"
)

// FSStore is a filesystem-backed Store implementation.
//
// Layout:
//
//	<root>/<name>/meta.json               - Space metadata.
//	<root>/<name>/Quarkfile               - Latest Quarkfile state.
//	<root>/<name>/kb/                     - KB collection directory.
//	<root>/<name>/plugins/                - Installed plugins directory.
//	<root>/<name>/sessions/               - Session JSONL files.
type FSStore struct {
	root string

	mu    sync.Mutex             // guards lock creation
	locks map[string]*sync.Mutex // per-space serialization
}

// DefaultRoot returns the default filesystem root for spaces:
// $QUARK_SPACES_ROOT when set, else $HOME/.quarkloop/spaces.
func DefaultRoot() (string, error) {
	if v := strings.TrimSpace(os.Getenv("QUARK_SPACES_ROOT")); v != "" {
		return filepath.Abs(v)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".quarkloop", "spaces"), nil
}

// NewFSStore returns a filesystem-backed Store rooted at root. The root
// directory is created if it does not exist.
func NewFSStore(root string) (*FSStore, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create spaces root: %w", err)
	}
	return &FSStore{
		root:  root,
		locks: make(map[string]*sync.Mutex),
	}, nil
}

// spaceLock returns a mutex scoped to a single space name.
func (s *FSStore) spaceLock(name string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.locks[name]
	if !ok {
		m = &sync.Mutex{}
		s.locks[name] = m
	}
	return m
}

func (s *FSStore) layout(name string) (spacemodel.Layout, error) {
	return spacemodel.NewLayout(s.root, name)
}

// readMeta loads the persisted Space metadata.
func (s *FSStore) readMeta(name string) (*space.Space, error) {
	l, err := s.layout(name)
	if err != nil {
		return nil, err
	}
	sp, err := spacemodel.ReadMetadata(l.MetaPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, store.NewNotFoundError(name)
	}
	if err != nil {
		return nil, err
	}
	return space.FromModel(*sp), nil
}

// writeMeta persists the Space metadata atomically.
func (s *FSStore) writeMeta(sp *space.Space) error {
	l, err := s.layout(sp.Name)
	if err != nil {
		return err
	}
	return spacemodel.WriteMetadata(l.MetaPath(), sp.Meta())
}

// Create registers a new space with the supervised layout and writes the
// Quarkfile to both supervisor storage and the working directory.
func (s *FSStore) Create(name string, quarkfileBytes []byte, workingDir string) (*space.Space, error) {
	if workingDir == "" {
		return nil, fmt.Errorf("working_dir is required")
	}
	l, err := s.layout(name)
	if err != nil {
		return nil, err
	}
	qf, err := spacemodel.ParseAndValidateQuarkfileForSpace(quarkfileBytes, name)
	if err != nil {
		return nil, err
	}

	lock := s.spaceLock(name)
	lock.Lock()
	defer lock.Unlock()

	if _, err := os.Stat(l.MetaPath()); err == nil {
		return nil, store.ErrAlreadyExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("stat meta: %w", err)
	}

	for _, dir := range l.RequiredDirs() {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create space dir %s: %w", dir, err)
		}
	}
	if err := spacemodel.WriteQuarkfileFile(l.QuarkfilePath(), quarkfileBytes); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir working dir: %w", err)
	}
	if err := spacemodel.WriteQuarkfile(workingDir, quarkfileBytes); err != nil {
		return nil, fmt.Errorf("write Quarkfile to working dir: %w", err)
	}

	now := time.Now().UTC()
	sp := &space.Space{
		Metadata: spacemodel.Metadata{
			Name:       name,
			WorkingDir: workingDir,
			Version:    qf.Meta.Version,
			CreatedAt:  now,
			UpdatedAt:  now,
		},
	}
	if err := s.writeMeta(sp); err != nil {
		return nil, err
	}
	return sp, nil
}

// UpdateQuarkfile replaces the latest Quarkfile state.
func (s *FSStore) UpdateQuarkfile(name string, quarkfileBytes []byte) (*space.Space, error) {
	qf, err := spacemodel.ParseAndValidateQuarkfileForSpace(quarkfileBytes, name)
	if err != nil {
		return nil, err
	}
	l, err := s.layout(name)
	if err != nil {
		return nil, err
	}
	lock := s.spaceLock(name)
	lock.Lock()
	defer lock.Unlock()

	sp, err := s.readMeta(name)
	if err != nil {
		return nil, err
	}
	if err := spacemodel.WriteQuarkfileFile(l.QuarkfilePath(), quarkfileBytes); err != nil {
		return nil, err
	}
	if err := spacemodel.WriteQuarkfile(sp.WorkingDir, quarkfileBytes); err != nil {
		return nil, fmt.Errorf("write Quarkfile to working dir: %w", err)
	}
	sp.Version = qf.Meta.Version
	sp.UpdatedAt = time.Now().UTC()
	if err := s.writeMeta(sp); err != nil {
		return nil, err
	}
	return sp, nil
}

// Get returns the metadata for the named space.
func (s *FSStore) Get(name string) (*space.Space, error) {
	return s.readMeta(name)
}

// List returns every registered space.
func (s *FSStore) List() ([]*space.Space, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, fmt.Errorf("read spaces root: %w", err)
	}
	out := make([]*space.Space, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sp, err := s.readMeta(e.Name())
		if err != nil {
			continue // skip malformed entries silently
		}
		out = append(out, sp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Delete permanently removes the named space and all of its data.
func (s *FSStore) Delete(name string) error {
	l, err := s.layout(name)
	if err != nil {
		return err
	}
	lock := s.spaceLock(name)
	lock.Lock()
	defer lock.Unlock()

	if _, err := os.Stat(l.Root); errors.Is(err, os.ErrNotExist) {
		return store.NewNotFoundError(name)
	}
	if err := os.RemoveAll(l.Root); err != nil {
		return fmt.Errorf("remove space dir: %w", err)
	}
	return nil
}

// Quarkfile returns the latest Quarkfile contents.
func (s *FSStore) Quarkfile(name string) ([]byte, error) {
	l, err := s.layout(name)
	if err != nil {
		return nil, err
	}
	if _, err := s.readMeta(name); err != nil {
		return nil, err
	}
	data, err := spacemodel.ReadQuarkfileFile(l.QuarkfilePath())
	if err != nil {
		return nil, err
	}
	return data, nil
}

// AgentEnvironment returns concrete environment entries derived from the
// Quarkfile model declarations. Missing declared variables fail fast at launch
// time so agents never inherit undeclared credentials accidentally.
func (s *FSStore) AgentEnvironment(name string) ([]string, error) {
	data, err := s.Quarkfile(name)
	if err != nil {
		return nil, err
	}
	qf, err := spacemodel.ParseAndValidateQuarkfileForSpace(data, name)
	if err != nil {
		return nil, err
	}
	env := make([]string, 0, len(qf.EnvironmentVariables())+2)
	if model, ok := qf.DefaultModel(); ok {
		env = append(env,
			"QUARK_MODEL_PROVIDER="+model.Provider,
			"QUARK_MODEL_NAME="+model.Name,
		)
	}
	names := qf.EnvironmentVariables()
	for _, key := range names {
		value, ok := os.LookupEnv(key)
		if !ok {
			return nil, fmt.Errorf("quarkfile declares %s but it is not set in supervisor environment", key)
		}
		env = append(env, key+"="+value)
	}
	return env, nil
}

// KB opens the KB store scoped to the named space.
func (s *FSStore) KB(name string) (kb.Store, error) {
	l, err := s.layout(name)
	if err != nil {
		return nil, err
	}
	if _, err := s.readMeta(name); err != nil {
		return nil, err
	}
	return kb.Open(l.KBPath())
}

// Plugins returns a plugin manager scoped to the named space.
func (s *FSStore) Plugins(name string) (*pluginmanager.Installer, error) {
	l, err := s.layout(name)
	if err != nil {
		return nil, err
	}
	if _, err := s.readMeta(name); err != nil {
		return nil, err
	}
	return pluginmanager.NewInstaller(l.PluginsPath()), nil
}

// Sessions returns the session store scoped to the named space.
func (s *FSStore) Sessions(name string) (*sessions.Store, error) {
	l, err := s.layout(name)
	if err != nil {
		return nil, err
	}
	if _, err := s.readMeta(name); err != nil {
		return nil, err
	}
	return sessions.Open(l.SessionsPath(), name)
}

// Doctor runs health checks against the named space's Quarkfile and
// installed plugins.
func (s *FSStore) Doctor(name string) (api.DoctorResponse, error) {
	qfBytes, err := s.Quarkfile(name)
	if err != nil {
		return api.DoctorResponse{}, err
	}

	mgr, err := s.Plugins(name)
	if err != nil {
		return api.DoctorResponse{}, err
	}

	installed, err := mgr.List()
	if err != nil {
		return api.DoctorResponse{}, err
	}

	return space.Doctor(qfBytes, installed), nil
}
