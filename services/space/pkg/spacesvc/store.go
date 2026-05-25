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

	spacemodel "github.com/quarkloop/pkg/space"
)

type Store struct {
	root string
	env  map[string]string

	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

type Paths struct {
	RootDir     string
	ConfigPath  string
	PluginsDir  string
	SessionsDir string
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

func (s *Store) Create(configBytes []byte) (*spacemodel.Config, error) {
	cfg, err := spacemodel.ParseConfig(configBytes)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	cfg.CreatedAt = now
	cfg.UpdatedAt = now
	if err := spacemodel.ValidateConfig(cfg); err != nil {
		return nil, err
	}
	layout, err := s.layout(cfg.Name)
	if err != nil {
		return nil, err
	}

	lock := s.spaceLock(cfg.Name)
	lock.Lock()
	defer lock.Unlock()

	if _, err := os.Stat(layout.ConfigPath()); err == nil {
		return nil, ErrAlreadyExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("stat space config: %w", err)
	}
	for _, dir := range layout.RequiredDirs() {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create space dir %s: %w", dir, err)
		}
	}
	data, err := spacemodel.MarshalConfig(cfg)
	if err != nil {
		return nil, err
	}
	if err := spacemodel.WriteConfigFile(layout.ConfigPath(), data); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (s *Store) UpdateConfig(configBytes []byte) (*spacemodel.Config, error) {
	cfg, err := spacemodel.ParseConfig(configBytes)
	if err != nil {
		return nil, err
	}
	layout, err := s.layout(cfg.Name)
	if err != nil {
		return nil, err
	}

	lock := s.spaceLock(cfg.Name)
	lock.Lock()
	defer lock.Unlock()

	existing, err := s.readConfig(cfg.Name)
	if err != nil {
		return nil, err
	}
	cfg.CreatedAt = existing.CreatedAt
	cfg.UpdatedAt = time.Now().UTC()
	if err := spacemodel.ValidateConfig(cfg); err != nil {
		return nil, err
	}
	data, err := spacemodel.MarshalConfig(cfg)
	if err != nil {
		return nil, err
	}
	if err := spacemodel.WriteConfigFile(layout.ConfigPath(), data); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (s *Store) Get(name string) (*spacemodel.Config, error) {
	return s.readConfig(name)
}

func (s *Store) List() ([]*spacemodel.Config, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, fmt.Errorf("read spaces root: %w", err)
	}
	out := make([]*spacemodel.Config, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		cfg, err := s.readConfig(entry.Name())
		if err != nil {
			continue
		}
		out = append(out, cfg)
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

func (s *Store) Config(name string) ([]byte, *spacemodel.Config, error) {
	layout, err := s.layout(name)
	if err != nil {
		return nil, nil, err
	}
	cfg, err := s.readConfig(name)
	if err != nil {
		return nil, nil, err
	}
	data, err := spacemodel.ReadConfigFile(layout.ConfigPath())
	if err != nil {
		return nil, nil, err
	}
	return data, cfg, nil
}

func (s *Store) AgentEnvironment(name string) ([]string, error) {
	data, _, err := s.Config(name)
	if err != nil {
		return nil, err
	}
	cfg, err := spacemodel.ParseAndValidateConfig(data, name)
	if err != nil {
		return nil, err
	}
	model, ok := cfg.DefaultModel()
	if !ok {
		return nil, fmt.Errorf("space config model is required to start an agent")
	}
	env := []string{
		"QUARK_MODEL_PROVIDER=" + model.Provider,
		"QUARK_MODEL_NAME=" + model.Name,
	}
	for _, key := range cfg.EnvironmentVariables() {
		value, ok := s.env[key]
		if !ok {
			return nil, fmt.Errorf("space config model.env declares %s but it is not set in space service environment", key)
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
	if _, err := s.readConfig(name); err != nil {
		return Paths{}, err
	}
	return Paths{
		RootDir:     layout.Root,
		ConfigPath:  layout.ConfigPath(),
		PluginsDir:  layout.PluginsPath(),
		SessionsDir: layout.SessionsPath(),
	}, nil
}

func (s *Store) Doctor(name string) (DoctorResult, error) {
	configBytes, _, err := s.Config(name)
	if err != nil {
		return DoctorResult{}, err
	}
	if _, err := spacemodel.ParseAndValidateConfig(configBytes, name); err != nil {
		return DoctorResult{OK: false, Issues: []DoctorIssue{{Severity: "error", Message: err.Error()}}}, nil
	}
	return DoctorResult{OK: true}, nil
}

func (s *Store) layout(name string) (spacemodel.Layout, error) {
	return spacemodel.NewLayout(s.root, name)
}

func (s *Store) readConfig(name string) (*spacemodel.Config, error) {
	layout, err := s.layout(name)
	if err != nil {
		return nil, err
	}
	data, err := spacemodel.ReadConfigFile(layout.ConfigPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, NotFoundError{Name: name}
	}
	if err != nil {
		return nil, err
	}
	return spacemodel.ParseAndValidateConfig(data, name)
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
