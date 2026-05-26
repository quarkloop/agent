package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"

	"github.com/quarkloop/pkg/serviceapi/clientcontract"
	spacemodel "github.com/quarkloop/pkg/space"
	"github.com/quarkloop/supervisor/pkg/pluginmanager"
	"github.com/quarkloop/supervisor/pkg/space"
)

func TestInspectServicesReportsNATSServiceFromManifest(t *testing.T) {
	srv := serviceTestServer(t)
	writeInstalledServicePlugin(t, srv, "test-space")

	services, err := srv.inspectServices(t.Context(), "test-space")
	if err != nil {
		t.Fatalf("inspect services: %v", err)
	}
	if len(services) != 1 {
		t.Fatalf("services = %+v", services)
	}
	if services[0].Status != clientcontract.ServiceStatusReady || services[0].SubjectPrefix != "svc.indexer.v1" {
		t.Fatalf("service info = %+v", services[0])
	}
}

func serviceTestServer(t *testing.T) *Server {
	t.Helper()
	bundled := t.TempDir()
	writeMainAgentPlugin(t, bundled)
	registry, err := pluginmanager.NewRegistry(bundled, filepath.Join(t.TempDir(), "installed"))
	if err != nil {
		t.Fatal(err)
	}
	store := newCatalogSpaceStore()
	if _, err := store.Create(testSpaceConfig(t, "test-space", t.TempDir())); err != nil {
		t.Fatal(err)
	}
	return &Server{store: store, pluginRegistry: registry}
}

func testSpaceConfig(t *testing.T, name, workDir string) []byte {
	t.Helper()
	data, err := spacemodel.MarshalConfig(spacemodel.NewConfig(name, workDir))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeMainAgentPlugin(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, "agents", "quark-main")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, dir, map[string]string{
		"manifest.yaml": `name: quark-main
version: "1.0.0"
type: agent
description: Main agent
agent:
  profile: PROFILE.yaml
  system: SYSTEM.md
  skill: SKILL.md
`,
		"PROFILE.yaml": "id: quark-main\nname: Quark Main\nrole: main\n",
		"SYSTEM.md":    "You are Quark Main.\n",
		"SKILL.md":     "Coordinate installed services.\n",
	})
}

func writeInstalledServicePlugin(t *testing.T, srv *Server, spaceID string) {
	t.Helper()
	writeInstalledServicePluginNamed(t, srv, spaceID, servicePluginFixture{
		Name:         "indexer",
		ProtoService: "quark.indexer.v1.IndexerService",
		FunctionName: "indexer_QueryContext",
	})
}

func installFixturePlugin(t *testing.T, srv *Server, dir string) {
	t.Helper()
	if _, err := srv.pluginRegistry.Install(context.Background(), dir); err != nil {
		t.Fatalf("install fixture plugin: %v", err)
	}
}

func selectServicePlugin(t *testing.T, srv *Server, spaceID, name string) {
	t.Helper()
	config, err := srv.readSpaceConfig(spaceID)
	if err != nil {
		t.Fatal(err)
	}
	service := spacemodel.ServiceRef{Name: name, Ref: name}
	if err := srv.writeSpaceConfig(config.WithPluginSelection(spacemodel.PluginRef{Ref: name}, &service)); err != nil {
		t.Fatal(err)
	}
}

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

type catalogSpaceStore struct {
	mu      sync.Mutex
	configs map[string][]byte
	records map[string]map[string][]byte
}

func newCatalogSpaceStore() *catalogSpaceStore {
	return &catalogSpaceStore{configs: make(map[string][]byte), records: make(map[string]map[string][]byte)}
}

func (s *catalogSpaceStore) Create(data []byte) (*space.Space, error) {
	cfg, err := spacemodel.ParseConfig(data)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.configs[cfg.Name]; exists {
		return nil, space.ErrAlreadyExists
	}
	s.configs[cfg.Name] = append([]byte(nil), data...)
	return space.FromConfig(*cfg), nil
}

func (s *catalogSpaceStore) UpdateConfig(data []byte) (*space.Space, error) {
	cfg, err := spacemodel.ParseConfig(data)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.configs[cfg.Name]; !exists {
		return nil, space.NewNotFoundError(cfg.Name)
	}
	s.configs[cfg.Name] = append([]byte(nil), data...)
	return space.FromConfig(*cfg), nil
}

func (s *catalogSpaceStore) Get(name string) (*space.Space, error) {
	data, err := s.Config(name)
	if err != nil {
		return nil, err
	}
	cfg, err := spacemodel.ParseConfig(data)
	if err != nil {
		return nil, err
	}
	return space.FromConfig(*cfg), nil
}

func (s *catalogSpaceStore) List() ([]*space.Space, error) {
	s.mu.Lock()
	names := make([]string, 0, len(s.configs))
	for name := range s.configs {
		names = append(names, name)
	}
	s.mu.Unlock()
	sort.Strings(names)
	out := make([]*space.Space, 0, len(names))
	for _, name := range names {
		item, err := s.Get(name)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *catalogSpaceStore) Delete(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.configs[name]; !exists {
		return space.NewNotFoundError(name)
	}
	delete(s.configs, name)
	return nil
}

func (s *catalogSpaceStore) Config(name string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, exists := s.configs[name]
	if !exists {
		return nil, space.NewNotFoundError(name)
	}
	return append([]byte(nil), data...), nil
}

func (s *catalogSpaceStore) PutRecord(name, namespace, key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	recordKey := fmt.Sprintf("%s/%s/%s", name, namespace, key)
	s.records[recordKey] = map[string][]byte{"data": append([]byte(nil), data...)}
	return nil
}

func (s *catalogSpaceStore) GetRecord(name, namespace, key string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, exists := s.records[fmt.Sprintf("%s/%s/%s", name, namespace, key)]
	if !exists {
		return nil, space.NewNotFoundError(key)
	}
	return append([]byte(nil), record["data"]...), nil
}

func (s *catalogSpaceStore) ListRecords(name, namespace string) ([][]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prefix := fmt.Sprintf("%s/%s/", name, namespace)
	out := make([][]byte, 0)
	for key, record := range s.records {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			out = append(out, append([]byte(nil), record["data"]...))
		}
	}
	return out, nil
}

func (s *catalogSpaceStore) DeleteRecord(name, namespace, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	recordKey := fmt.Sprintf("%s/%s/%s", name, namespace, key)
	if _, exists := s.records[recordKey]; !exists {
		return space.NewNotFoundError(key)
	}
	delete(s.records, recordKey)
	return nil
}

func (s *catalogSpaceStore) Doctor(name string) (space.DoctorResult, error) {
	if _, err := s.Config(name); err != nil {
		return space.DoctorResult{}, err
	}
	return space.DoctorResult{OK: true}, nil
}

var _ space.Store = (*catalogSpaceStore)(nil)
