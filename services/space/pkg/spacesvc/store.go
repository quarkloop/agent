package spacesvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	spacemodel "github.com/quarkloop/pkg/space"
)

type Store struct {
	root string

	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

type Record struct {
	Namespace string
	Key       string
	Data      []byte
	UpdatedAt time.Time
}

type DoctorResult struct {
	OK     bool
	Issues []DoctorIssue
}

type DoctorIssue struct {
	Severity string
	Message  string
}

func NewStore(root string) (*Store, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("space storage root is required")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create spaces root: %w", err)
	}
	return &Store{
		root:  root,
		locks: make(map[string]*sync.Mutex),
	}, nil
}

func (s *Store) Root() string { return s.root }

func (s *Store) Create(configBytes []byte) (*spacemodel.Config, error) {
	cfg, err := spacemodel.ParseConfig(configBytes)
	if err != nil {
		return nil, err
	}
	cfg = cfg.WithCreatedTimestamps(time.Now())
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
	cfg = cfg.WithUpdatedTimestamp(existing.CreatedAt, time.Now())
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

var recordTokenPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func (s *Store) PutRecord(name, namespace, key string, data []byte) (Record, error) {
	path, err := s.recordPath(name, namespace, key)
	if err != nil {
		return Record{}, err
	}
	lock := s.spaceLock(name)
	lock.Lock()
	defer lock.Unlock()
	if _, err := s.readConfig(name); err != nil {
		return Record{}, err
	}
	if err := writeAtomic(path, data); err != nil {
		return Record{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return Record{}, fmt.Errorf("stat record: %w", err)
	}
	return Record{Namespace: namespace, Key: key, Data: append([]byte(nil), data...), UpdatedAt: info.ModTime().UTC()}, nil
}

func (s *Store) GetRecord(name, namespace, key string) (Record, error) {
	path, err := s.recordPath(name, namespace, key)
	if err != nil {
		return Record{}, err
	}
	if _, err := s.readConfig(name); err != nil {
		return Record{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Record{}, NotFoundError{Name: namespace + "/" + key}
	}
	if err != nil {
		return Record{}, fmt.Errorf("read record: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return Record{}, fmt.Errorf("stat record: %w", err)
	}
	return Record{Namespace: namespace, Key: key, Data: data, UpdatedAt: info.ModTime().UTC()}, nil
}

func (s *Store) ListRecords(name, namespace string) ([]Record, error) {
	dir, err := s.recordNamespacePath(name, namespace)
	if err != nil {
		return nil, err
	}
	if _, err := s.readConfig(name); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Record{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read record namespace: %w", err)
	}
	records := make([]Record, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".bin" {
			continue
		}
		key := strings.TrimSuffix(entry.Name(), ".bin")
		record, err := s.GetRecord(name, namespace, key)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Key < records[j].Key })
	return records, nil
}

func (s *Store) DeleteRecord(name, namespace, key string) error {
	path, err := s.recordPath(name, namespace, key)
	if err != nil {
		return err
	}
	lock := s.spaceLock(name)
	lock.Lock()
	defer lock.Unlock()
	if _, err := s.readConfig(name); err != nil {
		return err
	}
	if err := os.Remove(path); errors.Is(err, os.ErrNotExist) {
		return NotFoundError{Name: namespace + "/" + key}
	} else if err != nil {
		return fmt.Errorf("delete record: %w", err)
	}
	return nil
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

func (s *Store) recordNamespacePath(name, namespace string) (string, error) {
	if !recordTokenPattern.MatchString(namespace) {
		return "", fmt.Errorf("record namespace %q is invalid", namespace)
	}
	layout, err := s.layout(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(layout.RecordsPath(), namespace), nil
}

func (s *Store) recordPath(name, namespace, key string) (string, error) {
	dir, err := s.recordNamespacePath(name, namespace)
	if err != nil {
		return "", err
	}
	if !recordTokenPattern.MatchString(key) {
		return "", fmt.Errorf("record key %q is invalid", key)
	}
	return filepath.Join(dir, key+".bin"), nil
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create record directory: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append([]byte(nil), data...), 0o600); err != nil {
		return fmt.Errorf("write record: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("replace record: %w", err)
	}
	return nil
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
