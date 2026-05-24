package spacesvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/quarkloop/pkg/plugin"
	spacemodel "github.com/quarkloop/pkg/space"
)

type Store struct {
	root string
	env  map[string]string

	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

type Paths struct {
	RootDir       string
	QuarkfilePath string
	KBDir         string
	PluginsDir    string
	SessionsDir   string
}

type DoctorResult struct {
	OK     bool
	Issues []DoctorIssue
}

type DoctorIssue struct {
	Severity string
	Message  string
}

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

func NewStore(root string) (*Store, error) {
	return NewStoreWithEnvironment(root, nil)
}

func NewStoreWithEnvironment(root string, environment []string) (*Store, error) {
	if root == "" {
		r, err := DefaultRoot()
		if err != nil {
			return nil, err
		}
		root = r
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create spaces root: %w", err)
	}
	return &Store{
		root:  root,
		env:   environmentMap(environment),
		locks: make(map[string]*sync.Mutex),
	}, nil
}

func (s *Store) Root() string { return s.root }

func (s *Store) Create(name string, quarkfileBytes []byte, workingDir string) (*spacemodel.Metadata, error) {
	if workingDir == "" {
		return nil, fmt.Errorf("working_dir is required")
	}
	layout, err := s.layout(name)
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

	if _, err := os.Stat(layout.MetaPath()); err == nil {
		return nil, ErrAlreadyExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("stat meta: %w", err)
	}
	for _, dir := range layout.RequiredDirs() {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create space dir %s: %w", dir, err)
		}
	}
	if err := spacemodel.WriteQuarkfileFile(layout.QuarkfilePath(), quarkfileBytes); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(workingDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir working dir: %w", err)
	}
	if err := spacemodel.WriteQuarkfile(workingDir, quarkfileBytes); err != nil {
		return nil, fmt.Errorf("write Quarkfile to working dir: %w", err)
	}

	now := time.Now().UTC()
	meta := &spacemodel.Metadata{
		Name:       name,
		WorkingDir: workingDir,
		Version:    qf.Meta.Version,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := spacemodel.WriteMetadata(layout.MetaPath(), meta); err != nil {
		return nil, err
	}
	return meta, nil
}

func (s *Store) UpdateQuarkfile(name string, quarkfileBytes []byte) (*spacemodel.Metadata, error) {
	qf, err := spacemodel.ParseAndValidateQuarkfileForSpace(quarkfileBytes, name)
	if err != nil {
		return nil, err
	}
	layout, err := s.layout(name)
	if err != nil {
		return nil, err
	}

	lock := s.spaceLock(name)
	lock.Lock()
	defer lock.Unlock()

	meta, err := s.readMeta(name)
	if err != nil {
		return nil, err
	}
	if err := spacemodel.WriteQuarkfileFile(layout.QuarkfilePath(), quarkfileBytes); err != nil {
		return nil, err
	}
	if err := spacemodel.WriteQuarkfile(meta.WorkingDir, quarkfileBytes); err != nil {
		return nil, fmt.Errorf("write Quarkfile to working dir: %w", err)
	}
	meta.Version = qf.Meta.Version
	meta.UpdatedAt = time.Now().UTC()
	if err := spacemodel.WriteMetadata(layout.MetaPath(), meta); err != nil {
		return nil, err
	}
	return meta, nil
}

func (s *Store) Get(name string) (*spacemodel.Metadata, error) {
	return s.readMeta(name)
}

func (s *Store) List() ([]*spacemodel.Metadata, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, fmt.Errorf("read spaces root: %w", err)
	}
	out := make([]*spacemodel.Metadata, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		meta, err := s.readMeta(entry.Name())
		if err != nil {
			continue
		}
		out = append(out, meta)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (s *Store) Delete(name string) error {
	layout, err := s.layout(name)
	if err != nil {
		return err
	}
	lock := s.spaceLock(name)
	lock.Lock()
	defer lock.Unlock()
	if _, err := os.Stat(layout.Root); errors.Is(err, os.ErrNotExist) {
		return NotFoundError{Name: name}
	}
	if err := os.RemoveAll(layout.Root); err != nil {
		return fmt.Errorf("remove space dir: %w", err)
	}
	return nil
}

func (s *Store) Quarkfile(name string) ([]byte, *spacemodel.Metadata, error) {
	layout, err := s.layout(name)
	if err != nil {
		return nil, nil, err
	}
	meta, err := s.readMeta(name)
	if err != nil {
		return nil, nil, err
	}
	data, err := spacemodel.ReadQuarkfileFile(layout.QuarkfilePath())
	if err != nil {
		return nil, nil, err
	}
	return data, meta, nil
}

func (s *Store) AgentEnvironment(name string) ([]string, error) {
	data, _, err := s.Quarkfile(name)
	if err != nil {
		return nil, err
	}
	qf, err := spacemodel.ParseAndValidateQuarkfileForSpace(data, name)
	if err != nil {
		return nil, err
	}
	model, ok := qf.DefaultModel()
	if !ok {
		return nil, fmt.Errorf("quarkfile model is required to start an agent")
	}
	env := []string{
		"QUARK_MODEL_PROVIDER=" + model.Provider,
		"QUARK_MODEL_NAME=" + model.Name,
	}
	for _, key := range qf.EnvironmentVariables() {
		value, ok := s.env[key]
		if !ok {
			return nil, fmt.Errorf("quarkfile model.env declares %s but it is not set in space service environment", key)
		}
		env = append(env, key+"="+value)
	}
	return env, nil
}

func environmentMap(entries []string) map[string]string {
	out := make(map[string]string, len(entries))
	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		key = strings.TrimSpace(key)
		if !ok || key == "" {
			continue
		}
		out[key] = value
	}
	return out
}

func (s *Store) Paths(name string) (Paths, error) {
	layout, err := s.layout(name)
	if err != nil {
		return Paths{}, err
	}
	if _, err := s.readMeta(name); err != nil {
		return Paths{}, err
	}
	return Paths{
		RootDir:       layout.Root,
		QuarkfilePath: layout.QuarkfilePath(),
		KBDir:         layout.KBPath(),
		PluginsDir:    layout.PluginsPath(),
		SessionsDir:   layout.SessionsPath(),
	}, nil
}

func (s *Store) Doctor(name string) (DoctorResult, error) {
	quarkfileBytes, _, err := s.Quarkfile(name)
	if err != nil {
		return DoctorResult{}, err
	}
	qf, err := spacemodel.ParseAndValidateQuarkfile(quarkfileBytes)
	if err != nil {
		return DoctorResult{OK: false, Issues: []DoctorIssue{{Severity: "error", Message: err.Error()}}}, nil
	}
	installed, err := s.installedPlugins(name)
	if err != nil {
		return DoctorResult{}, err
	}
	out := DoctorResult{OK: true}
	for _, ref := range qf.Plugins {
		name := pluginNameFromRef(ref.Ref)
		if !installed[name] {
			out.OK = false
			out.Issues = append(out.Issues, DoctorIssue{
				Severity: "error",
				Message:  fmt.Sprintf("plugin %q (ref %q) referenced in Quarkfile but not installed", name, ref.Ref),
			})
		}
	}
	return out, nil
}

func (s *Store) layout(name string) (spacemodel.Layout, error) {
	return spacemodel.NewLayout(s.root, name)
}

func (s *Store) readMeta(name string) (*spacemodel.Metadata, error) {
	layout, err := s.layout(name)
	if err != nil {
		return nil, err
	}
	meta, err := spacemodel.ReadMetadata(layout.MetaPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, NotFoundError{Name: name}
	}
	if err != nil {
		return nil, err
	}
	return meta, nil
}

func (s *Store) spaceLock(name string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	m, ok := s.locks[name]
	if !ok {
		m = &sync.Mutex{}
		s.locks[name] = m
	}
	return m
}

func (s *Store) installedPlugins(name string) (map[string]bool, error) {
	paths, err := s.Paths(name)
	if err != nil {
		return nil, err
	}
	installed := make(map[string]bool)
	for _, typ := range []string{"tools", "agents", "skills", "services"} {
		root := filepath.Join(paths.PluginsDir, typ)
		entries, err := os.ReadDir(root)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			manifest, err := plugin.ParseManifest(filepath.Join(root, entry.Name(), "manifest.yaml"))
			if err == nil {
				installed[manifest.Name] = true
			}
			installed[entry.Name()] = true
		}
	}
	return installed, nil
}

func pluginNameFromRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if idx := strings.LastIndexByte(ref, '/'); idx >= 0 {
		return ref[idx+1:]
	}
	return ref
}
